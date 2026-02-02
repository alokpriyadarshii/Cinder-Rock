package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
		eventID, _ := envelope["event_id"].(string)
		if eventID == "" {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		// event dedupe
		var exists string
		err = a.db.QueryRow(ctx, `select event_id from processed_events where event_id=$1`, eventID).Scan(&exists)
		if err == nil && exists != "" {
			_ = a.consumer.Commit(ctx, m)
			continue
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			a.log.Error("event dedupe lookup failed", map[string]any{"err": err.Error(), "event_id": eventID})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		switch et {
		case "OrderCreated":
			var ev redstone.OrderCreated
			if json.Unmarshal(m.Value, &ev) != nil {
				_ = a.consumer.Commit(ctx, m)
				continue
			}
			ok, reason := a.tryReserve(ctx, ev.OrderID, ev.Items)
			if ok {
				out := redstone.InventoryReserved{
					BaseEvent: redstone.BaseEvent{
						EventID:       uuid.NewString(),
						EventType:     "InventoryReserved",
						OccurredAt:    time.Now().UTC(),
						CorrelationID: ev.CorrelationID,
					},
					OrderID: ev.OrderID,
				}
				_ = a.producer.Write(ctx, ev.OrderID, out)
			} else {
				out := redstone.InventoryFailed{
					BaseEvent: redstone.BaseEvent{
						EventID:       uuid.NewString(),
						EventType:     "InventoryFailed",
						OccurredAt:    time.Now().UTC(),
						CorrelationID: ev.CorrelationID,
					},
					OrderID: ev.OrderID,
					Reason:  reason,
				}
			}
		case "OrderConfirmed":
			var ev redstone.OrderConfirmed
			if json.Unmarshal(m.Value, &ev) == nil {
				if err := a.finalizeReservation(ctx, ev.OrderID); err != nil {
					a.log.Error("finalize reservation failed", map[string]any{"err": err.Error(), "order_id": ev.OrderID})
				}
			}
		case "OrderCancelled":
			var ev redstone.OrderCancelled
			if json.Unmarshal(m.Value, &ev) == nil {
				if err := a.releaseReservation(ctx, ev.OrderID); err != nil {
					a.log.Error("release reservation failed", map[string]any{"err": err.Error(), "order_id": ev.OrderID})
				}
			}
			_ = a.producer.Write(ctx, ev.OrderID, out)
		}

		_, _ = a.db.Exec(ctx, `insert into processed_events(event_id,processed_at) values ($1,now()) on conflict (event_id) do nothing`, eventID)
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


func (a *App) finalizeReservation(ctx context.Context, orderID string) error {
	return a.updateReservation(ctx, orderID, "COMPLETED", true)
}

func (a *App) releaseReservation(ctx context.Context, orderID string) error {
	return a.updateReservation(ctx, orderID, "CANCELLED", false)
}

func (a *App) updateReservation(ctx context.Context, orderID, status string, deductOnHand bool) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `select sku, qty from reservations where order_id=$1 and status='RESERVED' for update`, orderID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var totalRows int
	for rows.Next() {
		var sku string
		var qty int64
		if err := rows.Scan(&sku, &qty); err != nil {
			return err
		}
		totalRows++
		if deductOnHand {
			if _, err := tx.Exec(ctx, `update stock set on_hand = on_hand - $2, reserved = reserved - $2 where sku=$1`, sku, qty); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `update stock set reserved = reserved - $2 where sku=$1`, sku, qty); err != nil {
				return err
			}
		}
	}

	if totalRows == 0 {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}

	if _, err := tx.Exec(ctx, `update reservations set status=$2 where order_id=$1 and status='RESERVED'`, orderID, status); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
