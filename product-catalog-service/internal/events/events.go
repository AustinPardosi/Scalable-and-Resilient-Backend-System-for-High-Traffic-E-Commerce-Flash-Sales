package events

import (
	"context"
	"encoding/json"
	"product-catalog-service/internal/entity"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

const (
	TopicStockChanged  = "stock.changed"
	TopicOrderCanceled = "order.canceled"
)

type Publisher struct {
	w *kafka.Writer
}

func NewPublisher(brokers []string) *Publisher {
	return &Publisher{w: &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        TopicStockChanged,
		Balancer:     &kafka.Hash{}, // same product -> same partition -> in order
		BatchTimeout: 10 * time.Millisecond,
		// ponytail: fire-and-forget, off the reservation hot path. Each event carries the
		// absolute stock level, so the next one heals a lost one. Move to an outbox if a
		// consumer ever needs every single event.
		Async: true,
		Completion: func(msgs []kafka.Message, err error) {
			if err != nil {
				log.Error().Err(err).Int("messages", len(msgs)).Msg("publish stock.changed")
			}
		},
	}}
}

// StockChanged announces the new stock level of each product.
func (p *Publisher) StockChanged(products []entity.Product) {
	msgs := make([]kafka.Message, 0, len(products))
	for _, pr := range products {
		value, _ := json.Marshal(pr)
		msgs = append(msgs, kafka.Message{Key: []byte(strconv.FormatInt(pr.ID, 10)), Value: value})
	}
	if len(msgs) > 0 {
		_ = p.w.WriteMessages(context.Background(), msgs...) // async: errors land in Completion
	}
}

func (p *Publisher) Close() error {
	return p.w.Close()
}

// ConsumeOrderCanceled calls release for every canceled order until ctx is done.
// At-least-once: the offset is committed only after release succeeds, so a crash
// means redelivery, and release must be idempotent.
func ConsumeOrderCanceled(ctx context.Context, r *kafka.Reader, release func(context.Context, int64) error) {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Msg("fetch order.canceled")
			if !sleep(ctx, time.Second) {
				return
			}
			continue
		}

		var order struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(msg.Value, &order); err != nil || order.ID == 0 {
			// A poison message would block the partition forever; log it and move on.
			log.Error().Err(err).Bytes("value", msg.Value).Msg("skipping malformed order.canceled")
		} else {
			// Transient failures (DB down) retry the same message, never skip it:
			// skipping would leak the order's stock.
			for {
				err := release(ctx, order.ID)
				if err == nil {
					break
				}
				log.Error().Err(err).Int64("order_id", order.ID).Msg("release stock, retrying")
				if !sleep(ctx, time.Second) {
					return
				}
			}
		}

		if err := r.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("commit order.canceled offset")
		}
	}
}

// sleep waits d, returning false if ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
