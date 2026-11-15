// Command resetlink prints a one-time password reset link for a user, for when no colleague can issue one (the
// company's only user forgot the password). It runs as the owner role (MIGRATE_DATABASE_URL). The link is valid for
// one hour; hand it to the user over a channel where you know it is them.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func main() {
	email := flag.String("email", "", "the user's e-mail")
	base := flag.String("base", "", "the app's address, e.g. https://matchwatch.example.com.tr")
	flag.Parse()
	if !strings.Contains(*email, "@") || *base == "" {
		log.Fatal("usage: resetlink -email user@company.com.tr -base https://APP-DOMAIN")
	}
	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		log.Fatal("MIGRATE_DATABASE_URL (owner role) is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)
	cred, ok, err := store.LookupLogin(ctx, strings.ToLower(strings.TrimSpace(*email)))
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("no user with this e-mail")
	}
	token, hash := auth.NewLinkToken()
	err = store.InTenant(ctx, cred.TenantID, func(tx *repository.Tx) error {
		_, _, err := tx.CreateResetLink(ctx, cred.UserID, nil, hash)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s/sifre#%s\n", strings.TrimRight(*base, "/"), token)
}
