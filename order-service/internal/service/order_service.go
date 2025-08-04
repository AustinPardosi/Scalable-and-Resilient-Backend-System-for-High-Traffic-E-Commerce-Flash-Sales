package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"order-service/internal/entity"
	"order-service/internal/repository"
	"os"

	"github.com/rs/zerolog"
)

var logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

type OrderService struct {
	orderRepo         repository.OrderRepository
	productServiceURL string
	pricingServiceURL string
}

// NewOrderService creates a new order service
func NewOrderService(orderRepo repository.OrderRepository, productServiceURL string, pricingServiceURL string) *OrderService {
	return &OrderService{
		orderRepo:         orderRepo,
		productServiceURL: productServiceURL,
		pricingServiceURL: pricingServiceURL,
	}
}

// CreateOrder creates a new order
func (s *OrderService) CreateOrder(order *entity.Order) (*entity.Order, error) {
	for _, productRequest := range order.ProductRequests {
		// Check product stock
		available, err := s.checkProductStock(productRequest.ProductID, productRequest.Quantity)
		if err != nil {
			logger.Error().Err(err).Msgf("Error checking stock for product %d", productRequest.ProductID)
			return nil, err
		}

		// Get pricing details
		pricing, err := s.getPricing(productRequest.ProductID)
		if err != nil {
			logger.Error().Err(err).Msgf("Error getting pricing for product %d", productRequest.ProductID)
			return nil, err
		}

		if !available {
			logger.Warn().Msgf("Product %d is out of stock", productRequest.ProductID)
			return nil, fmt.Errorf("product %d is out of stock", productRequest.ProductID)
		}

		productRequest.MarkUp = float64(productRequest.Quantity) * pricing.MarkUp
		productRequest.Discount = float64(productRequest.Quantity) * pricing.Discount
		productRequest.FinalPrice = float64(productRequest.Quantity) * pricing.FinalPrice
	}

	order.Total = 0
	for _, productRequest := range order.ProductRequests {
		order.Total += productRequest.FinalPrice
	}

	createdOrder, err := s.orderRepo.CreateOrder(order)
	if err != nil {
		logger.Error().Err(err).Msg("Error creating order")
		return nil, err
	}
	return createdOrder, nil
}

// UpdateOrder updates an existing order
func (s *OrderService) UpdateOrder(order *entity.Order) (*entity.Order, error) {
	updatedOrder, err := s.orderRepo.UpdateOrder(order)
	if err != nil {
		logger.Error().Err(err).Msg("Error updating order")
		return nil, err
	}
	return updatedOrder, nil
}

// CancelOrder cancels an existing order
func (s *OrderService) CancelOrder(id int) (*entity.Order, error) {
	order, err := s.orderRepo.GetOrderByID(id)
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving order by ID %d", id)
		return nil, fmt.Errorf("failed to get order: %w", err)
	}
	order.Status = "cancelled"
	updatedOrder, err := s.orderRepo.UpdateOrder(order)
	if err != nil {
		logger.Error().Err(err).Msgf("Error updating order status for ID %d", id)
		return nil, err
	}
	return updatedOrder, nil
}

func (s *OrderService) checkProductStock(productID int, quantity int) (bool, error) {
	response, err := http.Get(fmt.Sprintf("%s/products/%d/stock", s.productServiceURL, productID))
	if err != nil {
		logger.Error().Err(err).Msgf("Error checking stock for product %d", productID)
		return false, fmt.Errorf("failed to check product stock: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		logger.Warn().Msgf("Product %d not available", productID)
		return false, fmt.Errorf("product not available")
	}

	var stockData map[string]int
	if err := json.NewDecoder(response.Body).Decode(&stockData); err != nil {
		logger.Error().Err(err).Msgf("Error decoding stock data for product %d", productID)
		return false, err
	}

	availableStock := stockData["stock"]
	return availableStock >= quantity, nil
}

func (s *OrderService) getPricing(productID int) (*entity.Pricing, error) {
	response, err := http.Get(fmt.Sprintf("%s/pricing/%d", s.pricingServiceURL, productID))
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving pricing for product %d", productID)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		logger.Warn().Msgf("Pricing not found for product %d", productID)
		return nil, fmt.Errorf("pricing not found")
	}

	var pricing entity.Pricing
	if err := json.NewDecoder(response.Body).Decode(&pricing); err != nil {
		logger.Error().Err(err).Msgf("Error decoding pricing data for product %d", productID)
		return nil, err
	}

	return &pricing, nil
}
