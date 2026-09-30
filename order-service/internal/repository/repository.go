package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"order-service/internal/entity"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

var ErrNotFound = errors.New("order not found")

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (r *OrderRepository) GetOrderByID(ctx context.Context, id int64) (*entity.Order, error) {
	return getOrder(ctx, r.db, `id = ?`, id)
}

func (r *OrderRepository) GetOrderByKey(ctx context.Context, userID int64, idempotencyKey string) (*entity.Order, error) {
	return getOrder(ctx, r.db, `user_id = ? AND idempotency_key = ?`, userID, idempotencyKey)
}

// CreateOrder inserts a PENDING order with its items. If the user already placed an
// order with this idempotency key, it loads that order into order instead.
func (r *OrderRepository) CreateOrder(ctx context.Context, order *entity.Order, idempotencyKey string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var key any // NULL when absent, so orders without a key never collide
	if idempotencyKey != "" {
		key = idempotencyKey
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO orders (user_id, idempotency_key, status, total_cents) VALUES (?, ?, ?, ?)`,
		order.UserID, key, order.Status, order.TotalCents)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		// A concurrent duplicate won the insert (we waited for it to commit): use its order.
		tx.Rollback()
		existing, err := r.GetOrderByKey(ctx, order.UserID, idempotencyKey)
		if err != nil {
			return err
		}
		*order = *existing
		return nil
	}
	if err != nil {
		return err
	}
	if order.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	for _, it := range order.Items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, unit_price_cents) VALUES (?, ?, ?, ?)`,
			order.ID, it.ProductID, it.Quantity, it.UnitPriceCents); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Transition moves an order from one of the from statuses to status `to` and, in the
// same transaction, queues the event describing it in the outbox. Returns false and
// writes nothing if the order was not in a from status, e.g. someone else moved it first.
func (r *OrderRepository) Transition(ctx context.Context, id int64, to, reason, topic string, from ...string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	args := []any{to, reason, id}
	for _, s := range from {
		args = append(args, s)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE orders SET status = ?, cancel_reason = ? WHERE id = ? AND status IN (`+strings.Repeat(",?", len(from))[1:]+`)`,
		args...)
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}

	order, err := getOrder(ctx, tx, `id = ?`, id)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(order)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO outbox (topic, msg_key, payload) VALUES (?, ?, ?)`,
		topic, strconv.FormatInt(id, 10), payload); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// StaleOrders lists orders that have sat in status for longer than age.
func (r *OrderRepository) StaleOrders(ctx context.Context, status string, age time.Duration) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM orders WHERE status = ? AND updated_at < NOW(3) - INTERVAL ? SECOND ORDER BY id LIMIT 100`,
		status, int(age.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func getOrder(ctx context.Context, q querier, where string, args ...any) (*entity.Order, error) {
	var o entity.Order
	err := q.QueryRowContext(ctx,
		`SELECT id, user_id, status, cancel_reason, total_cents, created_at FROM orders WHERE `+where, args...).
		Scan(&o.ID, &o.UserID, &o.Status, &o.CancelReason, &o.TotalCents, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := q.QueryContext(ctx,
		`SELECT product_id, quantity, unit_price_cents FROM order_items WHERE order_id = ? ORDER BY product_id`, o.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it entity.Item
		if err := rows.Scan(&it.ProductID, &it.Quantity, &it.UnitPriceCents); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it)
	}
	return &o, rows.Err()
}
