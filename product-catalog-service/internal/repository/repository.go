package repository

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"product-catalog-service/internal/entity"
	"slices"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrNotFound   = errors.New("product not found")
	ErrOutOfStock = errors.New("out of stock")
)

type ProductRepository struct {
	db *sql.DB
}

func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (r *ProductRepository) GetProductByID(ctx context.Context, id int64) (*entity.Product, error) {
	products, err := getProducts(ctx, r.db, id)
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, ErrNotFound
	}
	return &products[0], nil
}

func (r *ProductRepository) GetProducts(ctx context.Context) ([]entity.Product, error) {
	return getProducts(ctx, r.db)
}

// getProducts loads the given products, or all of them when no ids are passed.
func getProducts(ctx context.Context, q querier, ids ...int64) ([]entity.Product, error) {
	query := `SELECT id, name, description, price_cents, stock FROM products`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if len(ids) > 0 {
		query += ` WHERE id IN (` + strings.Repeat(",?", len(ids))[1:] + `)`
	}
	rows, err := q.QueryContext(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products := []entity.Product{}
	for rows.Next() {
		var p entity.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Stock); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// Reserve takes stock for an order, all items or none. It is idempotent per order:
// retries and concurrent duplicates all get the same answer, decided by the
// order's reservation row. Returns the products whose stock changed (nil if this
// call changed nothing).
func (r *ProductRepository) Reserve(ctx context.Context, orderID int64, items []entity.Item) ([]entity.Product, error) {
	changed, err := r.tryReserve(ctx, orderID, items)
	if err == nil {
		return changed, nil
	}
	if !errors.Is(err, ErrOutOfStock) && !isDuplicateKey(err) {
		return nil, err
	}
	if errors.Is(err, ErrOutOfStock) {
		// Make "no" final for this order, so a retry can't grab stock that frees up later.
		// If a concurrent request for the same order already reserved, this is a no-op
		// and the read below reports success.
		if _, err := r.db.ExecContext(ctx,
			`INSERT IGNORE INTO reservations (order_id, status) VALUES (?, 'RELEASED')`, orderID); err != nil {
			return nil, err
		}
	}
	var status string
	if err := r.db.QueryRowContext(ctx,
		`SELECT status FROM reservations WHERE order_id = ?`, orderID).Scan(&status); err != nil {
		return nil, err
	}
	if status == "RESERVED" {
		return nil, nil
	}
	return nil, ErrOutOfStock
}

func (r *ProductRepository) tryReserve(ctx context.Context, orderID int64, items []entity.Item) ([]entity.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Claims the order. A concurrent duplicate blocks here until we commit or roll back.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO reservations (order_id, status) VALUES (?, 'RESERVED')`, orderID); err != nil {
		return nil, err
	}

	// Lock rows in id order so two multi-item orders can't deadlock each other.
	items = slices.Clone(items)
	slices.SortFunc(items, func(a, b entity.Item) int { return cmp.Compare(a.ProductID, b.ProductID) })
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ProductID
		// The whole anti-oversell trick: check and decrement in one atomic statement.
		res, err := tx.ExecContext(ctx,
			`UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?`,
			it.Quantity, it.ProductID, it.Quantity)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrOutOfStock
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO reservation_items (order_id, product_id, quantity) VALUES (?, ?, ?)`,
			orderID, it.ProductID, it.Quantity); err != nil {
			return nil, err
		}
	}

	changed, err := getProducts(ctx, tx, ids...)
	if err != nil {
		return nil, err
	}
	return changed, tx.Commit()
}

// Release returns an order's reserved stock. Safe to call any number of times, and
// before Reserve: it then leaves a RELEASED marker that blocks the late reserve.
func (r *ProductRepository) Release(ctx context.Context, orderID int64) ([]entity.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT IGNORE INTO reservations (order_id, status) VALUES (?, 'RELEASED')`, orderID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil, tx.Commit() // nothing was reserved
	}

	var status string
	if err := tx.QueryRowContext(ctx,
		`SELECT status FROM reservations WHERE order_id = ? FOR UPDATE`, orderID).Scan(&status); err != nil {
		return nil, err
	}
	if status == "RELEASED" {
		return nil, tx.Commit() // already released, e.g. a redelivered event
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT product_id, quantity FROM reservation_items WHERE order_id = ? ORDER BY product_id`, orderID)
	if err != nil {
		return nil, err
	}
	var items []entity.Item
	for rows.Next() {
		var it entity.Item
		if err := rows.Scan(&it.ProductID, &it.Quantity); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ProductID
		if _, err := tx.ExecContext(ctx,
			`UPDATE products SET stock = stock + ? WHERE id = ?`, it.Quantity, it.ProductID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE reservations SET status = 'RELEASED' WHERE order_id = ?`, orderID); err != nil {
		return nil, err
	}

	changed, err := getProducts(ctx, tx, ids...)
	if err != nil {
		return nil, err
	}
	return changed, tx.Commit()
}

func isDuplicateKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
