package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/redstone/order-service/internal/redstone"
)

func (a *App) consumeInventoryLoop(ctx context.Context) {
	for {
		m, err := a.invConsumer.Fetch(ctx)
		if err != nil {
			a.log.Error("inventory fetch failed", map[string]any{"err": err.Error()})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal(m.Value, &envelope); err != nil {
			_ = a.invConsumer.Commit(ctx, m)
			continue
		}

		// Determine event type
		et, _ := envelope["event_type"].(string)
		switch et {
		case "InventoryReserved":
			var ev redstone.InventoryReserved
			if json.Unmarshal(m.Value, &ev) == nil {
				_ = updateOrderStatus(ctx, a.db, a.log, ev.OrderID, "INVENTORY_RESERVED", "InventoryReserved", m.Value)
			}
		case "InventoryFailed":
			var ev redstone.InventoryFailed
			if json.Unmarshal(m.Value, &ev) == nil {
				_ = updateOrderStatus(ctx, a.db, a.log, ev.OrderID, "CANCELLED", "InventoryFailed", m.Value)
				// Emit OrderCancelled for notifications
				cancel := redstone.OrderCancelled{
					BaseEvent: redstone.BaseEvent{
						EventID: uuid.NewString(),
						EventType: "OrderCancelled",
						OccurredAt: time.Now().UTC(),
						CorrelationID: ev.CorrelationID,
					},
					OrderID: ev.OrderID,
					Reason: ev.Reason,
				}
				_ = a.ordersProducer.Write(ctx, ev.OrderID, cancel)
			}
		}

		_ = a.invConsumer.Commit(ctx, m)
	}
}

func (a *App) consumePaymentLoop(ctx context.Context) {
	for {
		m, err := a.payConsumer.Fetch(ctx)
		if err != nil {
			a.log.Error("payment fetch failed", map[string]any{"err": err.Error()})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal(m.Value, &envelope); err != nil {
			_ = a.payConsumer.Commit(ctx, m)
			continue
		}

		et, _ := envelope["event_type"].(string)
		switch et {
		case "PaymentCaptured":
			var ev redstone.PaymentCaptured
			if json.Unmarshal(m.Value, &ev) == nil {
				_ = updateOrderStatus(ctx, a.db, a.log, ev.OrderID, "PAID", "PaymentCaptured", m.Value)
				// Confirm order
				confirm := redstone.OrderConfirmed{
					BaseEvent: redstone.BaseEvent{
						EventID: uuid.NewString(),
						EventType: "OrderConfirmed",
						OccurredAt: time.Now().UTC(),
						CorrelationID: ev.CorrelationID,
					},
					OrderID: ev.OrderID,
				}
				_ = a.ordersProducer.Write(ctx, ev.OrderID, confirm)
				_ = updateOrderStatus(ctx, a.db, a.log, ev.OrderID, "CONFIRMED", "OrderConfirmed", mustJSON(confirm))
			}
		case "PaymentFailed":
			var ev redstone.PaymentFailed
			if json.Unmarshal(m.Value, &ev) == nil {
				_ = updateOrderStatus(ctx, a.db, a.log, ev.OrderID, "CANCELLED", "PaymentFailed", m.Value)
				cancel := redstone.OrderCancelled{
					BaseEvent: redstone.BaseEvent{
						EventID: uuid.NewString(),
						EventType: "OrderCancelled",
						OccurredAt: time.Now().UTC(),
						CorrelationID: ev.CorrelationID,
					},
					OrderID: ev.OrderID,
					Reason: ev.Reason,
				}
				_ = a.ordersProducer.Write(ctx, ev.OrderID, cancel)
			}
		}

		_ = a.payConsumer.Commit(ctx, m)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var _ = uuid.NewString
