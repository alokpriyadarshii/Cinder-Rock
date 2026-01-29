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

	"github.com/redstone/inventory-service/internal/redstone"
)

type Config struct {
	ServiceName   string
	HTTPPort      string
	DatabaseURL   string
	KafkaBrokers  []string
	TopicOrders   string
	TopicInventory string
	GroupID       string
}

func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" { return def }
	return v
}

func parseCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" { out = append(out, p) }
	}
	return out
}

func main() {
	cfg := Config{
		ServiceName: env("SERVICE_NAME","inventory-service"),
		HTTPPort: env("HTTP_PORT","8082"),
		DatabaseURL: env("DATABASE_URL",""),
		KafkaBrokers: parseCSV(env("KAFKA_BROKERS","localhost:9092")),
		TopicOrders: env("KAFKA_TOPIC_ORDERS","redstone.orders"),
		TopicInventory: env("KAFKA_TOPIC_INVENTORY","redstone.inventory"),
		GroupID: env("KAFKA_GROUP_ID","inventory-service"),
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
	if err := seed(ctx, db); err != nil {
		log.Error("seed failed", map[string]any{"err": err.Error()})
		os.Exit(1)
	}

	producer := redstone.NewProducer(cfg.KafkaBrokers, cfg.TopicInventory)
	defer producer.Close()

	consumer := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicOrders, cfg.GroupID)
	defer consumer.Close()

	app := &App{cfg: cfg, log: log, db: db, producer: producer, consumer: consumer}

	go app.consumeOrdersLoop(ctx)

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
	r.Get("/v1/stock/{sku}", app.getStockHandler)

	srv := &http.Server{
		Addr: ":" + cfg.HTTPPort,
		Handler: r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("http server starting", map[string]any{"port": cfg.HTTPPort})
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http server error", map[string]any{"err": err.Error()})
	}
}

type App struct {
	cfg Config
	log *redstone.Logger
	db *pgxpool.Pool
	producer *redstone.Producer
	consumer *redstone.Consumer
}

func (a *App) getStockHandler(w http.ResponseWriter, r *http.Request) {
	sku := chi.URLParam(r, "sku")
	var onHand, reserved int64
	err := a.db.QueryRow(r.Context(), `select on_hand,reserved from stock where sku=$1`, sku).Scan(&onHand, &reserved)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sku": sku, "on_hand": onHand, "reserved": reserved})
}

var _ = uuid.NewString
