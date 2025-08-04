package cmd

import (
	"order-service/internal/api"
	"order-service/internal/repository"
	"order-service/internal/service"

	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {

	// Initialize order service
	orderRepo := repository.NewOrderRepository()
	productServiceURL := "http://product-catalog-service:8081"
	pricingServiceURL := "http://pricing-service:8083"
	orderService := service.NewOrderService(*orderRepo, productServiceURL, pricingServiceURL)
	orderHandler := api.NewOrderHandler(*orderService)

	// Initialize echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(echojwt.JWT([]byte("secret")))

	// Routes
	e.POST("/orders", orderHandler.CreateOrder)
	e.PUT("/orders/:id", orderHandler.UpdateOrder)
	e.DELETE("/orders/:id", orderHandler.CancelOrder)

	// Start server
	e.Start(":8082")
}
