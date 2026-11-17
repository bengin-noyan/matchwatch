package transporthttp

import (
	"net/http"
	"time"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// getDashboard: ?from=YYYY-MM-DD&to=YYYY-MM-DD, faturanın düzenleme tarihine göre; ikisi de isteğe bağlı.
func (s *server) getDashboard(w http.ResponseWriter, r *http.Request) {
	var bounds [2]*time.Time
	for i, k := range []string{"from", "to"} {
		if v := r.URL.Query().Get(k); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				writeErr(w, 400, "BAD_REQUEST", k+" must be a date (YYYY-MM-DD)")
				return
			}
			bounds[i] = &t
		}
	}
	var d repository.Dashboard
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		d, err = tx.Dashboard(r.Context(), bounds[0], bounds[1])
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *server) listVendors(w http.ResponseWriter, r *http.Request) {
	var out []repository.VendorRow
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		out, err = tx.ListVendors(r.Context())
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *server) getVendor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var d repository.VendorDetail
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		d, err = tx.VendorDetail(r.Context(), id)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}
