package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/langri-sha/aperture/internal/aperture"
	"github.com/langri-sha/aperture/internal/fakegateway"
)

func gatewayFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../aperture/testdata/gateway.hujson")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func resourceFor(g *fakegateway.Gateway) *configResource {
	return &configResource{client: aperture.NewClient(aperture.ClientConfig{Endpoint: g.Endpoint()})}
}

func planFromFixture(t *testing.T, doc string) configResourceModel {
	t.Helper()
	cfg, err := aperture.ParseConfigDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	return fromAPIConfig(cfg)
}

func storedTree(t *testing.T, g *fakegateway.Gateway) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(g.Document()), &out); err != nil {
		t.Fatalf("stored document is not JSON: %v", err)
	}
	return out
}

func TestApplyPlan_KeepsKeysTheModelDoesNotCover(t *testing.T) {
	fixture := gatewayFixture(t)
	g := fakegateway.New(t, fixture)
	plan := planFromFixture(t, fixture)
	anthropic := plan.Providers["anthropic"]
	anthropic.Models = append(anthropic.Models, types.StringValue("claude-next"))
	plan.Providers["anthropic"] = anthropic

	var diags diag.Diagnostics
	if !resourceFor(g).applyPlan(context.Background(), &plan, "", &diags) {
		t.Fatalf("applyPlan: %v", diags)
	}

	providers := storedTree(t, g)["providers"].(map[string]any)
	stored := providers["anthropic"].(map[string]any)
	if stored["auth_mode"] != "passthrough" {
		t.Errorf("auth_mode lost: %v", stored)
	}
	if compat := stored["compatibility"].(map[string]any); compat["anthropic_messages"] != true {
		t.Errorf("compatibility lost: %v", stored)
	}
	if models := stored["models"].([]any); models[len(models)-1] != "claude-next" {
		t.Errorf("model not added: %v", models)
	}
	grants := storedTree(t, g)["grants"].([]any)
	caps := grants[1].(map[string]any)["app"].(map[string]any)["tailscale.com/cap/aperture"].([]any)
	if connectors := caps[1].(map[string]any)["connectors"]; connectors == nil {
		t.Errorf("connectors capability lost: %v", caps)
	}
}

func TestApplyPlan_RefusesToWriteOverAnExternalChange(t *testing.T) {
	fixture := gatewayFixture(t)
	g := fakegateway.New(t, fixture)
	r := resourceFor(g)
	_, planned, err := r.client.GetConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g.Replace(t, strings.Replace(fixture, `"gpt-5.5",`, `"gpt-5.5", "gpt-edited",`, 1))
	plan := planFromFixture(t, fixture)

	var diags diag.Diagnostics
	if r.applyPlan(context.Background(), &plan, planned, &diags) {
		t.Fatal("applyPlan wrote over an external change")
	}
	if len(g.Writes()) != 0 {
		t.Errorf("gateway was written: %v", g.Writes())
	}
}

func TestApplyPlan_CreateOverlaysOntoTheExistingDocument(t *testing.T) {
	fixture := gatewayFixture(t)
	g := fakegateway.New(t, fixture)
	plan := configResourceModel{
		Providers: map[string]providerEntry{
			"anthropic": {BaseURL: types.StringValue("https://api.anthropic.com"), Models: []types.String{types.StringValue("claude-opus-5")}},
		},
		Grants: planFromFixture(t, fixture).Grants,
	}

	var diags diag.Diagnostics
	if !resourceFor(g).applyPlan(context.Background(), &plan, "", &diags) {
		t.Fatalf("applyPlan: %v", diags)
	}

	providers := storedTree(t, g)["providers"].(map[string]any)
	if _, ok := providers["codex"]; ok {
		t.Errorf("codex provider not removed: %v", providers)
	}
	if providers["anthropic"].(map[string]any)["auth_mode"] != "passthrough" {
		t.Errorf("auth_mode lost: %v", providers["anthropic"])
	}
}
