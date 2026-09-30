package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"order-service/internal/entity"
	"order-service/internal/repository"
	"slices"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	TopicOrderCreated  = "order.created"
	TopicOrderPaid     = "order.paid"
	TopicOrderCanceled = "order.canceled"
)

var (
	ErrInvalid     = errors.New("invalid request")
	ErrNotFound    = repository.ErrNotFound
	ErrConflict    = errors.New("conflict")
	ErrUnavailable = errors.New("dependency unavailable")
	errOutOfStock  = errors.New("out of stock")
	errKeyReused   = fmt.Errorf("%w: this Idempotency-Key was already used for a different order", ErrInvalid)
)

const (
	// Caps how much of a sale one order can hold, so a single request can't buy it all.
	// ponytail: per order, not per user; per-user limits need a business rule first.
	maxQuantityPerItem = 10
	maxPriceCents      = 100_000_000 // $1M: anything above is a broken or tampered price
)

type OrderService struct {
	orderRepo         *repository.OrderRepository
	productServiceURL string
	pricingServiceURL string
	internalToken     string
	client            *http.Client
}

// NewOrderService creates a new order service
func NewOrderService(orderRepo *repository.OrderRepository, productServiceURL, pricingServiceURL, internalToken string) *OrderService {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 100 // the default of 2 means a new connection per request under load
	return &OrderService{
		orderRepo:         orderRepo,
		productServiceURL: productServiceURL,
		pricingServiceURL: pricingServiceURL,
		internalToken:     internalToken,
		client:            &http.Client{Timeout: 3 * time.Second, Transport: transport},
	}
}

// CreateOrder prices an order on the server, places it and reserves its stock.
// Safe to retry with the same idempotency key: a retry returns the original order,
// or finishes it if the first attempt was cut short.
func (s *OrderService) CreateOrder(ctx context.Context, userID int64, idempotencyKey string, items []entity.Item) (*entity.Order, error) {
	if err := validate(items); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		existing, err := s.orderRepo.GetOrderByKey(ctx, userID, idempotencyKey)
		if err == nil {
			if !sameItems(existing.Items, items) {
				return nil, errKeyReused
			}
			return s.finish(ctx, existing)
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
	}

	order := &entity.Order{UserID: userID, Status: entity.StatusPending, Items: slices.Clone(items)}
	for i := range order.Items {
		// Only the server sets prices; whatever the client sent is overwritten.
		price, err := s.getPrice(ctx, order.Items[i].ProductID)
		if err != nil {
			return nil, err
		}
		order.Items[i].UnitPriceCents = price
		order.TotalCents += price * int64(order.Items[i].Quantity)
	}

	if err := s.orderRepo.CreateOrder(ctx, order, idempotencyKey); err != nil {
		return nil, err
	}
	// A concurrent request with the same key may have won the insert; order is then theirs.
	if !sameItems(order.Items, items) {
		return nil, errKeyReused
	}
	return s.finish(ctx, order)
}

// finish drives a PENDING order to CONFIRMED (stock reserved) or CANCELED (sold out).
// Orders in any other status are returned as they are.
func (s *OrderService) finish(ctx context.Context, order *entity.Order) (*entity.Order, error) {
	if order.Status != entity.StatusPending {
		return order, nil
	}
	err := s.reserve(ctx, order)
	switch {
	case errors.Is(err, errOutOfStock):
		_, err = s.orderRepo.Transition(ctx, order.ID, entity.StatusCanceled, "out_of_stock", TopicOrderCanceled, entity.StatusPending)
	case err != nil:
		// The order stays PENDING: a retry with the same key picks it up, or the sweeper cancels it.
		return nil, fmt.Errorf("%w: reserve order %d: %v", ErrUnavailable, order.ID, err)
	default:
		_, err = s.orderRepo.Transition(ctx, order.ID, entity.StatusConfirmed, "", TopicOrderCreated, entity.StatusPending)
	}
	if err != nil {
		return nil, err
	}
	// Re-read: a concurrent duplicate or a cancel may have moved the order first.
	return s.orderRepo.GetOrderByID(ctx, order.ID)
}

// GetOrder returns the user's own order; other users' orders don't exist to them
func (s *OrderService) GetOrder(ctx context.Context, userID, id int64) (*entity.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, ErrNotFound
	}
	return order, nil
}

// PayOrder marks a confirmed order as paid (payment itself is out of scope)
func (s *OrderService) PayOrder(ctx context.Context, userID, id int64) (*entity.Order, error) {
	return s.transition(ctx, userID, id, entity.StatusPaid, "", TopicOrderPaid, entity.StatusConfirmed)
}

// CancelOrder cancels an unpaid order; its stock is released via the order.canceled event
func (s *OrderService) CancelOrder(ctx context.Context, userID, id int64) (*entity.Order, error) {
	return s.transition(ctx, userID, id, entity.StatusCanceled, "user", TopicOrderCanceled, entity.StatusPending, entity.StatusConfirmed)
}

func (s *OrderService) transition(ctx context.Context, userID, id int64, to, reason, topic string, from ...string) (*entity.Order, error) {
	if _, err := s.GetOrder(ctx, userID, id); err != nil {
		return nil, err
	}
	ok, err := s.orderRepo.Transition(ctx, id, to, reason, topic, from...)
	if err != nil {
		return nil, err
	}
	order, err := s.orderRepo.GetOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: order is %s", ErrConflict, order.Status)
	}
	return order, nil
}

const (
	pendingTimeout = 2 * time.Minute  // the reserve call failed and the client never retried
	paymentTimeout = 15 * time.Minute // stops buyers hoarding flash-sale stock without paying
)

// RunSweeper cancels stale orders until ctx is done. Canceling releases the stock they hold.
func (s *OrderService) RunSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.cancelStale(ctx, entity.StatusPending, pendingTimeout, "timeout")
		s.cancelStale(ctx, entity.StatusConfirmed, paymentTimeout, "payment_timeout")
	}
}

func (s *OrderService) cancelStale(ctx context.Context, status string, age time.Duration, reason string) {
	ids, err := s.orderRepo.StaleOrders(ctx, status, age)
	if err != nil {
		log.Error().Err(err).Str("status", status).Msg("find stale orders")
		return
	}
	for _, id := range ids {
		// Conditional on status: a payment that lands first wins, and this becomes a no-op.
		if _, err := s.orderRepo.Transition(ctx, id, entity.StatusCanceled, reason, TopicOrderCanceled, status); err != nil {
			log.Error().Err(err).Int64("order_id", id).Msg("cancel stale order")
		}
	}
}

func validate(items []entity.Item) error {
	if len(items) == 0 || len(items) > 20 {
		return fmt.Errorf("%w: an order needs 1-20 items", ErrInvalid)
	}
	seen := map[int64]bool{}
	for _, it := range items {
		if it.Quantity <= 0 || it.Quantity > maxQuantityPerItem || seen[it.ProductID] {
			return fmt.Errorf("%w: list each product once, with a quantity of 1-%d", ErrInvalid, maxQuantityPerItem)
		}
		seen[it.ProductID] = true
	}
	return nil
}

// sameItems reports whether a and b order the same quantities of the same products.
// Both must already be validated (each product listed once).
func sameItems(a, b []entity.Item) bool {
	if len(a) != len(b) {
		return false
	}
	want := make(map[int64]int, len(a))
	for _, it := range a {
		want[it.ProductID] = it.Quantity
	}
	for _, it := range b {
		if q, ok := want[it.ProductID]; !ok || q != it.Quantity {
			return false
		}
	}
	return true
}

// reserve asks the product service to take stock for the whole order, all or nothing.
// Idempotent per order ID on the product side, so retrying is always safe.
func (s *OrderService) reserve(ctx context.Context, order *entity.Order) error {
	body, err := json.Marshal(map[string]any{"order_id": order.ID, "items": order.Items})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.productServiceURL+"/internal/reservations", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", s.internalToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusConflict:
		return errOutOfStock
	}
	return fmt.Errorf("product service returned %d", resp.StatusCode)
}

func (s *OrderService) getPrice(ctx context.Context, productID int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/prices/%d", s.pricingServiceURL, productID), nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: pricing: %v", ErrUnavailable, err)
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return 0, fmt.Errorf("%w: unknown product %d", ErrInvalid, productID)
	default:
		return 0, fmt.Errorf("%w: pricing returned %d", ErrUnavailable, resp.StatusCode)
	}
	var price struct {
		PriceCents int64 `json:"price_cents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&price); err != nil {
		return 0, fmt.Errorf("%w: pricing: %v", ErrUnavailable, err)
	}
	// Orders are charged at this number, so a broken or tampered upstream must not be able
	// to make items free, negative, or large enough to overflow a total.
	if price.PriceCents <= 0 || price.PriceCents > maxPriceCents {
		return 0, fmt.Errorf("%w: pricing returned %d cents for product %d", ErrUnavailable, price.PriceCents, productID)
	}
	return price.PriceCents, nil
}

// drain reads the body to the end so the connection goes back to the pool.
func drain(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
