package entity

import "time"

// Order lifecycle: PENDING -> CONFIRMED -> PAID, and PENDING/CONFIRMED -> CANCELED.
// Unpaid CONFIRMED orders are canceled after 15 minutes so their stock goes back on sale.
const (
	StatusPending   = "PENDING"   // placed, stock not reserved yet
	StatusConfirmed = "CONFIRMED" // stock reserved
	StatusPaid      = "PAID"
	StatusCanceled  = "CANCELED" // see CancelReason: user, out_of_stock, timeout, payment_timeout
)

type Order struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Status       string    `json:"status"`
	CancelReason string    `json:"cancel_reason,omitempty"`
	TotalCents   int64     `json:"total_cents"`
	Items        []Item    `json:"items"`
	CreatedAt    time.Time `json:"created_at"`
}

type Item struct {
	ProductID      int64 `json:"product_id"`
	Quantity       int   `json:"quantity"`
	UnitPriceCents int64 `json:"unit_price_cents"` // price locked in when the order was placed
}
