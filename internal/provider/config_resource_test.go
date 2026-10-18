package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

func TestToAPIConfig_GrantCapabilityWithoutRoleOrModelsIsKept(t *testing.T) {
	m := &configResourceModel{
		Grants: []grantEntry{
			{
				Src:          []types.String{types.StringValue("*")},
				Capabilities: []capabilityEntry{{}},
			},
		},
	}
	c, err := toAPIConfig(m)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}
	if len(c.Grants) != 1 || len(c.Grants[0].App.Aperture) != 1 {
		t.Errorf("capability dropped: %+v", c.Grants)
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

func TestFromAPIConfig_NeverReadsAPIKeys(t *testing.T) {
	in := `{
		"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "apikey": "marker-from-gateway"}},
		"hooks": {"h": {"url": "https://h.example", "apikey": "marker-from-gateway"}}
	}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)

	if !model.Providers["a"].APIKey.IsNull() || !model.Hooks["h"].APIKey.IsNull() {
		t.Errorf("apikey read from the gateway: %+v %+v", model.Providers["a"].APIKey, model.Hooks["h"].APIKey)
	}
}

func TestPreserveSensitiveFromPrior_KeepsAPIKeysFromPriorState(t *testing.T) {
	prior := configResourceModel{
		Providers: map[string]providerEntry{"openai": {APIKey: types.StringValue("sk-real")}},
		Hooks:     map[string]hookEntry{"audit": {APIKey: types.StringValue("hook-real")}},
	}
	server := configResourceModel{
		Providers: map[string]providerEntry{"openai": {BaseURL: types.StringValue("https://api.openai.com")}},
		Hooks:     map[string]hookEntry{"audit": {URL: types.StringValue("https://audit.example")}},
	}

	preserveSensitiveFromPrior(&server, &prior)

	if got := server.Providers["openai"].APIKey.ValueString(); got != "sk-real" {
		t.Errorf("provider apikey = %q", got)
	}
	if got := server.Hooks["audit"].APIKey.ValueString(); got != "hook-real" {
		t.Errorf("hook apikey = %q", got)
	}
}

func TestPreserveSensitiveFromPrior_LeavesAPIKeyNullWithoutPriorState(t *testing.T) {
	server := configResourceModel{
		Providers: map[string]providerEntry{"openai": {BaseURL: types.StringValue("https://api.openai.com")}},
	}

	preserveSensitiveFromPrior(&server, &configResourceModel{})

	if !server.Providers["openai"].APIKey.IsNull() {
		t.Errorf("apikey = %v", server.Providers["openai"].APIKey)
	}
}

func TestRoundTrip_QuotasAreDollarStrings(t *testing.T) {
	in := `{"quotas": {"daily:<user>": {"capacity": "$10.00", "rate": "$5.00/day", "on_exceed": "reject"}}}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	q := out.Quotas["daily:<user>"]
	if q.Capacity != "$10.00" || q.Rate != "$5.00/day" || q.OnExceed != "reject" {
		t.Errorf("quota changed: %+v", q)
	}
}

func TestRoundTrip_ProviderAddHeadersAreHeaderLines(t *testing.T) {
	in := `{"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "add_headers": ["X-Team: platform", "X-Env: prod"]}}}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	got := out.Providers["a"].AddHeaders
	if len(got) != 2 || got[0] != "X-Team: platform" || got[1] != "X-Env: prod" {
		t.Errorf("add_headers changed: %v", got)
	}
}

func TestSchema_AuthorizationIsNotSensitive(t *testing.T) {
	var resp resource.SchemaResponse
	(&configResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	providers := resp.Schema.Attributes["providers"].(schema.MapNestedAttribute).NestedObject.Attributes
	hooks := resp.Schema.Attributes["hooks"].(schema.MapNestedAttribute).NestedObject.Attributes
	for name, attrs := range map[string]map[string]schema.Attribute{"providers": providers, "hooks": hooks} {
		if attrs["authorization"].IsSensitive() {
			t.Errorf("%s.authorization is sensitive", name)
		}
		if !attrs["apikey"].IsSensitive() {
			t.Errorf("%s.apikey is not sensitive", name)
		}
	}
}

func TestRoundTrip_ProviderAuthModeCompatibilityAndModelCostMap(t *testing.T) {
	in := `{"providers": {"a": {
		"baseurl": "https://a.example",
		"models": ["m"],
		"auth_mode": "passthrough",
		"compatibility": {"anthropic_messages": true, "openai_chat": false},
		"cost_basis": "anthropic",
		"model_cost_map": [{"match": "x-*", "as": "claude-opus-5", "adjustment": 1.1}, {"match": "y-*", "as": "claude-haiku-5"}]
	}}}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	p := out.Providers["a"]
	if p.AuthMode != "passthrough" {
		t.Errorf("auth_mode = %q", p.AuthMode)
	}
	if c := p.Compatibility; c == nil || c.AnthropicMessages == nil || !*c.AnthropicMessages || c.OpenAIChat == nil || *c.OpenAIChat || c.OpenAIResponses != nil {
		t.Errorf("compatibility = %+v", c)
	}
	if len(p.ModelCostMap) != 2 || *p.ModelCostMap[0].Adjustment != 1.1 || p.ModelCostMap[1].Adjustment != nil {
		t.Errorf("model_cost_map = %+v", p.ModelCostMap)
	}
}
