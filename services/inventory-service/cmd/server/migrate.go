package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	stmts := []string{
		`create table if not exists stock(
			sku text primary key,
			on_hand bigint not null,
			reserved bigint not null
		)`,
		`create table if not exists reservations(
			id bigserial primary key,
			order_id text not null,
			sku text not null,
			qty bigint not null,
			status text not null,
			created_at timestamptz not null
		)`,
		`create table if not exists processed_events(
			event_id text primary key,
			processed_at timestamptz not null
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func seed(ctx context.Context, db *pgxpool.Pool) error {
	// Seed some stock if not exists
	_, err := db.Exec(ctx, `insert into stock(sku,on_hand,reserved) values 
		('SKU-RED-1', 100, 0),
		('SKU-RED-2', 50, 0)
		on conflict (sku) do nothing`)
	return err
}
