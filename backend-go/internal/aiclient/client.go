// Package aiclient, Python ile yazdığımız AI servisini çağırır: sözleşme çıkarımı ve embedding.
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

// Error yalnızca bir kod ve HTTP durumu taşır. Yanıtın ya da PDF'in içeriği asla buraya girmez (KVKK).
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

// post, isteği gönderir ve 200 yanıtının gövdesini döner. Hata olursa yalnızca kod ve durum taşıyan bir *Error döner.
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
		return nil, unavailable // url.Error içinde adres de olabilir, onu atıyoruz
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

const embedBatch = 256 // ai-service'in tek istekte kabul ettiği sınır

// Embed, her metin için sırasıyla bir vektör döner. Vektörler ham JSON dizisi olarak kalır (geçerli birer pgvector
// değeri); hiçbir zaman float'a çevrilmez.
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
