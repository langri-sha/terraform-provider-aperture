// +build integration

package aperture

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestGetSetConfigRoundtrip tests the full read/update cycle with the mock server.
func TestGetSetConfigRoundtrip(t *testing.T) {
	server := newTestServer(t, &testServerConfig{
		TLS:  true,
		ETag: `"initial-etag"`,
	})
	defer server.Close()

	// Create a client with the test server's certificate skipping
	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	// Step 1: Read initial config
	config, etag, err := c.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if etag != `"initial-etag"` {
		t.Errorf("initial etag = %q, want %q", etag, `"initial-etag"`)
	}
	if !strings.Contains(config, "providers") {
		t.Errorf("config missing providers: %q", config)
	}

	// Step 2: Modify config
	modifiedConfig := `{"providers":{"openai":{"baseurl":"https://api.openai.com/v1","models":["gpt-4"]}}}`

	// Step 3: Write config back with ETag
	saved, newETag, err := c.SetConfig(context.Background(), modifiedConfig, etag)
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if newETag != `"updated-etag"` {
		t.Errorf("new etag = %q, want %q", newETag, `"updated-etag"`)
	}
	if saved == "" {
		t.Error("saved config is empty")
	}
}

// TestPreconditionFailedOnMismatch tests that 412 is returned when ETag mismatches.
func TestPreconditionFailedOnMismatch(t *testing.T) {
	server := newTestServer(t, &testServerConfig{
		TLS:  true,
		ETag: `"correct-etag"`,
	})
	defer server.Close()

	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	// Try to set config with wrong ETag
	_, _, err := c.SetConfig(context.Background(), `{}`, `"wrong-etag"`)
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Errorf("err = %v, want ErrPreconditionFailed", err)
	}
}

// TestRedirectRejectedByClient tests that redirects are not followed.
func TestRedirectRejectedByClient(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"301 Moved Permanently", http.StatusMovedPermanently},
		{"302 Found", http.StatusFound},
		{"307 Temporary Redirect", http.StatusTemporaryRedirect},
		{"308 Permanent Redirect", http.StatusPermanentRedirect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, &testServerConfig{
				TLS:              true,
				RedirectResponse: true,
				RedirectStatus:   tt.status,
				RedirectLocation: "http://attacker.example.com/malicious",
			})
			defer server.Close()

			c := NewClient(ClientConfig{
				Endpoint:   server.URL(),
				HTTPClient: server.Client(),
			})

			// GetConfig should fail because redirect is rejected
			_, _, err := c.GetConfig(context.Background())
			if err == nil {
				t.Error("expected error for rejected redirect, got nil")
			}

			// SetConfig should also fail
			_, _, err = c.SetConfig(context.Background(), `{}`, `"etag"`)
			if err == nil {
				t.Error("expected error for rejected redirect on SetConfig, got nil")
			}

			// ValidateConfig should also fail
			_, err = c.ValidateConfig(context.Background(), `{}`)
			if err == nil {
				t.Error("expected error for rejected redirect on ValidateConfig, got nil")
			}
		})
	}
}

// TestLargeResponseRejected tests that responses > 10MB are truncated and cause errors.
func TestLargeResponseRejected(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testServer, *Client)
	}{
		{
			name: "GetConfig with large response",
			testFunc: func(server *testServer, c *Client) {
				_, _, err := c.GetConfig(context.Background())
				if err == nil {
					t.Error("expected error for large response on GetConfig, got nil")
				}
			},
		},
		{
			name: "SetConfig with large response",
			testFunc: func(server *testServer, c *Client) {
				_, _, err := c.SetConfig(context.Background(), `{}`, `"test"`)
				if err == nil {
					t.Error("expected error for large response on SetConfig, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, &testServerConfig{
				TLS:           true,
				LargeResponse: true,
				Size:          11 * 1024 * 1024, // 11MB
			})
			defer server.Close()

			c := NewClient(ClientConfig{
				Endpoint:   server.URL(),
				HTTPClient: server.Client(),
			})

			tt.testFunc(server, c)
		})
	}
}

// TestInvalidJSONResponse tests that invalid JSON responses cause errors.
func TestInvalidJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testServer, *Client)
	}{
		{
			name: "GetConfig with invalid JSON",
			testFunc: func(server *testServer, c *Client) {
				_, _, err := c.GetConfig(context.Background())
				if err == nil {
					t.Error("expected error for invalid JSON on GetConfig, got nil")
				}
				if !strings.Contains(err.Error(), "decode") {
					t.Errorf("error = %v, want decode error", err)
				}
			},
		},
		{
			name: "SetConfig with invalid JSON",
			testFunc: func(server *testServer, c *Client) {
				_, _, err := c.SetConfig(context.Background(), `{}`, `"test"`)
				if err == nil {
					t.Error("expected error for invalid JSON on SetConfig, got nil")
				}
			},
		},
		{
			name: "ValidateConfig with invalid JSON",
			testFunc: func(server *testServer, c *Client) {
				_, err := c.ValidateConfig(context.Background(), `{}`)
				if err == nil {
					t.Error("expected error for invalid JSON on ValidateConfig, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, &testServerConfig{
				TLS:         true,
				InvalidJSON: true,
			})
			defer server.Close()

			c := NewClient(ClientConfig{
				Endpoint:   server.URL(),
				HTTPClient: server.Client(),
			})

			tt.testFunc(server, c)
		})
	}
}

// TestErrorResponseHandling tests that various HTTP errors are properly handled.
func TestErrorResponseHandling(t *testing.T) {
	tests := []struct {
		name        string
		errorStatus int
	}{
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"403 Forbidden", http.StatusForbidden},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"404 Not Found", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run("GetConfig "+tt.name, func(t *testing.T) {
			server := newTestServer(t, &testServerConfig{
				TLS:           true,
				ErrorResponse: true,
				ErrorStatus:   tt.errorStatus,
			})
			defer server.Close()

			c := NewClient(ClientConfig{
				Endpoint:   server.URL(),
				HTTPClient: server.Client(),
			})

			_, _, err := c.GetConfig(context.Background())
			if err == nil {
				t.Errorf("expected error for %d status, got nil", tt.errorStatus)
			}
		})
	}
}

// TestValidateConfigEndpoint tests the validation endpoint works correctly.
func TestValidateConfigEndpoint(t *testing.T) {
	server := newTestServer(t, &testServerConfig{
		TLS: true,
	})
	defer server.Close()

	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	result, err := c.ValidateConfig(context.Background(), `{"providers":{}}`)
	if err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
	if !result.Valid {
		t.Errorf("valid = false, want true")
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v, want empty", result.Errors)
	}
}

// TestHTTPSByDefault tests that the server uses HTTPS.
func TestHTTPSByDefault(t *testing.T) {
	server := newTestServer(t, nil) // defaults should include TLS
	defer server.Close()

	// The URL should be HTTPS
	if !strings.HasPrefix(server.url, "https://") {
		t.Errorf("server URL = %s, expected HTTPS", server.url)
	}
}

// TestIfMatchHeaderValidation tests that If-Match header is correctly sent.
func TestIfMatchHeaderValidation(t *testing.T) {
	expectedETag := `"expected-etag"`
	headerValidated := false

	server := newTestServer(t, &testServerConfig{
		TLS:  true,
		ETag: expectedETag,
		RequestValidator: func(_ *testing.T, r *http.Request) error {
			if r.Method == http.MethodPut {
				if header := r.Header.Get("If-Match"); header != expectedETag {
					return errors.New("If-Match header mismatch")
				}
				headerValidated = true
			}
			return nil
		},
	})
	defer server.Close()

	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	// SetConfig should include the If-Match header
	_, _, err := c.SetConfig(context.Background(), `{}`, expectedETag)
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if !headerValidated {
		t.Error("If-Match header was not validated")
	}
}

// TestNetworkErrorHandling tests that network errors are properly propagated.
func TestNetworkErrorHandling(t *testing.T) {
	c := NewClient(ClientConfig{
		Endpoint:   "http://invalid-nonexistent-host.local:9999/aperture",
		HTTPClient: &http.Client{},
	})

	_, _, err := c.GetConfig(context.Background())
	if err == nil {
		t.Error("expected network error, got nil")
	}
}

// TestEndpointValidation tests that empty endpoint is caught.
func TestEndpointValidation(t *testing.T) {
	c := NewClient(ClientConfig{
		Endpoint: "", // Empty endpoint
	})

	_, _, err := c.GetConfig(context.Background())
	if err == nil {
		t.Error("expected error for empty endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error = %v, expected 'endpoint' in message", err)
	}
}

// TestContextCancellation tests that context cancellation is respected.
func TestContextCancellation(t *testing.T) {
	server := newTestServer(t, &testServerConfig{
		TLS: true,
	})
	defer server.Close()

	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.GetConfig(ctx)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("error = %v (context.Canceled check: %v)", err, errors.Is(err, context.Canceled))
	}
}

// TestProblemDetailsError tests that RFC 9457 Problem Details errors are parsed.
func TestProblemDetailsError(t *testing.T) {
	server := newTestServer(t, &testServerConfig{
		TLS:           true,
		ErrorResponse: true,
		ErrorStatus:   http.StatusForbidden,
	})
	defer server.Close()

	c := NewClient(ClientConfig{
		Endpoint:   server.URL(),
		HTTPClient: server.Client(),
	})

	_, _, err := c.GetConfig(context.Background())
	if err == nil {
		t.Fatal("expected error for problem details response")
	}

	// The error should mention the status code
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, expected status code in message", err)
	}
}
