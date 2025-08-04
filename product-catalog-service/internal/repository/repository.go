package repository

import "product-catalog-service/internal/entity"

type ProductRepository struct {
}

func NewProductRepository() *ProductRepository {
	return &ProductRepository{}
}

var products = map[int]*entity.Product{
	1: {ID: 1, Name: "Product A", Description: "Description for Product A", Stock: 100, Price: 10.0},
	2: {ID: 2, Name: "Product B", Description: "Description for Product B", Stock: 50, Price: 20.0},
}

func (r *ProductRepository) GetProductByID(id int) (*entity.Product, error) {
	product, exists := products[id]
	if !exists {
		return nil, nil
	}
	return product, nil
}

func (r *ProductRepository) CreateProduct(product *entity.Product) (*entity.Product, error) {
	product.ID = 3
	products[product.ID] = product
	return product, nil
}

func (r *ProductRepository) UpdateProduct(product *entity.Product) (*entity.Product, error) {
	products[product.ID] = product
	return product, nil
}

func (r *ProductRepository) DeleteProduct(id int) error {
	delete(products, id)
	return nil
}

func (r *ProductRepository) GetProducts() ([]*entity.Product, error) {
	var productList []*entity.Product
	for _, product := range products {
		productList = append(productList, product)
	}
	return productList, nil
}
