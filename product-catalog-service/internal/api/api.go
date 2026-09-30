package api

import (
	"errors"
	"product-catalog-service/internal/entity"
	"product-catalog-service/internal/repository"
	"product-catalog-service/internal/service"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type ProductHandler struct {
	productService *service.ProductService
}

func NewProductHandler(productService *service.ProductService) *ProductHandler {
	return &ProductHandler{productService: productService}
}

// GetProducts lists the catalog --> /products
func (ph *ProductHandler) GetProducts(c echo.Context) error {
	products, err := ph.productService.GetProducts(c.Request().Context())
	if err != nil {
		return internalError(c, err)
	}
	return c.JSON(200, products)
}

// GetProduct gets one product, including its current stock --> /products/:id
func (ph *ProductHandler) GetProduct(c echo.Context) error {
	productID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid product ID"})
	}
	product, err := ph.productService.GetProductByID(c.Request().Context(), productID)
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(404, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return internalError(c, err)
	}
	return c.JSON(200, product)
}

// Reserve takes stock for an order, all items or none --> /internal/reservations
func (ph *ProductHandler) Reserve(c echo.Context) error {
	req := struct {
		OrderID int64         `json:"order_id"`
		Items   []entity.Item `json:"items"`
	}{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	if req.OrderID <= 0 || len(req.Items) == 0 {
		return c.JSON(400, map[string]string{"error": "order_id and items are required"})
	}
	seen := map[int64]bool{}
	for _, it := range req.Items {
		// A negative quantity would turn the stock decrement into an increment.
		if it.Quantity <= 0 || seen[it.ProductID] {
			return c.JSON(400, map[string]string{"error": "each product once, with a positive quantity"})
		}
		seen[it.ProductID] = true
	}

	err := ph.productService.Reserve(c.Request().Context(), req.OrderID, req.Items)
	if errors.Is(err, repository.ErrOutOfStock) {
		return c.JSON(409, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return internalError(c, err)
	}
	return c.JSON(200, map[string]string{"message": "Stock reserved successfully"})
}

func internalError(c echo.Context, err error) error {
	log.Error().Err(err).Str("path", c.Path()).Msg("request failed")
	return c.JSON(500, map[string]string{"error": "internal error"})
}
