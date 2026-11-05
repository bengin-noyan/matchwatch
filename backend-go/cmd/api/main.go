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

	"github.com/bengin-noyan/matchwatch/backend-go/internal/aiclient"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/config"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/tcmb"
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
	var ai transporthttp.Extractor // nil interfaces = disabled; a typed-nil *aiclient.Client would not be == nil
	var emb audit.Embedder
	if cfg.AIServiceURL != "" {
		c := aiclient.New(cfg.AIServiceURL, cfg.AIServiceToken, cfg.AIExtractTimeout)
		ai, emb = c, c
	}
	var fx audit.FXSource
	if cfg.TCMBBaseURL != "" {
		fx = tcmb.New(cfg.TCMBBaseURL)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           transporthttp.NewRouter(store, audit.New(store, emb, fx), cfg.StorageDir, cfg.JWTSecret, ai, cfg.AIExtractTimeout),
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
