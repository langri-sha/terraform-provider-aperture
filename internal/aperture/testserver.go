// +build integration

package aperture

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handleGetConfig processes GET /aperture/config requests
func handleGetConfig(w http.ResponseWriter, r *http.Request, t *testing.T, cfg *testServerConfig) {
	if cfg.RequestValidator != nil {
		if err := cfg.RequestValidator(t, r); err != nil {
			t.Errorf("request validation failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if cfg.RedirectResponse {
		w.Header().Set("Location", cfg.RedirectLocation)
		w.WriteHeader(cfg.RedirectStatus)
		fmt.Fprintf(w, "Redirect to %s", cfg.RedirectLocation)
		return
	}

	if cfg.ErrorResponse {
		w.WriteHeader(cfg.ErrorStatus)
		if cfg.ErrorStatus == http.StatusPreconditionFailed {
			fmt.Fprintf(w, `{"title":"Precondition Failed","status":%d}`, cfg.ErrorStatus)
		} else {
			fmt.Fprintf(w, `{"title":"Error","status":%d}`, cfg.ErrorStatus)
		}
		return
	}

	w.Header().Set("ETag", cfg.ETag)
	w.Header().Set("Content-Type", "application/json")

	if cfg.LargeResponse {
		// Return a response larger than 10MB
		largeValue := strings.Repeat(`x`, cfg.Size-100) // Leave room for JSON structure
		env := configEnvelope{Config: largeValue}
		data, _ := json.Marshal(env)
		w.Write(data)
		return
	}

	if cfg.InvalidJSON {
		w.Write([]byte(`{"config": invalid json!}`))
		return
	}

	// Normal response
	env := configEnvelope{Config: `{"providers":{}}`}
	data, _ := json.Marshal(env)
	w.Write(data)
}

// handlePutConfig processes PUT /aperture/config requests
func handlePutConfig(w http.ResponseWriter, r *http.Request, t *testing.T, cfg *testServerConfig) {
	if cfg.RequestValidator != nil {
		if err := cfg.RequestValidator(t, r); err != nil {
			t.Errorf("request validation failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Check for If-Match header (simulate precondition validation)
	ifMatch := r.Header.Get("If-Match")
	if ifMatch != "" && ifMatch != cfg.ETag {
		w.WriteHeader(http.StatusPreconditionFailed)
		fmt.Fprintf(w, `{"title":"Precondition Failed","status":412}`)
		return
	}

	if cfg.RedirectResponse {
		w.Header().Set("Location", cfg.RedirectLocation)
		w.WriteHeader(cfg.RedirectStatus)
		return
	}

	if cfg.ErrorResponse {
		w.WriteHeader(cfg.ErrorStatus)
		fmt.Fprintf(w, `{"title":"Error","status":%d}`, cfg.ErrorStatus)
		return
	}

	w.Header().Set("ETag", `"updated-etag"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	env := configEnvelope{Config: `{"providers":{}}`}
	data, _ := json.Marshal(env)
	w.Write(data)
}

// testServerConfig configures the behavior of testServer.
type testServerConfig struct {
	// LargeResponse enables returning a response larger than 10MB.
	LargeResponse bool
	Size          int // Size of large response in bytes (default 11MB)

	// RedirectResponse enables returning a redirect instead of success.
	RedirectResponse bool
	RedirectStatus   int    // HTTP status code (301, 302, 307, 308)
	RedirectLocation string // Location header value

	// InvalidJSON makes the server return malformed JSON.
	InvalidJSON bool

	// ErrorResponse makes the server return a specific HTTP status.
	ErrorResponse bool
	ErrorStatus   int // HTTP status code

	// ETag to return in response headers.
	ETag string

	// TLS enables HTTPS server (default true).
	TLS bool

	// RequestValidator is an optional callback to validate request properties.
	RequestValidator func(*testing.T, *http.Request) error
}

// testServer is a mock Aperture server for testing.
type testServer struct {
	t      *testing.T
	srv    *httptest.Server
	cfg    *testServerConfig
	url    string
	client *http.Client
}

// newTestServer creates a new mock Aperture server with the given configuration.
func newTestServer(t *testing.T, cfg *testServerConfig) *testServer {
	if cfg == nil {
		cfg = &testServerConfig{
			TLS:  true,
			ETag: `"default-etag"`,
		}
	}

	// Set defaults
	if cfg.Size == 0 && cfg.LargeResponse {
		cfg.Size = 11 * 1024 * 1024 // 11MB
	}
	if cfg.RedirectStatus == 0 && cfg.RedirectResponse {
		cfg.RedirectStatus = http.StatusMovedPermanently
	}
	if cfg.ErrorStatus == 0 && cfg.ErrorResponse {
		cfg.ErrorStatus = http.StatusInternalServerError
	}
	if cfg.ETag == "" {
		cfg.ETag = `"test-etag"`
	}

	mux := http.NewServeMux()

	// GET/PUT /aperture/config
	mux.HandleFunc("/aperture/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetConfig(w, r, t, cfg)
		} else if r.Method == http.MethodPut {
			handlePutConfig(w, r, t, cfg)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// POST /aperture/config:validate
	mux.HandleFunc("/aperture/config:validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if cfg.RequestValidator != nil {
			if err := cfg.RequestValidator(t, r); err != nil {
				t.Errorf("request validation failed: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		if cfg.RedirectResponse {
			w.Header().Set("Location", cfg.RedirectLocation)
			w.WriteHeader(cfg.RedirectStatus)
			return
		}

		if cfg.ErrorResponse {
			w.WriteHeader(cfg.ErrorStatus)
			fmt.Fprintf(w, `{"title":"Error","status":%d}`, cfg.ErrorStatus)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if cfg.InvalidJSON {
			w.Write([]byte(`{"valid": not json}`))
			return
		}

		if cfg.LargeResponse {
			// Return a large validation response
			largeErrors := make([]string, cfg.Size/100) // Create many error strings
			for i := range largeErrors {
				largeErrors[i] = fmt.Sprintf("error %d: configuration invalid", i)
			}
			result := ValidationResult{
				Valid:  false,
				Errors: largeErrors,
			}
			data, _ := json.Marshal(result)
			w.Write(data)
			return
		}

		// Normal response
		result := ValidationResult{
			Valid:  true,
			Errors: []string{},
		}
		data, _ := json.Marshal(result)
		w.Write(data)
	})

	var srv *httptest.Server
	if cfg.TLS {
		srv = httptest.NewUnstartedServer(mux)
		srv.TLS = &tls.Config{}
		srv.StartTLS()
	} else {
		srv = httptest.NewServer(mux)
	}

	ts := &testServer{
		t:   t,
		srv: srv,
		cfg: cfg,
		url: srv.URL,
	}

	// Create an HTTP client that doesn't verify TLS (for testing)
	if cfg.TLS {
		ts.client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	} else {
		ts.client = &http.Client{}
	}

	return ts
}

// URL returns the base URL of the test server with the /aperture prefix.
func (ts *testServer) URL() string {
	return ts.url + "/aperture"
}

// Client returns an HTTP client configured for this server.
// For TLS servers, it skips certificate verification.
func (ts *testServer) Client() *http.Client {
	return ts.client
}

// Close shuts down the test server.
func (ts *testServer) Close() {
	ts.srv.Close()
}

// SetLargeResponse configures the server to return a large response.
func (ts *testServer) SetLargeResponse(size int) {
	ts.cfg.LargeResponse = true
	ts.cfg.Size = size
}

// SetRedirect configures the server to return a redirect.
func (ts *testServer) SetRedirect(status int, location string) {
	ts.cfg.RedirectResponse = true
	ts.cfg.RedirectStatus = status
	ts.cfg.RedirectLocation = location
}

// SetError configures the server to return an error status.
func (ts *testServer) SetError(status int) {
	ts.cfg.ErrorResponse = true
	ts.cfg.ErrorStatus = status
}

// SetInvalidJSON configures the server to return invalid JSON.
func (ts *testServer) SetInvalidJSON(invalid bool) {
	ts.cfg.InvalidJSON = invalid
}

// SetETag configures the ETag to return in responses.
func (ts *testServer) SetETag(etag string) {
	ts.cfg.ETag = etag
}

// SetRequestValidator configures a callback to validate incoming requests.
func (ts *testServer) SetRequestValidator(validator func(*testing.T, *http.Request) error) {
	ts.cfg.RequestValidator = validator
}
