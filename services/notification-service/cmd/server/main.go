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

	"github.com/redstone/notification-service/internal/redstone"
)

type Config struct {
	ServiceName string
	HTTPPort string
	KafkaBrokers []string
	GroupID string
	TopicOrders string
	TopicInventory string
	TopicPayments string
	TopicNotifications string
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
		ServiceName: env("SERVICE_NAME","notification-service"),
		HTTPPort: env("HTTP_PORT","8084"),
		KafkaBrokers: parseCSV(env("KAFKA_BROKERS","localhost:9092")),
		GroupID: env("KAFKA_GROUP_ID","notification-service"),
		TopicOrders: env("KAFKA_TOPIC_ORDERS","redstone.orders"),
		TopicInventory: env("KAFKA_TOPIC_INVENTORY","redstone.inventory"),
		TopicPayments: env("KAFKA_TOPIC_PAYMENTS","redstone.payments"),
		TopicNotifications: env("KAFKA_TOPIC_NOTIFICATIONS","redstone.notifications"),
	}
	log := redstone.NewLogger(cfg.ServiceName)

	orders := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicOrders, cfg.GroupID+"-orders")
	inv := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicInventory, cfg.GroupID+"-inventory")
	pay := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicPayments, cfg.GroupID+"-payments")
	defer orders.Close(); defer inv.Close(); defer pay.Close()

	ctx := context.Background()
	go consume(ctx, log, "orders", orders)
	go consume(ctx, log, "inventory", inv)
	go consume(ctx, log, "payments", pay)

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })

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

func consume(ctx context.Context, log *redstone.Logger, stream string, c *redstone.Consumer) {
	for {
		m, err := c.Fetch(ctx)
		if err != nil {
			log.Error("consume fetch failed", map[string]any{"err": err.Error(), "stream": stream})
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var envelope map[string]any
		if json.Unmarshal(m.Value, &envelope) == nil {
			et, _ := envelope["event_type"].(string)
			oid, _ := envelope["order_id"].(string)
			log.Info("notify", map[string]any{"stream": stream, "event_type": et, "order_id": oid})
		}
		_ = c.Commit(ctx, m)
	}
}
