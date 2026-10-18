package aperture

import (
	"encoding/json"
	"reflect"
	"testing"
)

func tree(t *testing.T, doc string) map[string]any {
	t.Helper()
	d, err := ParseDocument(doc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	return d.tree
}

func rendered(t *testing.T, d *Document) map[string]any {
	t.Helper()
	s, err := d.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	return tree(t, s)
}

func applyUnchanged(t *testing.T, doc string) *Document {
	t.Helper()
	d, err := ParseDocument(doc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	cfg, err := d.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if err := d.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return d
}

func TestApplyKeepsKeysTheConfigDoesNotModel(t *testing.T) {
	doc := `{
		"exporters": {"s3": {"bucket": "b"}},
		"future_top_level": [1, 2],
		"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "future_provider_key": {"x": true}}},
		"hooks": {"h": {"url": "https://h.example", "future_hook_key": 1}},
		"quotas": {"q": {"capacity": "$1.00", "rate": "$1.00/day", "on_exceed": "reject", "future_quota_key": "x"}},
		"grants": [{
			"src": ["*"],
			"dst": ["tag:x"],
			"app": {
				"tailscale.com/cap/aperture": [{"role": "admin", "connectors": ["aperture/**"]}],
				"example.com/cap/other": [{"x": 1}]
			}
		}]
	}`

	got := rendered(t, applyUnchanged(t, doc))

	if want := tree(t, doc); !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("document changed:\n%s", gotJSON)
	}
}

func TestApplyKeepsUnmodelledKeysOfElementsAtTheSamePosition(t *testing.T) {
	d, err := ParseDocument(`{"grants": [
		{"src": ["a"], "app": {"tailscale.com/cap/aperture": [{"role": "user", "connectors": ["x"]}, {"models": "**", "connectors": ["y"]}]}}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Grants: []Grant{{
		Src: []string{"b"},
		App: GrantApp{Aperture: []GrantCapability{{Role: "admin"}, {Models: "anthropic/**"}}},
	}}}

	if err := d.Apply(cfg); err != nil {
		t.Fatal(err)
	}

	want := tree(t, `{"grants": [
		{"src": ["b"], "app": {"tailscale.com/cap/aperture": [{"role": "admin", "connectors": ["x"]}, {"models": "anthropic/**", "connectors": ["y"]}]}}
	]}`)
	if got := rendered(t, d); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestApplyRemovesModelledKeysTheConfigOmits(t *testing.T) {
	d, err := ParseDocument(`{
		"auto_cost_basis": true,
		"providers": {"gone": {"baseurl": "https://gone.example", "models": []}, "kept": {"baseurl": "https://k.example", "models": ["m"], "name": "Old", "preference": 3}},
		"hooks": {"h": {"url": "https://h.example"}},
		"exporters": {"keep": true}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Providers: map[string]Provider{"kept": {BaseURL: "https://k.example", Models: []string{"m"}}}}

	if err := d.Apply(cfg); err != nil {
		t.Fatal(err)
	}

	want := tree(t, `{
		"providers": {"kept": {"baseurl": "https://k.example", "models": ["m"]}},
		"exporters": {"keep": true}
	}`)
	if got := rendered(t, d); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestApplySetsModelledKeysFromTheConfig(t *testing.T) {
	d := NewDocument()
	cfg := &Config{
		Providers: map[string]Provider{"a": {BaseURL: "https://a.example", Models: []string{"m"}, Name: "A"}},
		Hooks:     map[string]Hook{"h": {URL: "https://h.example", Timeout: "5s"}},
	}

	if err := d.Apply(cfg); err != nil {
		t.Fatal(err)
	}

	want := tree(t, `{
		"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "name": "A"}},
		"hooks": {"h": {"url": "https://h.example", "timeout": "5s"}}
	}`)
	if got := rendered(t, d); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestApplyKeepsEmptyStringsTheConfigOmits(t *testing.T) {
	doc := `{"providers": {"a": {"baseurl": "https://a.example", "models": [], "apikey": ""}}}`

	got := rendered(t, applyUnchanged(t, doc))

	if want := tree(t, doc); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestApplyDoesNotKeepANonEmptyAPIKeyTheConfigOmits(t *testing.T) {
	d, err := ParseDocument(`{"providers": {"a": {"baseurl": "https://a.example", "models": [], "apikey": "secret"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Providers: map[string]Provider{"a": {BaseURL: "https://a.example", Models: []string{}}}}

	if err := d.Apply(cfg); err != nil {
		t.Fatal(err)
	}

	p := rendered(t, d)["providers"].(map[string]any)["a"].(map[string]any)
	if _, ok := p["apikey"]; ok {
		t.Errorf("apikey kept: %v", p)
	}
}

func TestApplyKeepsNumbersExactly(t *testing.T) {
	doc := `{"exporters": {"big": 9007199254740993, "ratio": 1.10}}`

	s, err := applyUnchanged(t, doc).String()
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Exporters struct{ Big, Ratio json.Number }
	}
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatal(err)
	}
	if got.Exporters.Big != "9007199254740993" || got.Exporters.Ratio != "1.10" {
		t.Errorf("numbers changed: %+v", got.Exporters)
	}
}

func TestParseDocumentAcceptsHuJSON(t *testing.T) {
	if _, err := ParseDocument("// comment\n{\"grants\": [],}"); err != nil {
		t.Errorf("ParseDocument: %v", err)
	}
}

func TestParseDocumentRejectsANonObject(t *testing.T) {
	if _, err := ParseDocument(`[1]`); err == nil {
		t.Error("want an error for an array document")
	}
}
