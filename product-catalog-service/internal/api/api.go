package api

import (
	"product-catalog-service/internal/service"
	"strconv"

	"github.com/labstack/echo/v4"
)

type ProductHandler struct {
	productService service.ProductService
}

func NewProductHandler(productService service.ProductService) *ProductHandler {
	return &ProductHandler{productService: productService}
}

// GetProductStock gets the stock of a product by its ID --> /products/:id/stock
func (ph *ProductHandler) GetProductStock(c echo.Context) error {
	productID := c.Param("id")
	productIDInt, err := strconv.Atoi(productID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "Invalid product ID"})
	}
	stock, err := ph.productService.GetProductStock(productIDInt)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]int{"stock": stock})
}

// ReserveProductStock reserves stock for a product --> /products/reserve
func (ph *ProductHandler) ReserveProductStock(c echo.Context) error {
	reservation := struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}{}
	if err := c.Bind(&reservation); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	err := ph.productService.ReserveProductStock(reservation.ProductID, reservation.Quantity)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]string{"message": "Stock reserved successfully"})
}

// ReleaseProductStock releases reserved stock for a product --> /products/release
func (ph *ProductHandler) ReleaseProductStock(c echo.Context) error {
	release := struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}{}
	if err := c.Bind(&release); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	err := ph.productService.ReleaseProductStock(release.ProductID, release.Quantity)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]string{"message": "Stock released successfully"})
}
