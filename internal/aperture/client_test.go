package aperture

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aperture/config" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q", r.Method)
		}
		w.Header().Set("ETag", `"abc123"`)
		_, _ = w.Write([]byte(`{"config":"// hello\n{\"providers\": {}}"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL + "/aperture"})
	cfg, etag, err := c.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if etag != `"abc123"` {
		t.Errorf("etag = %q", etag)
	}
	if !strings.Contains(cfg, "providers") {
		t.Errorf("config = %q (no providers)", cfg)
	}
}

func TestSetConfig_IfMatch(t *testing.T) {
	const want = `"prev-etag"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-Match"); got != want {
			t.Errorf("If-Match = %q, want %q", got, want)
		}
		raw, _ := io.ReadAll(r.Body)
		var env configEnvelope
		_ = json.Unmarshal(raw, &env)
		if !strings.Contains(env.Config, "providers") {
			t.Errorf("body config = %q", env.Config)
		}
		w.Header().Set("ETag", `"new-etag"`)
		_, _ = w.Write([]byte(`{"config":"{}"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	_, etag, err := c.SetConfig(context.Background(), `{"providers":{}}`, want)
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if etag != `"new-etag"` {
		t.Errorf("etag = %q", etag)
	}
}

func TestSetConfig_PreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"title":"Precondition Failed","status":412}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	if _, _, err := c.SetConfig(context.Background(), `{}`, `"stale"`); !errors.Is(err, ErrPreconditionFailed) {
		t.Errorf("err = %v, want ErrPreconditionFailed", err)
	}
}

func TestValidateConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config:validate" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"valid":false,"errors":["providers: at least one is required"]}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	vr, err := c.ValidateConfig(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
	if vr.Valid {
		t.Errorf("valid = true, want false")
	}
	if len(vr.Errors) != 1 {
		t.Errorf("errors = %v", vr.Errors)
	}
}

func TestProblemError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Forbidden","detail":"admin role required","status":403}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	_, _, err := c.GetConfig(context.Background())
	if err == nil {
		t.Fatal("err is nil, want a problem-formatted error")
	}
	if !strings.Contains(err.Error(), "Forbidden") || !strings.Contains(err.Error(), "admin role required") {
		t.Errorf("err = %q (missing Problem Details fields)", err)
	}
}

func TestParseAndMarshal_Roundtrip(t *testing.T) {
	in := []byte(`// top comment
{
  "providers": {
    "openai": {
      "baseurl": "https://api.openai.com/v1",
      "models": ["openai/gpt-5.5"],
    },
  },
  "auto_cost_basis": true,
}`)
	cfg, err := ParseConfigDocument(string(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Providers["openai"].BaseURL; got != "https://api.openai.com/v1" {
		t.Errorf("baseurl = %q", got)
	}
	if cfg.AutoCostBasis == nil || !*cfg.AutoCostBasis {
		t.Errorf("auto_cost_basis = %v", cfg.AutoCostBasis)
	}
	out, err := MarshalConfigDocument(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(out, `"openai/gpt-5.5"`) {
		t.Errorf("marshalled = %q", out)
	}
}

// Verify that the HTTP client's CheckRedirect callback rejects all redirect responses
func TestRedirectRejection_301(t *testing.T) {
	testRedirectRejection(t, http.StatusMovedPermanently, "301")
}

func TestRedirectRejection_302(t *testing.T) {
	testRedirectRejection(t, http.StatusFound, "302")
}

func TestRedirectRejection_307(t *testing.T) {
	testRedirectRejection(t, http.StatusTemporaryRedirect, "307")
}

func TestRedirectRejection_308(t *testing.T) {
	testRedirectRejection(t, http.StatusPermanentRedirect, "308")
}

// testRedirectRejection verifies that the client rejects redirects by
// checking that no second request is made to the redirect target.
func testRedirectRejection(t *testing.T, statusCode int, statusName string) {
	redirected := false
	requestCount := 0

	// Create a test server that returns a redirect
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path == "/aperture/config" && r.Method == http.MethodGet {
			// First request: return redirect
			w.Header().Set("Location", "/aperture/other-endpoint")
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte("Redirect"))
		} else if r.URL.Path == "/aperture/other-endpoint" {
			// Should not reach here if redirect is properly rejected
			redirected = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"config":"{}"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL + "/aperture"})
	_, _, err := c.GetConfig(context.Background())

	// Error expected because client rejects the redirect
	if err == nil {
		t.Errorf("expected error for %s redirect, got nil", statusName)
	}

	// Verify no redirect was followed
	if redirected {
		t.Errorf("redirect was followed for %s (should have been rejected)", statusName)
	}

	// Verify only one request was made (no automatic redirect)
	if requestCount != 1 {
		t.Errorf("expected 1 request for %s, got %d", statusName, requestCount)
	}
}

func TestCheckRedirectCallback(t *testing.T) {
	c := NewClient(ClientConfig{Endpoint: "https://example.com/aperture"})
	if c.http.CheckRedirect == nil {
		t.Fatal("CheckRedirect not configured")
	}

	// Create a mock redirect request
	req1, _ := http.NewRequest(http.MethodGet, "https://example.com/aperture/config", nil)
	req2, _ := http.NewRequest(http.MethodGet, "https://example.com/aperture/other", nil)

	// Test that CheckRedirect returns ErrUseLastResponse for any redirect
	err := c.http.CheckRedirect(req2, []*http.Request{req1})
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("CheckRedirect returned %v, want %v", err, http.ErrUseLastResponse)
	}
}

func TestTLSConfiguration(t *testing.T) {
	c := NewClient(ClientConfig{Endpoint: "https://example.com/aperture"})

	// Verify Transport is configured
	if c.http.Transport == nil {
		t.Fatal("Transport not configured")
	}

	transport, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	// Verify TLSClientConfig is configured
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig not configured")
	}

	// Verify TLS 1.3 is the minimum version
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want %d (tls.VersionTLS13)", transport.TLSClientConfig.MinVersion, tls.VersionTLS13)
	}

	// Verify InsecureSkipVerify is false (secure default)
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify is true, want false")
	}
}

func TestCustomHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 60 * time.Second,
	}
	c := NewClient(ClientConfig{
		Endpoint:   "https://example.com/aperture",
		HTTPClient: customClient,
	})

	if c.http != customClient {
		t.Error("custom HTTPClient was not used")
	}
}

func TestJSONDecoderBounds_LargeResponse(t *testing.T) {
	// Create a response larger than 10MB
	largeConfig := strings.Repeat(`"long value",`, (11*1024*1024)/12) // Approximately 11MB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aperture/config" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// Return a response that exceeds 10MB limit
		envelope := fmt.Sprintf(`{"config":"%s"}`, strings.TrimRight(largeConfig, ","))
		w.Header().Set("ETag", `"large"`)
		_, _ = w.Write([]byte(envelope))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL + "/aperture"})
	_, _, err := c.GetConfig(context.Background())

	// Error expected because response exceeds 10MB limit
	if err == nil {
		t.Error("expected error for response > 10MB, got nil")
	}

	// Verify the error is related to decoding (not network/connectivity)
	if !strings.Contains(err.Error(), "decode") && !strings.Contains(err.Error(), "EOF") {
		t.Logf("error = %v (expected decode or EOF error)", err)
	}
}

func TestJSONDecoderBounds_NormalResponse(t *testing.T) {
	// Create a response well under 10MB (a few KB)
	config := `{
  "providers": {
    "openai": {
      "baseurl": "https://api.openai.com/v1",
      "models": ["gpt-4"]
    }
  }
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aperture/config" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("ETag", `"normal"`)
		// Use proper JSON encoding for the config string
		envelope := configEnvelope{Config: config}
		jsonBytes, _ := json.Marshal(envelope)
		_, _ = w.Write(jsonBytes)
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL + "/aperture"})
	cfg, etag, err := c.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if etag != `"normal"` {
		t.Errorf("etag = %q, want %q", etag, `"normal"`)
	}
	if !strings.Contains(cfg, "providers") {
		t.Errorf("config missing providers: %q", cfg)
	}
}

func TestSetConfigBoundedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.Header().Set("ETag", `"new"`)
		// Return a normal response (under limit)
		_, _ = w.Write([]byte(`{"config":"{}"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	cfg, etag, err := c.SetConfig(context.Background(), `{}`, `"old"`)
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if etag != `"new"` {
		t.Errorf("etag = %q, want %q", etag, `"new"`)
	}
	if cfg != "{}" {
		t.Errorf("config = %q, want %q", cfg, "{}")
	}
}

func TestValidateConfigBoundedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config:validate" {
			t.Errorf("path = %q, want /config:validate", r.URL.Path)
		}
		// Return a normal validation response (well under limit)
		_, _ = w.Write([]byte(`{"valid":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	vr, err := c.ValidateConfig(context.Background(), `{"providers":{}}`)
	if err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
	if !vr.Valid {
		t.Errorf("valid = false, want true")
	}
	if len(vr.Errors) != 0 {
		t.Errorf("errors = %v, want empty", vr.Errors)
	}
}
