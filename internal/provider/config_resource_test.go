package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/langri-sha/aperture/internal/aperture"
)

func TestToAPIConfig_OmitsAbsentFields(t *testing.T) {
	m := &configResourceModel{
		Providers: map[string]providerEntry{
			"openai": {
				BaseURL: types.StringValue("https://api.openai.com/v1"),
				Models:  []types.String{types.StringValue("openai/gpt-5.5")},
			},
		},
	}
	c, err := toAPIConfig(m)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}
	p, ok := c.Providers["openai"]
	if !ok {
		t.Fatalf("missing openai provider: %#v", c.Providers)
	}
	if p.APIKey != "" {
		t.Errorf("apikey leaked into wire form: %q", p.APIKey)
	}
	if c.AutoCostBasis != nil {
		t.Errorf("auto_cost_basis leaked into wire form: %v", *c.AutoCostBasis)
	}
}

func TestToAPIConfig_GrantShape(t *testing.T) {
	m := &configResourceModel{
		Grants: []grantEntry{
			{
				Src: []types.String{types.StringValue("*")},
				Capabilities: []capabilityEntry{
					{Role: types.StringValue("user")},
					{Models: types.StringValue("openai/**")},
				},
			},
		},
	}
	c, err := toAPIConfig(m)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}
	body, err := aperture.MarshalConfigDocument(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Aperture grants live under app["tailscale.com/cap/aperture"].
	if !strings.Contains(body, `"tailscale.com/cap/aperture"`) {
		t.Errorf("expected canonical capability key in output: %s", body)
	}
}

func TestToAPIConfig_GrantWithoutRoleOrModelsErrors(t *testing.T) {
	m := &configResourceModel{
		Grants: []grantEntry{
			{
				Src:          []types.String{types.StringValue("*")},
				Capabilities: []capabilityEntry{{}},
			},
		},
	}
	if _, err := toAPIConfig(m); err == nil {
		t.Fatal("expected error for empty capability, got nil")
	}
}

func TestRoundTrip_PreservesShape(t *testing.T) {
	in := `{
  "providers": {
    "openai": {
      "baseurl": "https://api.openai.com/v1",
      "models": ["openai/gpt-5.5"]
    }
  },
  "grants": [
    {
      "src": ["group:developers"],
      "app": {
        "tailscale.com/cap/aperture": [
          {"role": "user"},
          {"models": "**"}
        ]
      }
    }
  ],
  "auto_cost_basis": true
}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	model := fromAPIConfig(doc)
	roundTripped, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}
	if got := roundTripped.Providers["openai"].Models[0]; got != "openai/gpt-5.5" {
		t.Errorf("model lost: %q", got)
	}
	if len(roundTripped.Grants) != 1 || len(roundTripped.Grants[0].App.Aperture) != 2 {
		t.Errorf("grant lost: %+v", roundTripped.Grants)
	}
	if roundTripped.AutoCostBasis == nil || !*roundTripped.AutoCostBasis {
		t.Errorf("auto_cost_basis lost: %v", roundTripped.AutoCostBasis)
	}
}

func TestPreserveSensitiveFromPrior(t *testing.T) {
	prior := configResourceModel{
		Providers: map[string]providerEntry{
			"openai": {
				BaseURL: types.StringValue("https://api.openai.com/v1"),
				APIKey:  types.StringValue("sk-real"),
			},
		},
	}
	server := configResourceModel{
		Providers: map[string]providerEntry{
			"openai": {
				BaseURL: types.StringValue("https://api.openai.com/v1"),
				APIKey:  types.StringValue("sk-***REDACTED***"),
			},
		},
	}
	preserveSensitiveFromPrior(&server, &prior)
	if got := server.Providers["openai"].APIKey.ValueString(); got != "sk-real" {
		t.Errorf("apikey not preserved: got %q, want %q", got, "sk-real")
	}
}
