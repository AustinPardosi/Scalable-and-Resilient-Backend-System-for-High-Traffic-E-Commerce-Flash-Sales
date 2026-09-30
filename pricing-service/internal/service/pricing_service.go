package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var ErrNotFound = errors.New("product not found")

// Scarcity pricing: the price climbs as the last units go.
const (
	lowStock      = 50 // at or below: +10%
	criticalStock = 10 // at or below: +25%
)

// A cached price lives 10s from when its base price was fetched from product-service.
// stock.changed events update its stock tier in between but don't extend that, so a base
// price change or a lost or out-of-order event is corrected within 10s.
const cacheTTL = 10 * time.Second

// DynamicPrice is the price of a product with the given base price and stock left.
func DynamicPrice(baseCents int64, stock int) int64 {
	switch {
	case stock <= criticalStock:
		return baseCents * 125 / 100
	case stock <= lowStock:
		return baseCents * 110 / 100
	}
	return baseCents
}

type Price struct {
	ProductID      int64 `json:"product_id"`
	BasePriceCents int64 `json:"base_price_cents"`
	PriceCents     int64 `json:"price_cents"`
	Stock          int   `json:"stock"`
}

// product is what pricing needs from the product service's product JSON, which
// both GET /products/:id and stock.changed events carry.
type product struct {
	ID         int64 `json:"id"`
	PriceCents int64 `json:"price_cents"`
	Stock      int   `json:"stock"`
}

type PricingService struct {
	redis             *redis.Client
	productServiceURL string
	client            *http.Client
}

func NewPricingService(rdb *redis.Client, productServiceURL string) *PricingService {
	return &PricingService{redis: rdb, productServiceURL: productServiceURL, client: &http.Client{Timeout: 2 * time.Second}}
}

// GetPrice serves from Redis; on a miss it asks the product service and caches the answer.
// ponytail: concurrent misses each call the product service; add singleflight if load tests show it.
func (s *PricingService) GetPrice(ctx context.Context, productID int64) (*Price, error) {
	if p := s.cached(ctx, productID); p != nil {
		return p, nil
	}
	pr, err := s.fetchProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	p := toPrice(pr)
	s.save(ctx, p, redis.SetArgs{TTL: cacheTTL})
	return p, nil
}

// OnStockChanged moves a cached price to the tier for the new stock level. Only the stock
// level is taken from the event; the base price always comes from product-service, so a
// forged event can at most shift a price between tiers, never set it. Products not in the
// cache are skipped: their next GetPrice fetches them fresh.
func (s *PricingService) OnStockChanged(ctx context.Context, value []byte) error {
	var evt product
	if err := json.Unmarshal(value, &evt); err != nil {
		return err
	}
	if p := s.cached(ctx, evt.ID); p != nil {
		// XX + KEEPTTL: never resurrect an expired entry, never extend its life.
		s.save(ctx, toPrice(product{ID: evt.ID, PriceCents: p.BasePriceCents, Stock: evt.Stock}),
			redis.SetArgs{Mode: "XX", KeepTTL: true})
	}
	return nil
}

func (s *PricingService) cached(ctx context.Context, productID int64) *Price {
	value, err := s.redis.Get(ctx, cacheKey(productID)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Warn().Err(err).Msg("redis get failed") // a cache outage isn't a pricing outage
		}
		return nil
	}
	var p Price
	if json.Unmarshal(value, &p) != nil {
		return nil
	}
	return &p
}

func (s *PricingService) save(ctx context.Context, p *Price, args redis.SetArgs) {
	value, _ := json.Marshal(p)
	if err := s.redis.SetArgs(ctx, cacheKey(p.ProductID), value, args).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Warn().Err(err).Int64("product_id", p.ProductID).Msg("redis set failed")
	}
}

func toPrice(pr product) *Price {
	return &Price{ProductID: pr.ID, BasePriceCents: pr.PriceCents, PriceCents: DynamicPrice(pr.PriceCents, pr.Stock), Stock: pr.Stock}
}

func (s *PricingService) fetchProduct(ctx context.Context, productID int64) (product, error) {
	var pr product
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/products/%d", s.productServiceURL, productID), nil)
	if err != nil {
		return pr, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return pr, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return pr, json.NewDecoder(resp.Body).Decode(&pr)
	case http.StatusNotFound:
		return pr, ErrNotFound
	}
	return pr, fmt.Errorf("product service returned %d", resp.StatusCode)
}

func cacheKey(productID int64) string {
	return "price:" + strconv.FormatInt(productID, 10)
}
