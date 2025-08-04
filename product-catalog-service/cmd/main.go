package main

import (
	"product-catalog-service/internal/api"
	"product-catalog-service/internal/repository"
	"product-catalog-service/internal/service"

	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize product service
	productRepository := repository.NewProductRepository()
	productService := service.NewProductService(*productRepository)
	productHandler := api.NewProductHandler(*productService)

	// Initialize echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(echojwt.JWT([]byte("secret")))

	// Routes
	e.GET("/products/:id/stock", productHandler.GetProductStock)
	e.POST("/products/reserve", productHandler.ReserveProductStock)
	e.POST("/products/release", productHandler.ReleaseProductStock)

	// Start server
	e.Start(":8081")
}
