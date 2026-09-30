package events

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

// RunRelay publishes outbox rows to Kafka until ctx is done.
//
// At-least-once: a crash between publishing and marking rows published re-sends
// that batch, so consumers must be idempotent. FOR UPDATE SKIP LOCKED lets several
// order-service replicas relay side by side without sending the same row twice.
func RunRelay(ctx context.Context, db *sql.DB, brokers []string) {
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.Hash{}, // same order -> same partition -> in order
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 10 * time.Millisecond, // default 1s would add a second to every event
	}
	defer w.Close()

	for {
		n, err := relayBatch(ctx, db, w)
		if err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("relay outbox")
		}
		if n == 0 { // idle or failing: back off
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

// ponytail: published rows are kept forever; add a retention delete when the table grows.
func relayBatch(ctx context.Context, db *sql.DB, w *kafka.Writer) (int, error) {
	// READ COMMITTED: under the default REPEATABLE READ this locking read would also lock the
	// gap at the end of the index, where every new outbox row goes, so every order transition
	// would wait on this batch's Kafka write.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT id, topic, msg_key, payload FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	var ids []any
	var msgs []kafka.Message
	for rows.Next() {
		var id int64
		var m kafka.Message
		if err := rows.Scan(&id, &m.Topic, &m.Key, &m.Value); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(msgs) == 0 {
		return 0, err
	}

	// Don't sit on the row locks if Kafka hangs; the batch is simply retried.
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := w.WriteMessages(writeCtx, msgs...); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE outbox SET published_at = NOW(3) WHERE id IN (`+strings.Repeat(",?", len(ids))[1:]+`)`, ids...); err != nil {
		return 0, err
	}
	return len(msgs), tx.Commit()
}
