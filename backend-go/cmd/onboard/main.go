// Command onboard, yeni bir müşteri hesabı açar: bir tenant ve onun ilk kullanıcısı. Owner rolüyle çalışır
// (MIGRATE_DATABASE_URL), çünkü API'nin rolü kullanıcı oluşturamaz. Şifreyi stdin'den okur.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func main() {
	name := flag.String("name", "", "company name")
	vkn := flag.String("vkn", "", "company VKN (10 or 11 digits)")
	rate := flag.String("rate", "0", "annual financing rate, e.g. 0.45")
	email := flag.String("email", "", "first user's e-mail")
	flag.Parse()
	if *name == "" || (len(*vkn) != 10 && len(*vkn) != 11) || !strings.Contains(*email, "@") {
		log.Fatal("usage: onboard -name NAME -vkn 9876543210 -rate 0.45 -email user@company.com.tr  (password on stdin)")
	}
	r, err := decimal.NewFromString(*rate)
	if err != nil {
		log.Fatalf("rate: %v", err)
	}
	pw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	pw = strings.TrimRight(pw, "\r\n")
	if (err != nil && pw == "") || len(pw) < 12 {
		log.Fatal("password (stdin) must be at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), auth.PasswordCost)
	if err != nil {
		log.Fatal(err)
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
	tid := uuid.New()
	var uid uuid.UUID
	err = repository.NewStore(pool).InTenant(ctx, tid, func(tx *repository.Tx) error { // ya tenant da kullanıcı da açılır ya da hiçbiri
		if err := tx.InsertTenant(ctx, *name, *vkn, r); err != nil {
			return err
		}
		uid, err = tx.CreateUser(ctx, *email, string(hash))
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tenant_id=%s user_id=%s\n", tid, uid)
}
