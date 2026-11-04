package aiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func code(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want *Error, got %v", err)
	}
	return e
}

func TestExtractOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/extract" ||
			r.Header.Get("Content-Type") != "application/pdf" ||
			r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad request: %s %s %v", r.Method, r.URL.Path, r.Header)
		}
		w.Write([]byte(`{"draft":{"contract_no":"C1"},"warnings":[],"meta":{"a":1}}`))
	}))
	defer srv.Close()
	res, err := New(srv.URL, "tok", time.Second).Extract(context.Background(), []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Draft), "contract_no") || string(res.Warnings) != "[]" || len(res.Meta) == 0 {
		t.Fatalf("result: %+v", res)
	}
}

func TestExtractServiceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte(`{"code":"LLM_UNAVAILABLE","error":"x"}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", time.Second).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "LLM_UNAVAILABLE" || e.Status != 502 {
		t.Fatalf("%+v", e)
	}
}

func TestExtractNonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte(`<html>bad gateway</html>`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", time.Second).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "AI_UNAVAILABLE" || e.Status != 503 {
		t.Fatalf("%+v", e)
	}
}

func TestExtractServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := New(url, "t", time.Second).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "AI_UNAVAILABLE" {
		t.Fatalf("%+v", e)
	}
}

func TestExtractTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", 100*time.Millisecond).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "AI_UNAVAILABLE" {
		t.Fatalf("%+v", e)
	}
}

func TestExtractTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxResponse+1))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", 10*time.Second).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "AI_RESPONSE_TOO_LARGE" || e.Status != 200 {
		t.Fatalf("%+v", e)
	}
}

func TestExtractNullDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"draft":null,"warnings":[],"meta":{}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", time.Second).Extract(context.Background(), nil)
	if e := code(t, err); e.Code != "AI_UNAVAILABLE" || e.Status != 200 {
		t.Fatalf("%+v", e)
	}
}

func TestEmbedBatchesAndKeepsOrder(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in struct{ Texts []string }
		if r.URL.Path != "/v1/embed" || r.Header.Get("Content-Type") != "application/json" ||
			r.Header.Get("Authorization") != "Bearer tok" || json.NewDecoder(r.Body).Decode(&in) != nil {
			t.Errorf("bad request: %s %v", r.URL.Path, r.Header)
		}
		vs := make([]string, len(in.Texts))
		for i, s := range in.Texts {
			vs[i] = "[" + s + ",1e-05]"
		}
		w.Write([]byte(`{"model":"bge-m3","vectors":[` + strings.Join(vs, ",") + `]}`))
	}))
	defer srv.Close()
	texts := make([]string, 300)
	for i := range texts {
		texts[i] = strconv.Itoa(i)
	}
	model, vecs, err := New(srv.URL, "tok", time.Second).Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || model != "bge-m3" || len(vecs) != 300 || string(vecs[0]) != "[0,1e-05]" || string(vecs[299]) != "[299,1e-05]" {
		t.Fatalf("calls=%d model=%s n=%d first=%s", calls, model, len(vecs), vecs[0])
	}
}

func TestEmbedErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		code   string
	}{
		"count mismatch": {200, `{"model":"m","vectors":[]}`, "AI_BAD_RESPONSE"},
		"no model":       {200, `{"vectors":[[1]]}`, "AI_BAD_RESPONSE"},
		"not array":      {200, `{"model":"m","vectors":["x"]}`, "AI_BAD_RESPONSE"},
		"upstream code":  {502, `{"code":"LLM_UNAVAILABLE","error":"x"}`, "LLM_UNAVAILABLE"},
		"garbage error":  {500, `oops`, "AI_UNAVAILABLE"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		_, _, err := New(srv.URL, "tok", time.Second).Embed(context.Background(), []string{"a"})
		if e := code(t, err); e.Code != tc.code {
			t.Errorf("%s: got %s", name, e.Code)
		}
		srv.Close()
	}
}
