package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/migrate"
)

func main() {
	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		log.Fatal("MIGRATE_DATABASE_URL is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	if err := migrate.Apply(ctx, conn); err != nil {
		log.Fatal(err)
	}
	log.Println("migrations applied")
}
