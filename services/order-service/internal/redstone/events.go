package redstone

import "time"

type BaseEvent struct {
	EventID        string    `json:"event_id"`
	EventType      string    `json:"event_type"`
	OccurredAt     time.Time `json:"occurred_at"`
	CorrelationID  string    `json:"correlation_id"`
}

type OrderItem struct {
	SKU       string `json:"sku"`
	Qty       int    `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

type OrderCreated struct {
	BaseEvent
	OrderID string     `json:"order_id"`
	UserID  string     `json:"user_id"`
	Items   []OrderItem `json:"items"`
}

type InventoryReserved struct {
	BaseEvent
	OrderID string `json:"order_id"`
}

type InventoryFailed struct {
	BaseEvent
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

type PaymentCaptured struct {
	BaseEvent
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

type PaymentFailed struct {
	BaseEvent
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

type OrderConfirmed struct {
	BaseEvent
	OrderID string `json:"order_id"`
}

type OrderCancelled struct {
	BaseEvent
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}
