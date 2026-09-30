package entity

type Product struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceCents  int64  `json:"price_cents"`
	Stock       int    `json:"stock"`
}

// Item is one line of a reservation.
type Item struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}
