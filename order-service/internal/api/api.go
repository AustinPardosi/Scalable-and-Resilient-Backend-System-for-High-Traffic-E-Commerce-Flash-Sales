package api

import (
	"context"
	"errors"
	"order-service/internal/entity"
	"order-service/internal/service"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type OrderHandler struct {
	orderService *service.OrderService
}

func NewOrderHandler(orderService *service.OrderService) *OrderHandler {
	return &OrderHandler{orderService: orderService}
}

// CreateOrder places an order --> POST /orders (optional Idempotency-Key header)
func (h *OrderHandler) CreateOrder(c echo.Context) error {
	userID, ok := callerID(c)
	if !ok {
		return c.JSON(401, map[string]string{"error": "Unauthorized"})
	}
	req := struct {
		Items []struct {
			ProductID int64 `json:"product_id"`
			Quantity  int   `json:"quantity"`
		} `json:"items"`
	}{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	key := c.Request().Header.Get("Idempotency-Key")
	if !validKey(key) {
		return c.JSON(400, map[string]string{"error": "Idempotency-Key must be at most 64 printable ASCII characters"})
	}
	items := make([]entity.Item, len(req.Items))
	for i, it := range req.Items {
		items[i] = entity.Item{ProductID: it.ProductID, Quantity: it.Quantity}
	}

	order, err := h.orderService.CreateOrder(c.Request().Context(), userID, key, items)
	if err != nil {
		return errorResponse(c, err)
	}
	if order.Status == entity.StatusCanceled {
		return c.JSON(409, order) // sold out (or canceled since); the body says which
	}
	return c.JSON(201, order)
}

// GetOrder returns one of the caller's orders --> GET /orders/:id
func (h *OrderHandler) GetOrder(c echo.Context) error {
	return h.withOrder(c, h.orderService.GetOrder)
}

// PayOrder marks a confirmed order as paid --> POST /orders/:id/pay
func (h *OrderHandler) PayOrder(c echo.Context) error {
	return h.withOrder(c, h.orderService.PayOrder)
}

// CancelOrder cancels an unpaid order --> POST /orders/:id/cancel
func (h *OrderHandler) CancelOrder(c echo.Context) error {
	return h.withOrder(c, h.orderService.CancelOrder)
}

func (h *OrderHandler) withOrder(c echo.Context, fn func(ctx context.Context, userID, id int64) (*entity.Order, error)) error {
	userID, ok := callerID(c)
	if !ok {
		return c.JSON(401, map[string]string{"error": "Unauthorized"})
	}
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid order ID"})
	}
	order, err := fn(c.Request().Context(), userID, orderID)
	if err != nil {
		return errorResponse(c, err)
	}
	return c.JSON(200, order)
}

// validKey accepts an absent key or up to 64 printable ASCII characters, which the
// database compares byte for byte.
func validKey(key string) bool {
	if len(key) > 64 {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '!' || key[i] > '~' {
			return false
		}
	}
	return true
}

// callerID reads the caller's ID from the JWT the middleware already verified.
func callerID(c echo.Context) (int64, bool) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return 0, false
	}
	sub, err := token.Claims.GetSubject()
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	return id, err == nil
}

func errorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalid):
		return c.JSON(400, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrConflict):
		return c.JSON(409, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrNotFound):
		return c.JSON(404, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrUnavailable):
		log.Warn().Err(err).Msg("dependency unavailable")
		return c.JSON(503, map[string]string{"error": "temporarily unavailable, retry with the same Idempotency-Key"})
	}
	log.Error().Err(err).Str("path", c.Path()).Msg("request failed")
	return c.JSON(500, map[string]string{"error": "internal error"})
}
