package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/aiclient"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type fakeAI struct {
	mu    sync.Mutex
	err   error
	delay time.Duration
	calls int
	// optional: Extract signals started, then waits for release (ignoring ctx unless it is cancelled)
	started, release chan struct{}
}

func (f *fakeAI) set(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeAI) Extract(ctx context.Context, pdf []byte) (aiclient.Result, error) {
	f.mu.Lock()
	err, delay := f.err, f.delay
	f.calls++
	f.mu.Unlock()
	if f.release != nil {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return aiclient.Result{}, ctx.Err()
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return aiclient.Result{}, ctx.Err()
		}
	}
	if err != nil {
		return aiclient.Result{}, err
	}
	return aiclient.Result{
		Draft:    json.RawMessage(`{"contract_no":"S-2026-001"}`),
		Warnings: json.RawMessage(`["low_confidence"]`),
		Meta:     json.RawMessage(`{"pages":3}`),
	}, nil
}

const fakePDF = "%PDF-1.7\n1 0 obj<<>>endobj\n%%EOF\n"

func multipartPDF(t *testing.T, content string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "contract.pdf")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(content))
	mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

// withAI swaps the env's router for one wired to ai.
func (e *env) withAI(ai Extractor, timeout time.Duration) {
	e.srv = NewRouter(e.store, audit.New(e.store, nil, nil), e.storage, secret, ai, timeout, false)
}

func (e *env) upload(content string) (int, resp) {
	e.t.Helper()
	body, ct := multipartPDF(e.t, content)
	req := httptest.NewRequest("POST", "/api/v1/contract-drafts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	var out resp
	d := json.NewDecoder(rec.Body)
	d.UseNumber()
	d.Decode(&out)
	return rec.Code, out
}

func contractWithDraft(t *testing.T, draftID string) []byte {
	b := fixture(t, "contract_basic.json")
	return []byte(strings.Replace(string(b), "{", `{"draft_id":"`+draftID+`",`, 1))
}

func (e *env) count(q string) int {
	var n int
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), q).Scan(&n)
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestDraftHappyPath(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	code, b := e.upload(fakePDF)
	e.mustStatus(201, code, b)
	id := str(b["id"])
	if str(b["status"]) != "READY" || str(obj(b["draft"])["contract_no"]) != "S-2026-001" || len(list(b["warnings"])) != 1 {
		t.Fatalf("upload resp %v", b)
	}
	code, g := e.req("GET", "/api/v1/contract-drafts/"+id, nil)
	e.mustStatus(200, code, g)
	if str(g["status"]) != "READY" || str(obj(g["draft"])["contract_no"]) != "S-2026-001" {
		t.Fatalf("get resp %v", g)
	}
	code, c := e.req("POST", "/api/v1/contracts", contractWithDraft(t, id))
	e.mustStatus(201, code, c)
	code, g = e.req("GET", "/api/v1/contract-drafts/"+id, nil)
	e.mustStatus(200, code, g)
	if str(g["status"]) != "APPROVED" || str(g["contract_id"]) != str(c["id"]) {
		t.Fatalf("approved resp %v", g)
	}
	pdfPath := filepath.Join(e.storage, e.tid.String(), "contract", id+".pdf")
	if got, err := os.ReadFile(pdfPath); err != nil || string(got) != fakePDF {
		t.Fatalf("pdf on disk: %q %v", got, err)
	}
	var fp string
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT file_path FROM contracts WHERE id=$1`, str(c["id"])).Scan(&fp)
	})
	if err != nil || fp != pdfPath {
		t.Fatalf("contracts.file_path %q want %q (%v)", fp, pdfPath, err)
	}
}

func TestDraftNotPDF(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	code, b := e.upload("hello")
	e.mustStatus(400, code, b)
	if str(b["code"]) != "NOT_PDF" {
		t.Fatalf("resp %v", b)
	}
	if n := e.count(`SELECT count(*) FROM contract_drafts`); n != 0 {
		t.Fatalf("drafts %d", n)
	}
}

func TestDraftRecoverAfterFailure(t *testing.T) {
	e := setup(t, "9876543210")
	ai := &fakeAI{err: &aiclient.Error{Code: "LLM_UNAVAILABLE", Status: 503}}
	e.withAI(ai, 10*time.Second)
	code, b := e.upload(fakePDF)
	e.mustStatus(201, code, b)
	id := str(b["id"])
	if str(b["status"]) != "FAILED" || str(b["error_code"]) != "LLM_UNAVAILABLE" {
		t.Fatalf("upload resp %v", b)
	}
	code, c := e.req("POST", "/api/v1/contracts", contractWithDraft(t, id))
	e.mustStatus(409, code, c)
	if str(c["code"]) != "DRAFT_NOT_READY" {
		t.Fatalf("contract resp %v", c)
	}
	ai.set(nil)
	code, x := e.req("POST", "/api/v1/contract-drafts/"+id+"/extract", nil)
	e.mustStatus(200, code, x)
	if str(x["status"]) != "READY" || str(x["error_code"]) != "" {
		t.Fatalf("extract resp %v", x)
	}
}

func TestDraftApproveTwice(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	code, b := e.upload(fakePDF)
	e.mustStatus(201, code, b)
	id := str(b["id"])
	code, c := e.req("POST", "/api/v1/contracts", contractWithDraft(t, id))
	e.mustStatus(201, code, c)
	code, c = e.req("POST", "/api/v1/contracts", contractWithDraft(t, id))
	e.mustStatus(409, code, c)
	if str(c["code"]) != "DRAFT_APPROVED" {
		t.Fatalf("second approve %v", c)
	}
	code, x := e.req("POST", "/api/v1/contract-drafts/"+id+"/extract", nil)
	e.mustStatus(409, code, x)
	if str(x["code"]) != "DRAFT_APPROVED" {
		t.Fatalf("extract resp %v", x)
	}
}

func TestDraftCrossTenant(t *testing.T) {
	a := setup(t, "9876543210")
	bt := setup(t, "9876543210")
	a.withAI(&fakeAI{}, 10*time.Second)
	bt.withAI(&fakeAI{}, 10*time.Second)
	code, b := a.upload(fakePDF)
	a.mustStatus(201, code, b)
	id := str(b["id"])
	code, g := bt.req("GET", "/api/v1/contract-drafts/"+id, nil)
	bt.mustStatus(404, code, g)
	code, c := bt.req("POST", "/api/v1/contracts", contractWithDraft(t, id))
	bt.mustStatus(404, code, c)
	if n := bt.count(`SELECT count(*) FROM contracts`); n != 0 {
		t.Fatalf("tenant B contracts %d", n)
	}
	code, x := bt.req("POST", "/api/v1/contract-drafts/"+id+"/extract", nil)
	bt.mustStatus(404, code, x)
}

func TestDraftDisabled(t *testing.T) {
	e := setup(t, "9876543210") // setup wires NewRouter(..., nil, 0)
	code, b := e.upload(fakePDF)
	e.mustStatus(503, code, b)
	if str(b["code"]) != "AI_DISABLED" {
		t.Fatalf("resp %v", b)
	}
	code, b = e.req("POST", "/api/v1/contract-drafts/"+e.tid.String()+"/extract", nil)
	e.mustStatus(503, code, b)
	if str(b["code"]) != "AI_DISABLED" {
		t.Fatalf("extract resp %v", b)
	}
}

// reading a draft needs no extractor: only the POST routes are gated
func TestDraftGetWithoutAI(t *testing.T) {
	e := setup(t, "9876543210")
	id := uuid.New()
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.InsertDraft(context.Background(), id, "p", "h")
	})
	if err != nil {
		t.Fatal(err)
	}
	code, b := e.req("GET", "/api/v1/contract-drafts/"+id.String(), nil)
	e.mustStatus(200, code, b)
}

func TestDraftSlowExtraction(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{delay: 2 * time.Second}, 10*time.Second)
	ts := httptest.NewUnstartedServer(e.srv)
	ts.Config.WriteTimeout = time.Second
	ts.Start()
	defer ts.Close()
	body, ct := multipartPDF(t, fakePDF)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/contract-drafts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", ct)
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	var out resp
	d := json.NewDecoder(res.Body)
	d.UseNumber()
	if err := d.Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	e.mustStatus(201, res.StatusCode, out)
	if str(out["status"]) != "READY" {
		t.Fatalf("resp %v", out)
	}
}

func TestContractWithoutDraftUnchanged(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	code, b := e.req("POST", "/api/v1/contracts", fixture(t, "contract_basic.json"))
	e.mustStatus(201, code, b)
	if str(b["id"]) == "" {
		t.Fatalf("resp %v", b)
	}
	var fp string
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT file_path FROM contracts WHERE id=$1`, str(b["id"])).Scan(&fp)
	})
	if err != nil || fp != "" {
		t.Fatalf("file_path %q (%v)", fp, err)
	}
}

func TestDraftTooLarge(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	code, b := e.upload(fakePDF + strings.Repeat("x", maxPDFBody))
	e.mustStatus(413, code, b)
	if n := e.count(`SELECT count(*) FROM contract_drafts`); n != 0 {
		t.Fatalf("drafts %d", n)
	}
}

func TestDraftPDFOver20MiB(t *testing.T) {
	e := setup(t, "9876543210")
	e.withAI(&fakeAI{}, 10*time.Second)
	// 20 MiB + 1 byte: inside the 21 MiB multipart cap, over the PDF cap
	code, b := e.upload("%PDF-" + strings.Repeat("x", maxPDF+1-5))
	e.mustStatus(413, code, b)
	if n := e.count(`SELECT count(*) FROM contract_drafts`); n != 0 {
		t.Fatalf("drafts %d", n)
	}
	if code, b = e.upload("%PDF-" + strings.Repeat("x", maxPDF-5)); code != 201 {
		t.Fatalf("exactly 20 MiB: %d %v", code, b)
	}
}

func TestDraftSurvivesClientDisconnect(t *testing.T) {
	e := setup(t, "9876543210")
	ai := &fakeAI{started: make(chan struct{}), release: make(chan struct{})}
	e.withAI(ai, 10*time.Second)
	body, ct := multipartPDF(t, fakePDF)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/api/v1/contract-drafts", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Content-Type", ct)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.srv.ServeHTTP(httptest.NewRecorder(), req)
	}()
	<-ai.started
	cancel() // client goes away mid-extraction
	time.Sleep(50 * time.Millisecond)
	close(ai.release)
	<-done
	if n := e.count(`SELECT count(*) FROM contract_drafts WHERE status='READY'`); n != 1 {
		t.Fatalf("READY drafts %d, want 1 (extraction must survive the disconnect)", n)
	}
}
