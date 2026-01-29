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

	"github.com/redstone/payment-service/internal/redstone"
)

type Config struct {
	ServiceName string
	HTTPPort string
	KafkaBrokers []string
	TopicInventory string
	TopicPayments string
	GroupID string
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
		ServiceName: env("SERVICE_NAME","payment-service"),
		HTTPPort: env("HTTP_PORT","8083"),
		KafkaBrokers: parseCSV(env("KAFKA_BROKERS","localhost:9092")),
		TopicInventory: env("KAFKA_TOPIC_INVENTORY","redstone.inventory"),
		TopicPayments: env("KAFKA_TOPIC_PAYMENTS","redstone.payments"),
		GroupID: env("KAFKA_GROUP_ID","payment-service"),
	}
	log := redstone.NewLogger(cfg.ServiceName)

	producer := redstone.NewProducer(cfg.KafkaBrokers, cfg.TopicPayments)
	defer producer.Close()

	consumer := redstone.NewConsumer(cfg.KafkaBrokers, cfg.TopicInventory, cfg.GroupID)
	defer consumer.Close()

	app := &App{cfg: cfg, log: log, producer: producer, consumer: consumer}

	ctx := context.Background()
	go app.consumeInventoryLoop(ctx)

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
	r.Get("/v1/mock/provider", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status":"ok"})
	})

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
	producer *redstone.Producer
	consumer *redstone.Consumer
}

func (a *App) consumeInventoryLoop(ctx context.Context) {
	for {
		m, err := a.consumer.Fetch(ctx)
		if err != nil {
			a.log.Error("inventory fetch failed", map[string]any{"err": err.Error()})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal(m.Value, &envelope); err != nil {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		et, _ := envelope["event_type"].(string)
		if et != "InventoryReserved" {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		var ev redstone.InventoryReserved
		if json.Unmarshal(m.Value, &ev) != nil {
			_ = a.consumer.Commit(ctx, m)
			continue
		}

		// Mock payment: succeed most of the time; fail if order_id ends with certain pattern
		if strings.HasSuffix(ev.OrderID, "0") {
			out := redstone.PaymentFailed{
				BaseEvent: redstone.BaseEvent{
					EventID: uuid.NewString(),
					EventType: "PaymentFailed",
					OccurredAt: time.Now().UTC(),
					CorrelationID: ev.CorrelationID,
				},
				OrderID: ev.OrderID,
				Reason: "mock decline",
			}
			_ = a.producer.Write(ctx, ev.OrderID, out)
		} else {
			out := redstone.PaymentCaptured{
				BaseEvent: redstone.BaseEvent{
					EventID: uuid.NewString(),
					EventType: "PaymentCaptured",
					OccurredAt: time.Now().UTC(),
					CorrelationID: ev.CorrelationID,
				},
				OrderID: ev.OrderID,
				Amount: 0,
			}
			_ = a.producer.Write(ctx, ev.OrderID, out)
		}

		_ = a.consumer.Commit(ctx, m)
	}
}
