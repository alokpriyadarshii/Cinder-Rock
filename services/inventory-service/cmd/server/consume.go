package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/redstone/inventory-service/internal/redstone"
)

func (a *App) consumeOrdersLoop(ctx context.Context) {
	for {
		m, err := a.consumer.Fetch(ctx)
		if err != nil {
			a.log.Error("orders fetch failed", map[string]any{"err": err.Error()})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal(m.Value, &envelope); err != nil {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		et, _ := envelope["event_type"].(string)
		if et != "OrderCreated" {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		var ev redstone.OrderCreated
		if json.Unmarshal(m.Value, &ev) != nil {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		// event dedupe
		var exists string
		err = a.db.QueryRow(ctx, `select event_id from processed_events where event_id=$1`, ev.EventID).Scan(&exists)
		if err == nil && exists != "" {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		ok, reason := a.tryReserve(ctx, ev.OrderID, ev.Items)
		if ok {
			out := redstone.InventoryReserved{
				BaseEvent: redstone.BaseEvent{
					EventID: uuid.NewString(),
					EventType: "InventoryReserved",
					OccurredAt: time.Now().UTC(),
					CorrelationID: ev.CorrelationID,
				},
				OrderID: ev.OrderID,
			}
			_ = a.producer.Write(ctx, ev.OrderID, out)
		} else {
			out := redstone.InventoryFailed{
				BaseEvent: redstone.BaseEvent{
					EventID: uuid.NewString(),
					EventType: "InventoryFailed",
					OccurredAt: time.Now().UTC(),
					CorrelationID: ev.CorrelationID,
				},
				OrderID: ev.OrderID,
				Reason: reason,
			}
			_ = a.producer.Write(ctx, ev.OrderID, out)
		}

		_, _ = a.db.Exec(ctx, `insert into processed_events(event_id,processed_at) values ($1,now()) on conflict (event_id) do nothing`, ev.EventID)
		_ = a.consumer.Commit(ctx, m)
	}
}

func (a *App) tryReserve(ctx context.Context, orderID string, items []redstone.OrderItem) (bool, string) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, "db error"
	}
	defer tx.Rollback(ctx)

	for _, it := range items {
		var onHand, reserved int64
		if err := tx.QueryRow(ctx, `select on_hand,reserved from stock where sku=$1 for update`, it.SKU).Scan(&onHand, &reserved); err != nil {
			return false, "sku not found: " + it.SKU
		}
		available := onHand - reserved
		if available < int64(it.Qty) {
			return false, "insufficient stock for " + it.SKU
		}
		_, err := tx.Exec(ctx, `update stock set reserved = reserved + $2 where sku=$1`, it.SKU, int64(it.Qty))
		if err != nil {
			return false, "db error"
		}
		_, _ = tx.Exec(ctx, `insert into reservations(order_id,sku,qty,status,created_at) values ($1,$2,$3,'RESERVED',now())`,
			orderID, it.SKU, int64(it.Qty))
	}

	if err := tx.Commit(ctx); err != nil {
		return false, "db error"
	}
	a.log.Info("inventory reserved", map[string]any{"order_id": orderID})
	return true, ""
}
