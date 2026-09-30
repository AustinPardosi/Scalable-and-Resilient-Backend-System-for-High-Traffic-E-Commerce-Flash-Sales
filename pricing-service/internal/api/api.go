package api

import (
	"errors"
	"pricing-service/internal/service"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type PricingHandler struct {
	pricingService *service.PricingService
}

func NewPricingHandler(pricingService *service.PricingService) *PricingHandler {
	return &PricingHandler{pricingService: pricingService}
}

// GetPrice returns a product's current dynamic price --> /prices/:id
func (h *PricingHandler) GetPrice(c echo.Context) error {
	productID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid product ID"})
	}
	price, err := h.pricingService.GetPrice(c.Request().Context(), productID)
	if errors.Is(err, service.ErrNotFound) {
		return c.JSON(404, map[string]string{"error": err.Error()})
	}
	if err != nil {
		log.Error().Err(err).Int64("product_id", productID).Msg("get price")
		return c.JSON(503, map[string]string{"error": "price temporarily unavailable"})
	}
	return c.JSON(200, price)
}
