package aiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
