package api

import (
	"order-service/internal/entity"
	"order-service/internal/service"
	"strconv"

	"github.com/labstack/echo/v4"
)

type OrderHandler struct {
	orderService service.OrderService
}

func NewOrderHandler(orderService service.OrderService) *OrderHandler {
	return &OrderHandler{orderService: orderService}
}

func (h *OrderHandler) CreateOrder(c echo.Context) error {
	order := entity.Order{}
	if err := c.Bind(&order); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}

	newOrder, err := h.orderService.CreateOrder(&order)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(200, newOrder)
}

func (h *OrderHandler) UpdateOrder(c echo.Context) error {
	order := entity.Order{}
	if err := c.Bind(&order); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}

	updatedOrder, err := h.orderService.UpdateOrder(&order)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(200, updatedOrder)
}

func (h *OrderHandler) CancelOrder(c echo.Context) error {
	orderID := c.Param("id")
	orderIDInt, err := strconv.Atoi(orderID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid order ID"})
	}

	cancelledOrder, err := h.orderService.CancelOrder(orderIDInt)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(200, cancelledOrder)
}
