package repository

import "order-service/internal/entity"

type OrderRepository struct {
}

func NewOrderRepository() *OrderRepository {
	return &OrderRepository{}
}

var orders = map[int]*entity.Order{
	1: {ID: 1, ProductRequests: make([]entity.ProductRequest, 0), Total: 100.0, Status: "created"},
	2: {ID: 2, ProductRequests: make([]entity.ProductRequest, 0), Total: 200.0, Status: "paid"},
}

func (r *OrderRepository) GetOrderByID(id int) (*entity.Order, error) {
	order, exists := orders[id]
	if !exists {
		return nil, nil
	}
	return order, nil
}

func (r *OrderRepository) CreateOrder(order *entity.Order) (*entity.Order, error) {
	order.ID = len(orders) + 1
	orders[order.ID] = order
	return order, nil
}

func (r *OrderRepository) UpdateOrder(order *entity.Order) (*entity.Order, error) {
	orders[order.ID] = order
	return order, nil
}

func (r *OrderRepository) DeleteOrder(id int) error {
	delete(orders, id)
	return nil
}
