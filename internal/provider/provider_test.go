package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This helper creates a StringRequest for testing validators.
func newStringRequest(value string) validator.StringRequest {
	return validator.StringRequest{
		Path:        path.Root("endpoint"),
		ConfigValue: types.StringValue(value),
	}
}

func newNullStringRequest() validator.StringRequest {
	return validator.StringRequest{
		Path:        path.Root("endpoint"),
		ConfigValue: types.StringNull(),
	}
}

func TestEndpointProtocolValidator_RejectsUnsupportedSchemes(t *testing.T) {
	v := endpointProtocolValidator{}
	tests := []string{
		"ftp://example.com/aperture",
		"file:///aperture",
		"ws://example.com/aperture",
		"ai.example.com/aperture", // no scheme at all
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			req := newStringRequest(endpoint)
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected error for %q, got none", endpoint)
			}

			// Verify error message is helpful
			if summary := resp.Diagnostics[0].Summary(); summary != "Invalid Endpoint Protocol" {
				t.Errorf("error summary = %q, want 'Invalid Endpoint Protocol'", summary)
			}
		})
	}
}

// Aperture serves its admin API over plain HTTP on the tailnet, so http:// is
// the expected endpoint shape, not a tolerated fallback.
func TestEndpointProtocolValidator_AcceptsHTTP(t *testing.T) {
	v := endpointProtocolValidator{}
	req := newStringRequest("http://ai.tail396699.ts.net/aperture")
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error for http:// protocol: %v", resp.Diagnostics)
	}
}

// Gateways fronted with a Tailscale TLS certificate serve the same admin API
// over https://, so https:// stays valid.
func TestEndpointProtocolValidator_AcceptsHTTPS(t *testing.T) {
	v := endpointProtocolValidator{}
	req := newStringRequest("https://example.com/aperture")
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error for https:// protocol: %v", resp.Diagnostics)
	}
}

func TestEndpointProtocolValidator_SkipsNull(t *testing.T) {
	v := endpointProtocolValidator{}
	req := newNullStringRequest()
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Error("should not validate null value")
	}
}

func TestEndpointPathValidator_RejectsMissingAperturePath(t *testing.T) {
	v := endpointPathValidator{}
	tests := []string{
		"https://example.com",
		"https://example.com/",
		"https://example.com/config",
		"https://example.com/api",
		"https://example.com/aperture-old",
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			req := newStringRequest(endpoint)
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Errorf("expected error for %q (missing /aperture), got none", endpoint)
			}

			// Verify error message includes the actual path
			if len(resp.Diagnostics) > 0 {
				detail := resp.Diagnostics[0].Detail()
				if !strings.Contains(detail, "aperture") {
					t.Errorf("error detail should mention 'aperture': %q", detail)
				}
			}
		})
	}
}

func TestEndpointPathValidator_AcceptsAperturePath(t *testing.T) {
	v := endpointPathValidator{}
	tests := []string{
		"http://example.com/aperture",
		"http://example.com/aperture/",
		"https://sub.example.com/aperture",
		"http://localhost:8080/aperture",
		"http://ai.tail396699.ts.net/aperture",
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			req := newStringRequest(endpoint)
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Errorf("unexpected error for %q: %v", endpoint, resp.Diagnostics)
			}
		})
	}
}

func TestEndpointPathValidator_SkipsNull(t *testing.T) {
	v := endpointPathValidator{}
	req := newNullStringRequest()
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Error("should not validate null value")
	}
}

func TestEndpointURLValidator_RejectsMalformedURLs(t *testing.T) {
	v := endpointURLValidator{}
	tests := []string{
		"://invalid",                       // Missing scheme
		"http://[invalid]",                 // Invalid IPv6 address
		"https://example.com:invalid/path", // Invalid port
		"\n\ninjection",                    // Control character in URL
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			req := newStringRequest(endpoint)
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Errorf("expected error for malformed URL %q, got none", endpoint)
			}
		})
	}
}

func TestEndpointURLValidator_AcceptsValidURLs(t *testing.T) {
	v := endpointURLValidator{}
	tests := []string{
		"http://example.com/aperture",
		"https://sub.domain.example.com/aperture",
		"http://localhost:8080/aperture",
		"https://example.com:443/aperture",
		"http://192.168.1.1/aperture",
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			req := newStringRequest(endpoint)
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Errorf("unexpected error for %q: %v", endpoint, resp.Diagnostics)
			}
		})
	}
}

func TestEndpointURLValidator_SkipsNull(t *testing.T) {
	v := endpointURLValidator{}
	req := newNullStringRequest()
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Error("should not validate null value")
	}
}

func TestValidatorDescriptions(t *testing.T) {
	tests := []struct {
		name     string
		v        validator.String
		wantDesc string
	}{
		{
			name:     "URL validator",
			v:        endpointURLValidator{},
			wantDesc: "endpoint must be a valid URL",
		},
		{
			name:     "Protocol validator",
			v:        endpointProtocolValidator{},
			wantDesc: "endpoint must use http:// or https:// protocol",
		},
		{
			name:     "Path validator",
			v:        endpointPathValidator{},
			wantDesc: "endpoint path must end with /aperture",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := tt.v.Description(context.Background())
			if desc != tt.wantDesc {
				t.Errorf("Description = %q, want %q", desc, tt.wantDesc)
			}

			mdDesc := tt.v.MarkdownDescription(context.Background())
			if mdDesc != tt.wantDesc {
				t.Errorf("MarkdownDescription = %q, want %q", mdDesc, tt.wantDesc)
			}
		})
	}
}

// Test that all validators work together to catch various errors
func TestCombinedValidation_UnsupportedSchemeWithAperture(t *testing.T) {
	endpoint := "ftp://example.com/aperture"

	// URL validator should pass
	urlResp := &validator.StringResponse{}
	endpointURLValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), urlResp)
	if urlResp.Diagnostics.HasError() {
		t.Error("URL validator should accept valid URL")
	}

	// Protocol validator should fail
	protoResp := &validator.StringResponse{}
	endpointProtocolValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), protoResp)
	if !protoResp.Diagnostics.HasError() {
		t.Error("Protocol validator should reject ftp://")
	}

	// Path validator should pass — the /aperture suffix is there
	pathResp := &validator.StringResponse{}
	endpointPathValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), pathResp)
	if pathResp.Diagnostics.HasError() {
		t.Error("Path validator should accept the /aperture suffix")
	}
}

func TestCombinedValidation_HTTPWithoutPath(t *testing.T) {
	endpoint := "http://example.com"

	// URL validator should pass
	urlResp := &validator.StringResponse{}
	endpointURLValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), urlResp)
	if urlResp.Diagnostics.HasError() {
		t.Error("URL validator should accept valid URL")
	}

	// Protocol validator should pass
	protoResp := &validator.StringResponse{}
	endpointProtocolValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), protoResp)
	if protoResp.Diagnostics.HasError() {
		t.Error("Protocol validator should accept http://")
	}

	// Path validator should fail
	pathResp := &validator.StringResponse{}
	endpointPathValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), pathResp)
	if !pathResp.Diagnostics.HasError() {
		t.Error("Path validator should require /aperture suffix")
	}
}

func TestCombinedValidation_Valid(t *testing.T) {
	endpoints := []string{
		"http://ai.tail396699.ts.net/aperture",
		"https://ai.tail396699.ts.net/aperture",
	}

	validators := []validator.String{
		endpointURLValidator{},
		endpointProtocolValidator{},
		endpointPathValidator{},
	}

	for _, endpoint := range endpoints {
		req := newStringRequest(endpoint)
		for _, v := range validators {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("%T should accept %q: %v", v, endpoint, resp.Diagnostics)
			}
		}
	}
}
