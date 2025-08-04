package service

import (
	"fmt"
	"os"
	"product-catalog-service/internal/repository"

	"github.com/rs/zerolog"
)

var logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

type ProductService struct {
	productRepo repository.ProductRepository
}

// NewProductService creates a new instance of ProductService
func NewProductService(repo repository.ProductRepository) *ProductService {
	return &ProductService{productRepo: repo}
}

// GetProductStock retrieves the stock for a product
func (p *ProductService) GetProductStock(productID int) (int, error) {
	product, err := p.productRepo.GetProductByID(productID)
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving product by ID %d", productID)
		return 0, err
	}
	return product.Stock, nil
}

// ReserveProductStock reserves stock for an order
func (p *ProductService) ReserveProductStock(productID, quantity int) error {
	product, err := p.productRepo.GetProductByID(productID)
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving product by ID %d", productID)
		return err
	}
	if product.Stock < quantity {
		logger.Warn().Msgf("Insufficient stock for product ID %d: requested %d, available %d", productID, quantity, product.Stock)
		return fmt.Errorf("product out of stock")
	}
	product.Stock -= quantity
	_, err = p.productRepo.UpdateProduct(product)
	if err != nil {
		logger.Error().Err(err).Msgf("Error updating product stock for ID %d", productID)
		return err
	}
	return nil
}

// ReleaseProductStock releases reserved stock when an order is canceled
func (p *ProductService) ReleaseProductStock(productID, quantity int) error {
	product, err := p.productRepo.GetProductByID(productID)
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving product by ID %d", productID)
		return err
	}
	product.Stock += quantity
	_, err = p.productRepo.UpdateProduct(product)
	if err != nil {
		logger.Error().Err(err).Msgf("Error updating product stock for ID %d", productID)
		return err
	}
	return nil
}
