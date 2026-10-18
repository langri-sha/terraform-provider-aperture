package fakegateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const adminGrant = `{"src": ["*"], "app": {"tailscale.com/cap/aperture": [{"role": "admin"}]}}`

func document(body string) string {
	return `{"grants": [` + adminGrant + `], ` + body + `}`
}

type response struct {
	status int
	header http.Header
	body   map[string]any
}

func do(t *testing.T, method, url string, header map[string]string, body string) response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := response{status: resp.StatusCode, header: resp.Header}
	_ = json.NewDecoder(resp.Body).Decode(&out.body)
	return out
}

func envelope(t *testing.T, doc string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"config": doc})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGetRedactsEveryAPIKey(t *testing.T) {
	g := New(t, document(`
		"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "apikey": "sk-provider"}},
		"hooks": {"h": {"url": "https://h.example", "apikey": "sk-hook"}}`))

	got := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "")

	config, _ := got.body["config"].(string)
	if strings.Contains(config, "sk-provider") || strings.Contains(config, "sk-hook") {
		t.Errorf("credentials leaked through GET: %s", config)
	}
	if strings.Count(config, RedactedMarker) != 2 {
		t.Errorf("want both apikeys replaced by the marker: %s", config)
	}
}

func TestGetLeavesEmptyAPIKeyEmpty(t *testing.T) {
	g := New(t, document(`"providers": {"a": {"baseurl": "https://a.example", "models": ["m"], "apikey": ""}}`))

	got := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "")

	config, _ := got.body["config"].(string)
	if strings.Contains(config, RedactedMarker) {
		t.Errorf("empty apikey must not be redacted: %s", config)
	}
}

func TestETagIsUnquotedAndChangesWithTheDocument(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	first := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")

	g.Replace(t, document(`"auto_cost_basis": false`))
	second := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")

	if first == "" || strings.HasPrefix(first, `"`) {
		t.Errorf("want an unquoted etag, got %q", first)
	}
	if first == second {
		t.Errorf("etag did not change when the document did: %q", first)
	}
}

func TestPutStoresTheBodyAndRotatesTheETag(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	etag := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")
	next := document(`"auto_cost_basis": false`)

	got := do(t, http.MethodPut, g.Endpoint()+"/config", map[string]string{"If-Match": etag}, envelope(t, next))

	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body = %v", got.status, got.body)
	}
	if g.Document() != next {
		t.Errorf("stored document = %s", g.Document())
	}
	if got.header.Get("ETag") == etag || got.header.Get("ETag") == "" {
		t.Errorf("etag after put = %q, before = %q", got.header.Get("ETag"), etag)
	}
}

func TestPutWithoutIfMatchIsRejected(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))

	got := do(t, http.MethodPut, g.Endpoint()+"/config", nil, envelope(t, document(`"auto_cost_basis": false`)))

	if got.status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d", got.status)
	}
	if len(g.Writes()) != 0 {
		t.Errorf("document was written: %v", g.Writes())
	}
}

func TestPutWithStaleETagIsRejected(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	etag := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")
	g.Replace(t, document(`"auto_cost_basis": false`))

	got := do(t, http.MethodPut, g.Endpoint()+"/config", map[string]string{"If-Match": etag}, envelope(t, document(`"auto_cost_basis": true`)))

	if got.status != http.StatusPreconditionFailed {
		t.Errorf("status = %d", got.status)
	}
}

func TestPutOfInvalidDocumentIsRejected(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	etag := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")

	got := do(t, http.MethodPut, g.Endpoint()+"/config", map[string]string{"If-Match": etag}, envelope(t, document(`"bogus": 1`)))

	if got.status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d", got.status)
	}
	if len(g.Writes()) != 0 {
		t.Errorf("document was written: %v", g.Writes())
	}
}

func TestValidateReportsTheShapesTheGatewayRejects(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"unknown provider key":     {`"providers": {"a": {"baseurl": "https://a.example", "models": [], "bogus": 1}}`, `unknown config key "providers.a.bogus"`},
		"map add_headers":          {`"providers": {"a": {"baseurl": "https://a.example", "models": [], "add_headers": {"X": "y"}}}`, "providers.a.add_headers: must be an array of strings"},
		"numeric quota":            {`"quotas": {"q": {"capacity": 10, "rate": 5, "on_exceed": "reject"}}`, "cannot unmarshal float64"},
		"undollared quota":         {`"quotas": {"q": {"capacity": "10", "rate": "$5.00/day", "on_exceed": "reject"}}`, "dollar amount must start"},
		"override without apikey":  {`"providers": {"a": {"baseurl": "https://a.example", "models": [], "auth_mode": "override"}}`, `auth_mode "override" requires a non-empty apikey`},
		"hook without url":         {`"hooks": {"h": {}}`, "hook h has no url configured"},
		"invalid hook fail_policy": {`"hooks": {"h": {"url": "https://h.example", "fail_policy": "open"}}`, "invalid fail_policy"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			g := New(t, document(`"auto_cost_basis": true`))

			got := do(t, http.MethodPost, g.Endpoint()+"/config:validate", nil, envelope(t, document(tc.body)))

			errs, _ := got.body["errors"].([]any)
			if got.body["valid"] != false || !strings.Contains(toString(errs), tc.want) {
				t.Errorf("valid = %v, errors = %v, want one containing %q", got.body["valid"], errs, tc.want)
			}
		})
	}
}

func TestValidateRequiresAnAdminGrant(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	body := `{"grants": [{"src": ["*"], "app": {"tailscale.com/cap/aperture": [{"models": "**"}]}}]}`

	got := do(t, http.MethodPost, g.Endpoint()+"/config:validate", nil, envelope(t, body))

	errs, _ := got.body["errors"].([]any)
	if got.body["valid"] != false || !strings.Contains(toString(errs), "role:admin") {
		t.Errorf("valid = %v, errors = %v", got.body["valid"], errs)
	}
}

func TestValidateAcceptsCommentsAndTrailingCommas(t *testing.T) {
	g := New(t, document(`"auto_cost_basis": true`))
	body := "// note\n{\"grants\": [" + adminGrant + ",],}"

	got := do(t, http.MethodPost, g.Endpoint()+"/config:validate", nil, envelope(t, body))

	if got.body["valid"] != true {
		t.Errorf("errors = %v", got.body["errors"])
	}
}

func toString(v any) string {
	errs, _ := v.([]any)
	lines := make([]string, 0, len(errs))
	for _, e := range errs {
		lines = append(lines, fmt.Sprint(e))
	}
	return strings.Join(lines, "\n")
}
