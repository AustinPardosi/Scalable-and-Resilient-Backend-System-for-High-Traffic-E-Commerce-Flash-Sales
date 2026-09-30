package service

import (
	"context"
	"product-catalog-service/internal/entity"
	"product-catalog-service/internal/events"
	"product-catalog-service/internal/repository"
)

type ProductService struct {
	productRepo *repository.ProductRepository
	publisher   *events.Publisher
}

// NewProductService creates a new instance of ProductService
func NewProductService(repo *repository.ProductRepository, publisher *events.Publisher) *ProductService {
	return &ProductService{productRepo: repo, publisher: publisher}
}

func (p *ProductService) GetProducts(ctx context.Context) ([]entity.Product, error) {
	return p.productRepo.GetProducts(ctx)
}

func (p *ProductService) GetProductByID(ctx context.Context, id int64) (*entity.Product, error) {
	return p.productRepo.GetProductByID(ctx, id)
}

// Reserve takes stock for an order (all items or none) and announces the new levels
func (p *ProductService) Reserve(ctx context.Context, orderID int64, items []entity.Item) error {
	changed, err := p.productRepo.Reserve(ctx, orderID, items)
	if err != nil {
		return err
	}
	p.publisher.StockChanged(changed)
	return nil
}

// Release gives a canceled order's stock back and announces the new levels
func (p *ProductService) Release(ctx context.Context, orderID int64) error {
	changed, err := p.productRepo.Release(ctx, orderID)
	if err != nil {
		return err
	}
	p.publisher.StockChanged(changed)
	return nil
}
