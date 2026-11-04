// Package aiclient calls the Python extraction service.
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResponse = 8 << 20

type Result struct {
	Draft, Warnings, Meta json.RawMessage
}

// Error carries only a code and status; never response or PDF content (KVKK).
type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string { return fmt.Sprintf("aiclient: %s (status %d)", e.Code, e.Status) }

type Client struct {
	base, token string
	hc          *http.Client
}

func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, hc: &http.Client{Timeout: timeout}}
}

func (c *Client) Extract(ctx context.Context, pdf []byte) (Result, error) {
	unavailable := &Error{Code: "AI_UNAVAILABLE"}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/extract", bytes.NewReader(pdf))
	if err != nil {
		return Result{}, unavailable
	}
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Result{}, unavailable // url.Error may embed the URL; drop it
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	unavailable.Status = resp.StatusCode
	if err != nil {
		return Result{}, unavailable
	}
	if resp.StatusCode != http.StatusOK {
		var eb struct{ Code string }
		if json.Unmarshal(body, &eb) != nil || eb.Code == "" {
			return Result{}, unavailable
		}
		return Result{}, &Error{Code: eb.Code, Status: resp.StatusCode}
	}
	var r Result
	var raw struct{ Draft, Warnings, Meta json.RawMessage }
	if json.Unmarshal(body, &raw) != nil || len(raw.Draft) == 0 {
		return Result{}, unavailable
	}
	r = Result(raw)
	return r, nil
}
