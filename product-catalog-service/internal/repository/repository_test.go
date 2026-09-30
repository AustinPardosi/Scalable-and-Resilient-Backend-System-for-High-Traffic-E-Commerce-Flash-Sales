package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"product-catalog-service/internal/entity"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Runs against a real MySQL with schema.sql applied, e.g. the one in docker compose:
// PRODUCTS_TEST_DSN="product_svc:$PRODUCT_DB_PASSWORD@tcp(localhost:3306)/products_db" go test ./...
func setup(t *testing.T, stock int) (*ProductRepository, int64) {
	dsn := os.Getenv("PRODUCTS_TEST_DSN")
	if dsn == "" {
		t.Skip("PRODUCTS_TEST_DSN not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { db.Close() })
	res, err := db.Exec(`INSERT INTO products (name, description, price_cents, stock) VALUES ('test', '', 100, ?)`, stock)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return NewProductRepository(db), id
}

func stockOf(t *testing.T, r *ProductRepository, id int64) int {
	p, err := r.GetProductByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p.Stock
}

// Order IDs unique across test runs against the same database.
var nextOrderID = time.Now().UnixNano()

func newOrderID() int64 { return atomic.AddInt64(&nextOrderID, 1) }

func TestReserveNeverOversells(t *testing.T) {
	const stock, buyers = 50, 300
	r, pid := setup(t, stock)

	var sold atomic.Int32
	var wg sync.WaitGroup
	for range buyers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Reserve(context.Background(), newOrderID(), []entity.Item{{ProductID: pid, Quantity: 1}})
			switch {
			case err == nil:
				sold.Add(1)
			case !errors.Is(err, ErrOutOfStock):
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if sold.Load() != stock {
		t.Fatalf("sold %d, want exactly %d", sold.Load(), stock)
	}
	if got := stockOf(t, r, pid); got != 0 {
		t.Fatalf("stock left %d, want 0", got)
	}
}

func TestReserveAndReleaseAreIdempotent(t *testing.T) {
	ctx := context.Background()
	r, pid := setup(t, 5)
	items := []entity.Item{{ProductID: pid, Quantity: 2}}

	// Retried reserve takes stock once.
	order := newOrderID()
	for range 3 {
		if _, err := r.Reserve(ctx, order, items); err != nil {
			t.Fatal(err)
		}
	}
	if got := stockOf(t, r, pid); got != 3 {
		t.Fatalf("after reserve: stock %d, want 3", got)
	}

	// Redelivered release gives it back once, and the order can't reserve again.
	for range 3 {
		if _, err := r.Release(ctx, order); err != nil {
			t.Fatal(err)
		}
	}
	if got := stockOf(t, r, pid); got != 5 {
		t.Fatalf("after release: stock %d, want 5", got)
	}
	if _, err := r.Reserve(ctx, order, items); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("reserve after release: got %v, want ErrOutOfStock", err)
	}

	// Release that overtakes its reserve blocks the late reserve.
	late := newOrderID()
	if _, err := r.Release(ctx, late); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reserve(ctx, late, items); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("late reserve: got %v, want ErrOutOfStock", err)
	}

	// Out of stock is final for the order even if stock frees up later.
	greedy := newOrderID()
	if _, err := r.Reserve(ctx, greedy, []entity.Item{{ProductID: pid, Quantity: 6}}); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("oversized reserve: got %v, want ErrOutOfStock", err)
	}
	if _, err := r.Reserve(ctx, greedy, items); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("retry after out of stock: got %v, want ErrOutOfStock", err)
	}
	if got := stockOf(t, r, pid); got != 5 {
		t.Fatalf("end: stock %d, want 5", got)
	}
}
