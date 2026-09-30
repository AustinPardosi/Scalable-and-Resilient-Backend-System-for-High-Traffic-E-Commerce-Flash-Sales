package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"order-service/internal/entity"
	"order-service/internal/repository"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Runs against a real MySQL with schema.sql applied, e.g. the one in docker compose:
// ORDERS_TEST_DSN="order_svc:$ORDER_DB_PASSWORD@tcp(localhost:3306)/orders_db?parseTime=true" go test ./...
// The pricing and product services are faked; reserveCode sets what reserve answers.
func setup(t *testing.T) (*OrderService, *sql.DB, *atomic.Int32) {
	dsn := os.Getenv("ORDERS_TEST_DSN")
	if dsn == "" {
		t.Skip("ORDERS_TEST_DSN not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"price_cents": 1500}`))
	}))
	var reserveCode atomic.Int32
	reserveCode.Store(200)
	product := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(reserveCode.Load()))
	}))
	t.Cleanup(pricing.Close)
	t.Cleanup(product.Close)

	return NewOrderService(repository.NewOrderRepository(db), product.URL, pricing.URL, "token"), db, &reserveCode
}

func outboxTopics(t *testing.T, db *sql.DB, orderID int64) []string {
	rows, err := db.Query(`SELECT topic FROM outbox WHERE msg_key = ? ORDER BY id`, strconv.FormatInt(orderID, 10))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var topics []string
	for rows.Next() {
		var topic string
		rows.Scan(&topic)
		topics = append(topics, topic)
	}
	return topics
}

func expect(t *testing.T, order *entity.Order, err error, status string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != status {
		t.Fatalf("order %d is %s (%s), want %s", order.ID, order.Status, order.CancelReason, status)
	}
}

// User IDs unique across test runs against the same database.
var nextUserID = time.Now().UnixNano()

func TestOrderLifecycle(t *testing.T) {
	s, db, reserveCode := setup(t)
	ctx := context.Background()
	user := atomic.AddInt64(&nextUserID, 1)
	// The client's price is ignored; the fake pricing service says 1500.
	items := []entity.Item{{ProductID: 1, Quantity: 2, UnitPriceCents: 1}}

	// Product service down: the order waits in PENDING and nothing is announced yet.
	reserveCode.Store(503)
	if _, err := s.CreateOrder(ctx, user, "k1", items); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}

	// Retrying with the same key finishes that same order.
	reserveCode.Store(200)
	order, err := s.CreateOrder(ctx, user, "k1", items)
	expect(t, order, err, entity.StatusConfirmed)
	if order.TotalCents != 3000 || order.Items[0].UnitPriceCents != 1500 {
		t.Fatalf("priced %+v, want 2 x 1500 = 3000", order)
	}
	again, err := s.CreateOrder(ctx, user, "k1", items)
	if err != nil || again.ID != order.ID {
		t.Fatalf("replay gave order %v (%v), want %d", again, err, order.ID)
	}

	// A key belongs to its request: other items under it are refused, and keys are case-sensitive.
	if _, err := s.CreateOrder(ctx, user, "k1", []entity.Item{{ProductID: 2, Quantity: 1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reused key: got %v, want ErrInvalid", err)
	}
	upper, err := s.CreateOrder(ctx, user, "K1", items)
	expect(t, upper, err, entity.StatusConfirmed)
	if upper.ID == order.ID {
		t.Fatalf("key K1 returned k1's order %d", order.ID)
	}

	// Paid orders can't be canceled, and other users can't see them.
	paid, err := s.PayOrder(ctx, user, order.ID)
	expect(t, paid, err, entity.StatusPaid)
	if _, err := s.CancelOrder(ctx, user, order.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancel paid order: got %v, want ErrConflict", err)
	}
	if _, err := s.GetOrder(ctx, user+1, order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's order: got %v, want ErrNotFound", err)
	}
	if got := outboxTopics(t, db, order.ID); !slices.Equal(got, []string{TopicOrderCreated, TopicOrderPaid}) {
		t.Fatalf("events %v", got)
	}

	// Sold out: canceled, and order.canceled goes out (releasing nothing is a no-op).
	reserveCode.Store(409)
	soldOut, err := s.CreateOrder(ctx, user, "k2", items)
	expect(t, soldOut, err, entity.StatusCanceled)
	if soldOut.CancelReason != "out_of_stock" {
		t.Fatalf("cancel reason %q", soldOut.CancelReason)
	}
	if got := outboxTopics(t, db, soldOut.ID); !slices.Equal(got, []string{TopicOrderCanceled}) {
		t.Fatalf("events %v", got)
	}

	// A user cancel announces order.canceled so the stock comes back.
	reserveCode.Store(200)
	placed, err := s.CreateOrder(ctx, user, "k3", items)
	expect(t, placed, err, entity.StatusConfirmed)
	canceled, err := s.CancelOrder(ctx, user, placed.ID)
	expect(t, canceled, err, entity.StatusCanceled)
	if got := outboxTopics(t, db, placed.ID); !slices.Equal(got, []string{TopicOrderCreated, TopicOrderCanceled}) {
		t.Fatalf("events %v", got)
	}
}
