package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/redstone/order-service/internal/redstone"
)

type outboxRow struct {
	ID          int64
	AggregateID string
	EventType   string
	Payload     []byte
}

func (a *App) outboxLoop(ctx context.Context) {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.drainOutbox(ctx)
		}
	}
}

func (a *App) drainOutbox(ctx context.Context) {
	rows, err := a.db.Query(ctx, `select id,aggregate_id,event_type,payload from outbox where status='PENDING' order by id asc limit 50`)
	if err != nil {
		a.log.Error("outbox query failed", map[string]any{"err": err.Error()})
		return
	}
	defer rows.Close()

	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.AggregateID, &r.EventType, &r.Payload); err == nil {
			batch = append(batch, r)
		}
	}

	for _, r := range batch {
		// publish to orders topic (key = aggregate/order id)
		var anyPayload any
		_ = json.Unmarshal(r.Payload, &anyPayload) // just for safety; payload already JSON
		if err := a.ordersProducer.Write(ctx, r.AggregateID, anyPayload); err != nil {
			a.log.Error("outbox publish failed", map[string]any{"err": err.Error(), "id": r.ID})
			continue
		}
		_, _ = a.db.Exec(ctx, `update outbox set status='PUBLISHED', published_at=now() where id=$1`, r.ID)
	}
}

// Helper to update order status idempotently based on event type.
func updateOrderStatus(ctx context.Context, db *pgxpool.Pool, log *redstone.Logger, orderID, newStatus string, eventType string, payload []byte) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var current string
	if err := tx.QueryRow(ctx, `select status from orders where id=$1`, orderID).Scan(&current); err != nil {
		return err
	}
	if current == newStatus {
		// idempotent
		_ = tx.Commit(ctx)
		return nil
	}

	_, err = tx.Exec(ctx, `update orders set status=$2, updated_at=now() where id=$1`, orderID, newStatus)
	if err != nil {
		return err
	}

	_, _ = tx.Exec(ctx, `insert into order_events(order_id,type,payload,created_at) values ($1,$2,$3,now())`, orderID, eventType, payload)

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Info("order status updated", map[string]any{"order_id": orderID, "status": newStatus, "event": eventType})
	return nil
}

var _ = pgx.ErrNoRows
