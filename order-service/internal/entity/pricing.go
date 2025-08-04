package entity

type Pricing struct {
	ProductID  int     `json:"product_id"`
	MarkUp     float64 `json:"mark_up"`     // Percentage markup on the base price
	Discount   float64 `json:"discount"`    // Percentage discount on the base price
	FinalPrice float64 `json:"final_price"` // Final price after applying markup and discount
}
