package cmd

import (
	"order-service/internal/api"
	"order-service/internal/service"

	"github.com/labstack/echo/v4"
)

func main() {

	// Initialize order service
	orderService := service.NewOrderService()
	orderHandler := api.NewOrderHandler(*orderService)

	// Initialize echo
	e := echo.New()

	// Routes
	e.POST("/orders", orderHandler.CreateOrder)
	e.PUT("/orders/:id", orderHandler.UpdateOrder)
	e.DELETE("/orders/:id", orderHandler.CancelOrder)

	// Start server
	e.Start(":8082")
}
