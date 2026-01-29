package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	stmts := []string{
		`create table if not exists orders(
			id text primary key,
			user_id text not null,
			status text not null,
			total_amount bigint not null,
			currency text not null,
			created_at timestamptz not null,
			updated_at timestamptz not null
		)`,
		`create table if not exists order_items(
			id bigserial primary key,
			order_id text not null references orders(id) on delete cascade,
			sku text not null,
			qty int not null,
			unit_price bigint not null,
			total_price bigint not null
		)`,
		`create table if not exists order_events(
			id bigserial primary key,
			order_id text not null,
			type text not null,
			payload jsonb not null,
			created_at timestamptz not null
		)`,
		`create table if not exists idempotency(
			idem_key text primary key,
			order_id text not null,
			created_at timestamptz not null
		)`,
		`create table if not exists outbox(
			id bigserial primary key,
			aggregate_id text not null,
			event_type text not null,
			payload jsonb not null,
			status text not null,
			created_at timestamptz not null,
			published_at timestamptz null
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
