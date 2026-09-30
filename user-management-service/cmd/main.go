package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"user-management-service/internal/api"
	"user-management-service/internal/repository"
	"user-management-service/internal/service"

	_ "github.com/go-sql-driver/mysql"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.SetDefault(slog.New(zerolog.NewSlogHandler(log.Logger))) // echo's request logs, same JSON as ours

	db := openDB(mustEnv("DB_DSN"))
	defer db.Close()
	jwtSecret := []byte(mustSecret("JWT_SECRET"))

	// Initialize user service
	userService := service.NewUserService(repository.NewUserRepository(db))
	userHandler := api.NewUserHandler(userService, jwtSecret)

	// Initialize echo
	e := echo.New()
	e.HideBanner = true
	e.IPExtractor = echo.ExtractIPDirect() // no proxy in front: a client-sent X-Forwarded-For is not its IP

	// Middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("64K"))

	// Routes: signup and login are public, everything else needs a token
	e.GET("/healthz", func(c echo.Context) error {
		if err := db.PingContext(c.Request().Context()); err != nil {
			return c.NoContent(503)
		}
		return c.NoContent(200)
	})
	// Slows password guessing and signup spam: 10 attempts a minute per IP.
	// ponytail: in-memory, so per replica; move to Redis or the gateway when scaled out.
	authLimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(
		middleware.RateLimiterMemoryStoreConfig{Rate: rate.Every(6 * time.Second), Burst: 10, ExpiresIn: 3 * time.Minute}))
	e.POST("/users", userHandler.CreateUser, authLimit)
	e.POST("/login", userHandler.Login, authLimit)
	e.GET("/users/me", userHandler.Me, echojwt.JWT(jwtSecret))

	serve(ctx, e, ":8080")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal().Msgf("%s is required", key)
	}
	return v
}

// mustSecret is mustEnv for keys and tokens: one short enough to guess is a startup error.
func mustSecret(key string) string {
	v := mustEnv(key)
	if len(v) < 32 {
		log.Fatal().Msgf("%s must be at least 32 characters", key)
	}
	return v
}

func openDB(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err == nil {
		err = db.Ping()
	}
	if err != nil {
		log.Fatal().Err(err).Msg("connect to mysql")
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(30)
	return db
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
