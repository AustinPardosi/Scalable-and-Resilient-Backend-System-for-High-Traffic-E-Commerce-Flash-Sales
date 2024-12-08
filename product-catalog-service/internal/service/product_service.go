package service

import "fmt"

type ProductService struct {
}

// NewProducyService creates a new instance of ProductService
func NewProductService() *ProductService {
	return &ProductService{}
}

// GetProductStock retrieves the stock for a product
func (p *ProductService) GetProductStock(productID int) (int, error) {
	stock := 50
	return stock, nil
}

// ReserveProductStock reserves stock for an order
func (p *ProductService) ReserveProductStock(productID, quantity int) error {
	fmt.Printf("Reserved %d units of product %d\n", quantity, productID)
	return nil
}

// ReleaseProductStock releases reserved stock when an order is canceled
func (p *ProductService) ReleaseProductStock(productID, quantity int) error {
	fmt.Printf("Released %d units of product %d\n", quantity, productID)
	return nil
}
