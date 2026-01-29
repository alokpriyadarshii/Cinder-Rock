package redstone

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	w *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		w: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			BatchTimeout: 50 * time.Millisecond,
			RequiredAcks: kafka.RequireAll,
		},
	}
}

func (p *Producer) Close() error { return p.w.Close() }

func (p *Producer) Write(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return p.w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: b,
		Time:  time.Now(),
	})
}

type Consumer struct {
	r *kafka.Reader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			Topic:    topic,
			GroupID:  groupID,
			MinBytes: 1,
			MaxBytes: 10e6,
		}),
	}
}

func (c *Consumer) Close() error { return c.r.Close() }

func (c *Consumer) Fetch(ctx context.Context) (kafka.Message, error) {
	return c.r.FetchMessage(ctx)
}

func (c *Consumer) Commit(ctx context.Context, m kafka.Message) error {
	return c.r.CommitMessages(ctx, m)
}
