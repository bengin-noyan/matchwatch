// Command devtoken creates a tenant and prints a 24h JWT for it (development only).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/config"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func main() {
	name := flag.String("name", "", "tenant name")
	vkn := flag.String("vkn", "", "tenant VKN (10 or 11 digits)")
	rate := flag.String("rate", "0", "annual financing rate, e.g. 0.45")
	flag.Parse()
	if *name == "" || (len(*vkn) != 10 && len(*vkn) != 11) {
		log.Fatal("usage: devtoken -name NAME -vkn 9876543210 -rate 0.45")
	}
	r, err := decimal.NewFromString(*rate)
	if err != nil {
		log.Fatalf("rate: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	tid, err := repository.NewStore(pool).CreateTenant(ctx, *name, *vkn, r)
	if err != nil {
		log.Fatal(err)
	}
	tok, err := auth.Sign(cfg.JWTSecret, tid, "devtoken", 24*time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tenant_id=%s\n%s\n", tid, tok)
}
