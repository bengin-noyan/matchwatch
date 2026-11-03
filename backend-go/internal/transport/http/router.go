// Package transporthttp is the REST API under /api/v1; every route except /healthz requires a tenant JWT.
package transporthttp

import (
	"net/http"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type server struct {
	store      *repository.Store
	auditor    *audit.Service
	storageDir string
}

func NewRouter(store *repository.Store, auditor *audit.Service, storageDir string, secret []byte) http.Handler {
	s := &server{store: store, auditor: auditor, storageDir: storageDir}
	api := http.NewServeMux()
	api.HandleFunc("POST /api/v1/contracts", s.postContract)
	api.HandleFunc("POST /api/v1/despatches", s.postDespatch)
	api.HandleFunc("POST /api/v1/receipts", s.postReceipt)
	api.HandleFunc("POST /api/v1/invoices", s.postInvoice)
	api.HandleFunc("GET /api/v1/invoices", s.listInvoices)
	api.HandleFunc("GET /api/v1/invoices/{id}", s.getInvoice)
	api.HandleFunc("POST /api/v1/invoices/{id}/audit", s.auditInvoice)
	api.HandleFunc("PATCH /api/v1/findings/{id}", s.patchFinding)
	api.HandleFunc("GET /api/v1/summary", s.getSummary)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.Handle("/api/", auth.Middleware(secret)(api))
	return root
}
