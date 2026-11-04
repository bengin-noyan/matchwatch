// Package transporthttp is the REST API under /api/v1; every route except /healthz requires a tenant JWT.
package transporthttp

import (
	"net/http"
	"time"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type server struct {
	store          *repository.Store
	auditor        *audit.Service
	storageDir     string
	ai             Extractor // nil = extraction disabled
	extractTimeout time.Duration
	secret         []byte
	cookieSecure   bool
	limiter        *auth.Limiter
}

func NewRouter(store *repository.Store, auditor *audit.Service, storageDir string, secret []byte, ai Extractor, extractTimeout time.Duration, cookieSecure bool) http.Handler {
	s := &server{store: store, auditor: auditor, storageDir: storageDir, ai: ai, extractTimeout: extractTimeout,
		secret: secret, cookieSecure: cookieSecure, limiter: auth.NewLimiter(10, 15*time.Minute)}
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/auth/me", s.me)
	api.HandleFunc("POST /api/v1/contracts", s.postContract)
	api.HandleFunc("POST /api/v1/contract-drafts", s.postDraft)
	api.HandleFunc("GET /api/v1/contract-drafts/{id}", s.getDraft)
	api.HandleFunc("POST /api/v1/contract-drafts/{id}/extract", s.reextractDraft)
	api.HandleFunc("POST /api/v1/despatches", s.postDespatch)
	api.HandleFunc("POST /api/v1/receipts", s.postReceipt)
	api.HandleFunc("POST /api/v1/invoices", s.postInvoice)
	api.HandleFunc("GET /api/v1/invoices", s.listInvoices)
	api.HandleFunc("GET /api/v1/invoices/{id}", s.getInvoice)
	api.HandleFunc("POST /api/v1/invoices/{id}/audit", s.auditInvoice)
	api.HandleFunc("POST /api/v1/invoice-lines/{id}/match", s.postLineMatch)
	api.HandleFunc("PATCH /api/v1/findings/{id}", s.patchFinding)
	api.HandleFunc("GET /api/v1/summary", s.getSummary)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.HandleFunc("POST /api/v1/auth/login", s.login) // the only API routes without a session
	root.HandleFunc("POST /api/v1/auth/logout", s.logout)
	root.Handle("/api/", auth.Middleware(secret)(api))
	return root
}
