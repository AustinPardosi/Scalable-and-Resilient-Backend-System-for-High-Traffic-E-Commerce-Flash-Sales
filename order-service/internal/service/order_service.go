package service

import "order-service/internal/entity"

type OrderService struct {
}

// NewOrderService creates a new order service
func NewOrderService() *OrderService {
	return &OrderService{}
}

// CreateOrder creates a new order
func (s *OrderService) CreateOrder(order *entity.Order) (*entity.Order, error) {
	order.ID = 1
	order.Status = "created"
	order.Total = 100.0
	return order, nil
}

// UpdateOrder updates an existing order
func (s *OrderService) UpdateOrder(order *entity.Order) (*entity.Order, error) {
	return order, nil
}

// CancelOrder cancels an existing order
func (s *OrderService) CancelOrder(id int) (*entity.Order, error) {
	order := &entity.Order{
		ID:     id,
		Status: "cancelled",
	}
	return order, nil
}
