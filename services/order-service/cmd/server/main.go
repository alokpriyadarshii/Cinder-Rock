package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/redstone/order-service/internal/redstone"
)

type Config struct {
	ServiceName         string
	HTTPPort            string
	DatabaseURL         string
	KafkaBrokers        []string
	TopicOrders         string
	TopicInventory       string
	TopicPayments        string
	GroupID             string
}

type CreateOrderRequest struct {
	UserID string `json:"user_id"`
	Items  []struct {
		SKU       string `json:"sku"`
		Qty       int    `json:"qty"`
		UnitPrice int64  `json:"unit_price"`
	} `json:"items"`
}

func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func parseCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	cfg := Config{
		ServiceName:   env("SERVICE_NAME", "order-service"),
		HTTPPort:      env("HTTP_PORT", "8081"),
		DatabaseURL:   env("DATABASE_URL", ""),
		KafkaBrokers:  parseCSV(env("KAFKA_BROKERS", "localhost:9092")),
		TopicOrders:   env("KAFKA_TOPIC_ORDERS", "redstone.orders"),
		TopicInventory: env("KAFKA_TOPIC_INVENTORY", "redstone.inventory"),
		TopicPayments: env("KAFKA_TOPIC_PAYMENTS", "redstone.payments"),
		GroupID:       env("KAFKA_GROUP_ID", "order-service"),
	}

	log := redstone.NewLogger(cfg.ServiceName)

	if cfg.DatabaseURL == "" {
		log.Error("DATABASE_URL is required", nil)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect failed", map[string]any{"err": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate(ctx, db); err != nil {
		log.Error("migration failed", map[string]any{"err": err.Error()})
		os.Exit(1)
	}

	ordersProducer := redstone.NewProducer(cfg.KafkaBrokers, cfg.TopicOrders)
	defer ordersProducer.Close()

	// Consumers for saga results
	invConsumer := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicInventory, cfg.GroupID)
	payConsumer := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicPayments, cfg.GroupID)
	defer invConsumer.Close()
	defer payConsumer.Close()

	app := &App{cfg: cfg, log: log, db: db, ordersProducer: ordersProducer, invConsumer: invConsumer, payConsumer: payConsumer}

	go app.outboxLoop(ctx)
	go app.consumeInventoryLoop(ctx)
	go app.consumePaymentLoop(ctx)

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
	r.Post("/v1/orders", app.createOrderHandler)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("http server starting", map[string]any{"port": cfg.HTTPPort})
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http server error", map[string]any{"err": err.Error()})
	}
}

type App struct {
	cfg            Config
	log            *redstone.Logger
	db             *pgxpool.Pool
	ordersProducer *redstone.Producer
	invConsumer    *redstone.Consumer
	payConsumer    *redstone.Consumer
}

func (a *App) createOrderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idem == "" {
		http.Error(w, "missing Idempotency-Key", 400)
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	if req.UserID == "" || len(req.Items) == 0 {
		http.Error(w, "user_id and items required", 400)
		return
	}

	// compute totals
	var total int64
	items := make([]redstone.OrderItem, 0, len(req.Items))
	for _, it := range req.Items {
		if it.SKU == "" || it.Qty <= 0 || it.UnitPrice < 0 {
			http.Error(w, "invalid item", 400)
			return
		}
		total += int64(it.Qty) * it.UnitPrice
		items = append(items, redstone.OrderItem{SKU: it.SKU, Qty: it.Qty, UnitPrice: it.UnitPrice})
	}

	// Idempotency: return existing order if idem key already used
	var existingID string
	err := a.db.QueryRow(ctx, `select order_id from idempotency where idem_key=$1`, idem).Scan(&existingID)
	if err == nil && existingID != "" {
		ord, err := a.getOrder(ctx, existingID)
		if err != nil {
			http.Error(w, "failed to load order", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(ord)
		return
	}

	orderID := "ord_" + uuid.NewString()
	corr := uuid.NewString()

	tx, err := a.db.Begin(ctx)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `insert into orders(id,user_id,status,total_amount,currency,created_at,updated_at) values ($1,$2,$3,$4,$5,now(),now())`,
		orderID, req.UserID, "PENDING", total, "INR")
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	for _, it := range items {
		_, err = tx.Exec(ctx, `insert into order_items(order_id,sku,qty,unit_price,total_price) values ($1,$2,$3,$4,$5)`,
			orderID, it.SKU, it.Qty, it.UnitPrice, int64(it.Qty)*it.UnitPrice)
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
	}

	// store idempotency mapping
	_, _ = tx.Exec(ctx, `insert into idempotency(idem_key,order_id,created_at) values ($1,$2,now()) on conflict (idem_key) do nothing`, idem, orderID)

	// Outbox event
	ev := redstone.OrderCreated{
		BaseEvent: redstone.BaseEvent{
			EventID:       uuid.NewString(),
			EventType:     "OrderCreated",
			OccurredAt:    time.Now().UTC(),
			CorrelationID: corr,
		},
		OrderID: orderID,
		UserID:  req.UserID,
		Items:   items,
	}
	b, _ := json.Marshal(ev)
	_, err = tx.Exec(ctx, `insert into outbox(aggregate_id,event_type,payload,status,created_at) values ($1,$2,$3,'PENDING',now())`,
		orderID, "OrderCreated", b)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}

	// Audit timeline
	_, _ = tx.Exec(ctx, `insert into order_events(order_id,type,payload,created_at) values ($1,$2,$3,now())`,
		orderID, "OrderCreated", b)

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "db error", 500)
		return
	}

	ord, _ := a.getOrder(ctx, orderID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	_ = json.NewEncoder(w).Encode(ord)
}

type Order struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	TotalAmount int64  `json:"total_amount"`
	Currency    string `json:"currency"`
}

func (a *App) getOrder(ctx context.Context, id string) (*Order, error) {
	var o Order
	err := a.db.QueryRow(ctx, `select id,user_id,status,total_amount,currency from orders where id=$1`, id).
		Scan(&o.ID, &o.UserID, &o.Status, &o.TotalAmount, &o.Currency)
	if err != nil {
		return nil, err
	}
	return &o, nil
}
