package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"pricing-service/internal/api"
	"pricing-service/internal/service"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.SetDefault(slog.New(zerolog.NewSlogHandler(log.Logger))) // echo's request logs, same JSON as ours

	rdb := redis.NewClient(&redis.Options{Addr: mustEnv("REDIS_ADDR"), Password: mustEnv("REDIS_PASSWORD")})
	defer rdb.Close()

	// Initialize pricing service
	pricingService := service.NewPricingService(rdb, mustEnv("PRODUCT_SERVICE_URL"))
	pricingHandler := api.NewPricingHandler(pricingService)

	// Reprice whenever stock changes
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        strings.Split(mustEnv("KAFKA_BROKERS"), ","),
		GroupID:        "pricing-service",
		Topic:          "stock.changed",
		CommitInterval: time.Second,
	})
	defer reader.Close()
	go consume(ctx, reader, pricingService.OnStockChanged)

	// Initialize echo
	e := echo.New()
	e.HideBanner = true
	e.IPExtractor = echo.ExtractIPDirect() // no proxy in front: a client-sent X-Forwarded-For is not its IP

	// Middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("64K"))

	// Routes: prices are public. Health ignores Redis: without the cache, prices still work.
	e.GET("/healthz", func(c echo.Context) error { return c.NoContent(200) })
	e.GET("/prices/:id", pricingHandler.GetPrice)

	serve(ctx, e, ":8083")
}

// consume feeds every message to handle until ctx is done. Best-effort: a message
// that fails is logged and skipped, and the cache TTL heals the missed update.
func consume(ctx context.Context, r *kafka.Reader, handle func(context.Context, []byte) error) {
	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Msg("read stock.changed")
			time.Sleep(time.Second)
			continue
		}
		if err := handle(ctx, msg.Value); err != nil {
			log.Error().Err(err).Bytes("value", msg.Value).Msg("skipping stock.changed")
		}
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal().Msgf("%s is required", key)
	}
	return v
}

// serve runs e until ctx is canceled, then lets in-flight requests finish.
func serve(ctx context.Context, e *echo.Echo, addr string) {
	// Slow or idle clients can't hold connections open forever.
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 15 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	go func() {
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server stopped")
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown")
	}
}
