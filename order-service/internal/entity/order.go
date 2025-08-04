package entity

type Order struct {
	ID              int              `json:"id"`
	UserID          int              `json:"user_id"`
	ProductRequests []ProductRequest `json:"product_requests"`
	Quantity        int              `json:"quantity"`
	Total           float64          `json:"total"`
	TotalMarkUp     float64          `json:"total_mark_up"`
	TotalDiscount   float64          `json:"total_discount"`
	Status          string           `json:"status"`
}

type ProductRequest struct {
	ProductID  int     `json:"product_id"`
	Quantity   int     `json:"quantity"`
	MarkUp     float64 `json:"mark_up"`
	Discount   float64 `json:"discount"`
	FinalPrice float64 `json:"final_price"`
}
