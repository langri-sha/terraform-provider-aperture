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
		Path:       path.Root("endpoint"),
		ConfigValue: types.StringValue(value),
	}
}

func newNullStringRequest() validator.StringRequest {
	return validator.StringRequest{
		Path:       path.Root("endpoint"),
		ConfigValue: types.StringNull(),
	}
}

func TestEndpointProtocolValidator_RejectsHTTP(t *testing.T) {
	v := endpointProtocolValidator{}
	req := newStringRequest("http://example.com/aperture")
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for http:// protocol, got none")
	}

	// Verify error message is helpful
	if len(resp.Diagnostics) > 0 {
		errMsg := resp.Diagnostics[0].Summary()
		if errMsg != "Invalid Endpoint Protocol" {
			t.Errorf("error summary = %q, want 'Invalid Endpoint Protocol'", errMsg)
		}
	}
}

func TestEndpointProtocolValidator_RejectsFTP(t *testing.T) {
	v := endpointProtocolValidator{}
	req := newStringRequest("ftp://example.com/aperture")
	resp := &validator.StringResponse{}

	v.ValidateString(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for ftp:// protocol, got none")
	}
}

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
		"https://example.com/aperture",
		"https://example.com/aperture/",
		"https://sub.example.com/aperture",
		"https://localhost:8080/aperture",
		"https://ai.tail396699.ts.net/aperture",
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
		"://invalid",                          // Missing scheme
		"http://[invalid]",                    // Invalid IPv6 address
		"https://example.com:invalid/path",    // Invalid port
		"\n\ninjection",                       // Control character in URL
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
		"https://example.com/aperture",
		"https://sub.domain.example.com/aperture",
		"https://localhost:8080/aperture",
		"https://example.com:443/aperture",
		"https://192.168.1.1/aperture",
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
			wantDesc: "endpoint must use https:// protocol",
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
func TestCombinedValidation_HTTPWithAperture(t *testing.T) {
	endpoint := "http://example.com/aperture"

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
		t.Error("Protocol validator should reject http://")
	}
}

func TestCombinedValidation_HTTPSWithoutPath(t *testing.T) {
	endpoint := "https://example.com"

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
		t.Error("Protocol validator should accept https://")
	}

	// Path validator should fail
	pathResp := &validator.StringResponse{}
	endpointPathValidator{}.ValidateString(context.Background(), newStringRequest(endpoint), pathResp)
	if !pathResp.Diagnostics.HasError() {
		t.Error("Path validator should require /aperture suffix")
	}
}

func TestCombinedValidation_Valid(t *testing.T) {
	endpoint := "https://ai.example.com/aperture"

	validators := []validator.String{
		endpointURLValidator{},
		endpointProtocolValidator{},
		endpointPathValidator{},
	}

	req := newStringRequest(endpoint)
	for _, v := range validators {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%T should accept %q: %v", v, endpoint, resp.Diagnostics)
		}
	}
}
