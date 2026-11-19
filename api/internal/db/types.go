package db

import "time"

// Product is a catalog row in one workspace.
type Product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceMinor int64  `json:"priceMinor"`
	Currency   string `json:"currency"`
	Stock      int    `json:"stock"`
}

// Line is one product on a request or a draft order.
type Line struct {
	ProductID      string `json:"productId"`
	ProductName    string `json:"productName"`
	Quantity       int    `json:"quantity"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
	Currency       string `json:"currency"`
	LineTotalMinor int64  `json:"lineTotalMinor"`
}

// OrderRequest is a buyer's submission.
type OrderRequest struct {
	ID         string    `json:"id"`
	BuyerName  string    `json:"buyerName"`
	BuyerEmail string    `json:"buyerEmail"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	Currency   string    `json:"currency"`
	TotalMinor int64     `json:"totalMinor"`
	Lines      []Line    `json:"lines"`
}

// DraftOrder is created when sales accepts a request.
type DraftOrder struct {
	ID             string    `json:"id"`
	OrderRequestID string    `json:"orderRequestId"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	Currency       string    `json:"currency"`
	TotalMinor     int64     `json:"totalMinor"`
	Lines          []Line    `json:"lines"`
}

// ListFilter selects a page of requests. A nil Status returns every status.
type ListFilter struct {
	Status *string
	Limit  int
	Offset int
}
