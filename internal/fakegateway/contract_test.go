package fakegateway

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

type openAPI struct {
	Paths map[string]map[string]struct {
		Parameters []struct {
			Name     string `json:"name"`
			In       string `json:"in"`
			Required bool   `json:"required"`
		} `json:"parameters"`
		Responses map[string]json.RawMessage `json:"responses"`
	} `json:"paths"`
}

func loadOpenAPI(t *testing.T) openAPI {
	t.Helper()
	raw, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec openAPI
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestEveryResponseStatusIsDeclaredByTheOpenAPIDocument(t *testing.T) {
	spec := loadOpenAPI(t)
	g := New(t, document(`"auto_cost_basis": true`))
	etag := do(t, http.MethodGet, g.Endpoint()+"/config", nil, "").header.Get("ETag")

	calls := []struct {
		method, path string
		header       map[string]string
		body         string
		wantStatus   string
	}{
		{http.MethodGet, "/config", nil, "", "200"},
		{http.MethodPut, "/config", map[string]string{"If-Match": etag}, envelope(t, document(`"auto_cost_basis": false`)), "200"},
		{http.MethodPut, "/config", map[string]string{"If-Match": "stale"}, envelope(t, document(`"auto_cost_basis": false`)), "412"},
		{http.MethodPut, "/config", map[string]string{"If-Match": etag}, "{}", "422"},
		{http.MethodPost, "/config:validate", nil, envelope(t, document(`"auto_cost_basis": false`)), "200"},
	}
	for _, c := range calls {
		got := do(t, c.method, g.Endpoint()+c.path, c.header, c.body)
		op, ok := spec.Paths[c.path][strings.ToLower(c.method)]
		if !ok {
			t.Errorf("%s %s is not in the OpenAPI document", c.method, c.path)
			continue
		}
		if got.status != mustAtoi(t, c.wantStatus) {
			t.Errorf("%s %s answered %d, want %s", c.method, c.path, got.status, c.wantStatus)
		}
		if _, ok := op.Responses[strconv.Itoa(got.status)]; !ok {
			t.Errorf("%s %s answered %d, not declared by the OpenAPI document", c.method, c.path, got.status)
		}
	}
}

func TestPutDeclaresIfMatchAsRequired(t *testing.T) {
	spec := loadOpenAPI(t)

	for _, p := range spec.Paths["/config"]["put"].Parameters {
		if p.Name == "If-Match" && p.In == "header" && p.Required {
			return
		}
	}
	t.Error("the OpenAPI document no longer marks If-Match as required on PUT /config")
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
