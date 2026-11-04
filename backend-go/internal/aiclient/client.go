// Package aiclient calls the Python AI service (extraction and embeddings).
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

// post sends body and returns the 200 response body, or an *Error carrying only a code and status.
func (c *Client) post(ctx context.Context, path, contentType string, body []byte) ([]byte, error) {
	unavailable := &Error{Code: "AI_UNAVAILABLE"}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, unavailable
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, unavailable // url.Error may embed the URL; drop it
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	unavailable.Status = resp.StatusCode
	if err != nil {
		return nil, unavailable
	}
	if len(out) > maxResponse {
		return nil, &Error{Code: "AI_RESPONSE_TOO_LARGE", Status: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		var eb struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(out, &eb) != nil || eb.Code == "" {
			return nil, unavailable
		}
		return nil, &Error{Code: eb.Code, Status: resp.StatusCode}
	}
	return out, nil
}

func (c *Client) Extract(ctx context.Context, pdf []byte) (Result, error) {
	body, err := c.post(ctx, "/v1/extract", "application/pdf", pdf)
	if err != nil {
		return Result{}, err
	}
	var raw struct{ Draft, Warnings, Meta json.RawMessage }
	if json.Unmarshal(body, &raw) != nil || len(raw.Draft) == 0 || string(raw.Draft) == "null" {
		return Result{}, &Error{Code: "AI_UNAVAILABLE", Status: http.StatusOK}
	}
	return Result(raw), nil
}

const embedBatch = 256 // ai-service limit per request

// Embed returns one vector per text, in order, as raw JSON arrays (valid pgvector literals; never parsed into floats).
func (c *Client) Embed(ctx context.Context, texts []string) (string, []json.RawMessage, error) {
	var model string
	var vecs []json.RawMessage
	for start := 0; start < len(texts); start += embedBatch {
		chunk := texts[start:min(start+embedBatch, len(texts))]
		reqBody, err := json.Marshal(map[string][]string{"texts": chunk})
		if err != nil {
			return "", nil, &Error{Code: "AI_UNAVAILABLE"}
		}
		body, err := c.post(ctx, "/v1/embed", "application/json", reqBody)
		if err != nil {
			return "", nil, err
		}
		var r struct {
			Model   string            `json:"model"`
			Vectors []json.RawMessage `json:"vectors"`
		}
		if json.Unmarshal(body, &r) != nil || r.Model == "" || len(r.Vectors) != len(chunk) || (model != "" && r.Model != model) {
			return "", nil, &Error{Code: "AI_BAD_RESPONSE", Status: http.StatusOK}
		}
		for _, v := range r.Vectors {
			if len(v) == 0 || v[0] != '[' {
				return "", nil, &Error{Code: "AI_BAD_RESPONSE", Status: http.StatusOK}
			}
		}
		model = r.Model
		vecs = append(vecs, r.Vectors...)
	}
	return model, vecs, nil
}
