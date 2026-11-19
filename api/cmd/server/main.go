package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ploiid10/beautyscale-order-requests/api/internal/db"
	httpapi "github.com/ploiid10/beautyscale-order-requests/api/internal/http"
	"github.com/ploiid10/beautyscale-order-requests/api/internal/order"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := env("DATABASE_URL", "postgres://orders:orders@localhost:5432/orders?sslmode=disable")
	addr := env("HTTP_ADDR", ":8080")

	if err := db.Migrate(databaseURL, env("MIGRATIONS_DIR", "migrations")); err != nil {
		return err
	}
	if env("SEED", "true") != "false" {
		if err := db.Seed(ctx, databaseURL, env("SEED_FILE", "seed/seed.sql")); err != nil {
			return err
		}
	}

	store, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(order.NewService(store)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
