// Package aperture is the HTTP client for the Aperture admin API.
//
// API surface (see /aperture/openapi.json on a live gateway):
//
//	GET  /config            -> {config: <hujson>}, ETag header
//	PUT  /config            -> If-Match required; replaces document
//	POST /config:validate   -> {valid, errors}
//
// Auth is by Tailscale identity at the network layer — the caller
// must be on the tailnet with the admin role grant. This package
// therefore sends no Authorization header.
package aperture

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tailscale/hujson"
)

// ErrPreconditionFailed is returned from SetConfig when the gateway's
// stored ETag doesn't match the one we sent. The caller should
// re-Read and re-plan.
var ErrPreconditionFailed = errors.New("aperture: configuration changed since last read; re-plan required")

// ClientConfig configures a Client. Only Endpoint is required.
type ClientConfig struct {
	// Endpoint is the full base URL including the /aperture path
	// prefix, e.g. "http://ai.tail396699.ts.net/aperture".
	Endpoint string

	UserAgent string

	// HTTPClient is optional; a sensible default is used when nil.
	HTTPClient *http.Client
}

// Client talks to an Aperture admin endpoint over HTTP.
type Client struct {
	cfg  ClientConfig
	http *http.Client
}

// NewClient returns a configured Client. Endpoint may be empty, in
// which case every method returns a clear error — this lets the
// provider instantiate a Client without forcing endpoint discovery
// at provider Configure time.
func NewClient(cfg ClientConfig) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS13,
				},
			},
		}
	}
	return &Client{cfg: cfg, http: httpClient}
}

// Endpoint returns the configured endpoint, or "" if none was set.
func (c *Client) Endpoint() string { return c.cfg.Endpoint }

// configEnvelope wraps a HuJSON config string in the {config: ...}
// shape that GetConfig/SetConfig/ValidateConfig all use.
type configEnvelope struct {
	Config string `json:"config"`
}

// ValidationResult mirrors the upstream ValidationResult schema.
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// GetConfig returns the current Aperture configuration as a
// HuJSON string and its ETag. API keys are redacted server-side;
// don't trust the apikey/authorization fields from this response.
func (c *Client) GetConfig(ctx context.Context) (config string, etag string, err error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", c.problemError(resp)
	}
	var env configEnvelope
	limitedBody := io.LimitReader(resp.Body, 10*1024*1024)
	if err := json.NewDecoder(limitedBody).Decode(&env); err != nil {
		return "", "", fmt.Errorf("aperture: decode get-config: %w", err)
	}
	return env.Config, resp.Header.Get("ETag"), nil
}

// SetConfig replaces the entire Aperture configuration. ifMatch must
// be the ETag from a prior GetConfig (the API requires it). On 412,
// returns ErrPreconditionFailed.
func (c *Client) SetConfig(ctx context.Context, config, ifMatch string) (saved string, etag string, err error) {
	body, err := json.Marshal(configEnvelope{Config: config})
	if err != nil {
		return "", "", err
	}
	req, err := c.newRequest(ctx, http.MethodPut, "/config", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var env configEnvelope
		limitedBody := io.LimitReader(resp.Body, 10*1024*1024)
		if err := json.NewDecoder(limitedBody).Decode(&env); err != nil {
			return "", "", fmt.Errorf("aperture: decode set-config: %w", err)
		}
		return env.Config, resp.Header.Get("ETag"), nil
	case http.StatusPreconditionFailed:
		return "", "", ErrPreconditionFailed
	default:
		return "", "", c.problemError(resp)
	}
}

// ValidateConfig POSTs /config:validate. Network or server errors
// surface as Go errors; a structurally-bad config returns a non-nil
// result with valid=false and a populated Errors slice.
func (c *Client) ValidateConfig(ctx context.Context, config string) (*ValidationResult, error) {
	body, err := json.Marshal(configEnvelope{Config: config})
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/config:validate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.problemError(resp)
	}
	var vr ValidationResult
	limitedBody := io.LimitReader(resp.Body, 10*1024*1024)
	if err := json.NewDecoder(limitedBody).Decode(&vr); err != nil {
		return nil, fmt.Errorf("aperture: decode validate-config: %w", err)
	}
	return &vr, nil
}

// ParseConfigDocument parses a HuJSON config string returned by
// GetConfig into the typed Config struct. HuJSON allows comments and
// trailing commas; we strip those via hujson.Standardize before
// json.Unmarshal so callers don't have to depend on the hujson
// package directly.
func ParseConfigDocument(s string) (*Config, error) {
	pure, err := hujson.Standardize([]byte(s))
	if err != nil {
		return nil, fmt.Errorf("aperture: standardize hujson: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(pure, &cfg); err != nil {
		return nil, fmt.Errorf("aperture: unmarshal config: %w", err)
	}
	return &cfg, nil
}

// MarshalConfigDocument returns a JSON-encoded form of the Config
// suitable for SetConfig. JSON is a strict subset of HuJSON, so we
// emit JSON to keep things deterministic — comment preservation can
// come later if it turns out to matter.
func MarshalConfigDocument(c *Config) (string, error) {
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if c.cfg.Endpoint == "" {
		return nil, errors.New("aperture: endpoint not configured")
	}
	endpoint := strings.TrimRight(c.cfg.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, body)
	if err != nil {
		return nil, err
	}
	if c.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", c.cfg.UserAgent)
	}
	return req, nil
}

// problemError consumes the response body and returns an error formed
// from an RFC 9457 Problem Details document, falling back to a plain
// HTTP error when the body isn't problem+json.
func (c *Client) problemError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	var pd struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(raw, &pd); err == nil && pd.Title != "" {
		msg := pd.Title
		if pd.Detail != "" {
			msg = msg + ": " + pd.Detail
		}
		return fmt.Errorf("aperture %d: %s", pd.Status, msg)
	}
	return fmt.Errorf("aperture: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
}
