package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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

func TestRoundTrip_GrantDestination(t *testing.T) {
	in := `{"grants": [
		{"src": ["*"], "dst": ["tag:ai"], "app": {"tailscale.com/cap/aperture": [{"role": "admin"}]}},
		{"src": ["*"], "app": {"tailscale.com/cap/aperture": [{"models": "**"}]}}
	]}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	if len(out.Grants[0].Dst) != 1 || out.Grants[0].Dst[0] != "tag:ai" {
		t.Errorf("dst = %v", out.Grants[0].Dst)
	}
	if model.Grants[1].Dst != nil {
		t.Errorf("absent dst must read back as null, got %v", model.Grants[1].Dst)
	}
}

func TestRoundTrip_GrantCapabilityFields(t *testing.T) {
	in := `{"grants": [{"src": ["*"], "app": {"tailscale.com/cap/aperture": [
		{"connectors": ["aperture/**", "label:system"]},
		{"add_headers": ["X-Team: platform"], "filterable_headers": ["*"]},
		{"enable_portal": false, "read_metrics": true},
		{"set_cors": true, "cors_paths": ["/v1/**"]}
	]}}]}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	caps := out.Grants[0].App.Aperture
	if len(caps[0].Connectors) != 2 || caps[0].Connectors[1] != "label:system" {
		t.Errorf("connectors = %v", caps[0].Connectors)
	}
	if caps[1].AddHeaders[0] != "X-Team: platform" || caps[1].FilterableHeaders[0] != "*" {
		t.Errorf("headers = %+v", caps[1])
	}
	if caps[2].EnablePortal == nil || *caps[2].EnablePortal || caps[2].ReadMetrics == nil || !*caps[2].ReadMetrics {
		t.Errorf("flags = %+v", caps[2])
	}
	if caps[3].SetCORS == nil || !*caps[3].SetCORS || caps[3].CORSPaths[0] != "/v1/**" {
		t.Errorf("cors = %+v", caps[3])
	}
	if model.Grants[0].Capabilities[2].Connectors != nil {
		t.Errorf("absent connectors must read back as null")
	}
}

func TestRoundTrip_GrantQuotasAndSendHooks(t *testing.T) {
	in := `{"grants": [{"src": ["*"], "app": {"tailscale.com/cap/aperture": [
		{"models": "**", "quotas": [{"bucket": "daily:<user>", "overdrafts": ["pool"]}, {"bucket": "pool"}],
		 "send_hooks": [{"name": "oso", "events": ["tool_call_entire_request"], "send": ["tools", "user_message"]}]}
	]}}]}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	c := out.Grants[0].App.Aperture[0]
	if len(c.Quotas) != 2 || c.Quotas[0].Bucket != "daily:<user>" || c.Quotas[0].Overdrafts[0] != "pool" || c.Quotas[1].Overdrafts != nil {
		t.Errorf("quotas = %+v", c.Quotas)
	}
	if len(c.SendHooks) != 1 || c.SendHooks[0].Name != "oso" || c.SendHooks[0].Events[0] != "tool_call_entire_request" || len(c.SendHooks[0].Send) != 2 {
		t.Errorf("send_hooks = %+v", c.SendHooks)
	}
}

func TestRoundTrip_JSONSections(t *testing.T) {
	in := `{"exporters": {"s3": {"bucket": "b"}}, "flags": {"f": {"value": true}}, "chat_models": [{"model": "**"}]}`
	doc, err := aperture.ParseConfigDocument(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	model := fromAPIConfig(doc)
	out, err := toAPIConfig(&model)
	if err != nil {
		t.Fatalf("toAPIConfig: %v", err)
	}

	if string(out.Exporters) != `{"s3":{"bucket":"b"}}` || string(out.Flags) != `{"f":{"value":true}}` || string(out.ChatModels) != `[{"model":"**"}]` {
		t.Errorf("sections = %s %s %s", out.Exporters, out.Flags, out.ChatModels)
	}
	if !model.Database.IsNull() || len(out.Database) != 0 {
		t.Errorf("absent section must stay null: %v %s", model.Database, out.Database)
	}
}

func TestToAPIConfig_RejectsAJSONSectionThatIsNotJSON(t *testing.T) {
	m := &configResourceModel{Exporters: jsontypes.NewNormalizedValue("{not json")}

	if _, err := toAPIConfig(m); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLeaveUnmanagedSectionsNull(t *testing.T) {
	server := configResourceModel{
		Exporters: jsontypes.NewNormalizedValue(`{"a":1}`),
		Flags:     jsontypes.NewNormalizedValue(`{"f":{"value":true}}`),
	}
	prior := configResourceModel{Flags: jsontypes.NewNormalizedValue(`{"f":{"value":false}}`)}

	leaveUnmanagedSectionsNull(&server, &prior)

	if !server.Exporters.IsNull() {
		t.Errorf("unmanaged exporters = %v", server.Exporters)
	}
	if server.Flags.ValueString() != `{"f":{"value":true}}` {
		t.Errorf("managed flags = %v", server.Flags)
	}
}

func TestSchema_ETagMayChangeOnUpdate(t *testing.T) {
	var resp resource.SchemaResponse
	(&configResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	etag := resp.Schema.Attributes["etag"].(schema.StringAttribute)

	if len(etag.PlanModifiers) != 0 {
		t.Errorf("etag keeps its prior value in the plan, but every write changes it: %v", etag.PlanModifiers)
	}
}
