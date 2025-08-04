package main

import (
	"product-catalog-service/internal/api"
	"product-catalog-service/internal/service"

	"github.com/labstack/echo/v4"
)

func main() {
	// Initialize product service
	productService := service.NewProductService()
	productHandler := api.NewProductHandler(*productService)

	// Initialize echo
	e := echo.New()

	// Routes
	e.GET("/products/:id/stock", productHandler.GetProductStock)
	e.POST("/products/reserve", productHandler.ReserveProductStock)
	e.POST("/products/release", productHandler.ReleaseProductStock)

	// Start server
	e.Start(":8081")
}
