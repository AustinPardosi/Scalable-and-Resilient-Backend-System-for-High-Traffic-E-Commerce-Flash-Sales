package entity

type Order struct {
	ID     int `json:"id"`
	UserID int `json:"user_id"`
	ProductID int `json:"product_id"`
	Quantity int `json:"quantity"`
	Total float64 `json:"total"`
	Status string `json:"status"`
}
