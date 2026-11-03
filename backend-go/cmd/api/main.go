// Command api serves the matchwatch REST API.
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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/config"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	transporthttp "github.com/bengin-noyan/matchwatch/backend-go/internal/transport/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           transporthttp.NewRouter(store, audit.New(store), cfg.StorageDir, cfg.JWTSecret),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
