// Package fakegateway is an in-memory stand-in for an Aperture gateway's
// admin API, shaped after the OpenAPI document a live gateway serves at
// /aperture/openapi.json. It exists for tests only and is imported by
// nothing that ships in the provider binary.
//
// Behaviour mirrored from a live gateway:
//
//   - GET returns the stored document with credentials redacted, and an
//     unquoted ETag that changes with every stored change.
//   - PUT requires If-Match, answers 412 on a stale ETag and 422 on a
//     document that fails validation, and replaces the whole document.
//   - POST /config:validate reports every problem it finds in one pass.
//   - Keys the gateway does not know are rejected, and the value shapes
//     that changed over time (array add_headers, dollar-string quotas)
//     are enforced.
package fakegateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/tailscale/hujson"
)

// RedactedMarker is what GET returns in place of every non-empty apikey.
// A real gateway uses an opaque string of its own; tests must treat this
// one as opaque too.
const RedactedMarker = "redacted_fake_marker"

// Gateway is a running fake. Its zero value is not usable; call New.
type Gateway struct {
	srv *httptest.Server

	mu   sync.Mutex
	doc  string
	puts []string
}

// New starts a fake gateway serving initial, which must be a valid
// configuration document. It is stopped when the test ends.
func New(t testing.TB, initial string) *Gateway {
	t.Helper()
	if errs := validate(initial); len(errs) > 0 {
		t.Fatalf("fakegateway: initial document is invalid: %s", strings.Join(errs, "; "))
	}
	g := &Gateway{doc: initial}
	mux := http.NewServeMux()
	mux.HandleFunc("/aperture/config", g.handleConfig)
	mux.HandleFunc("/aperture/config:validate", g.handleValidate)
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

// Endpoint is the admin API base URL, including the /aperture prefix.
func (g *Gateway) Endpoint() string { return g.srv.URL + "/aperture" }

// Document returns the stored document exactly as last written, with
// credentials intact.
func (g *Gateway) Document() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.doc
}

// Writes returns the bodies of every accepted PUT, oldest first.
func (g *Gateway) Writes() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.puts...)
}

// Replace swaps the stored document the way an edit in the admin console
// would, rotating the ETag.
func (g *Gateway) Replace(t testing.TB, doc string) {
	t.Helper()
	if errs := validate(doc); len(errs) > 0 {
		t.Fatalf("fakegateway: replacement document is invalid: %s", strings.Join(errs, "; "))
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.doc = doc
}

func (g *Gateway) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.mu.Lock()
		doc := g.doc
		g.mu.Unlock()
		redacted, err := redact(doc)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		w.Header().Set("ETag", etag(doc))
		writeJSON(w, http.StatusOK, map[string]string{"config": redacted})
	case http.MethodPut:
		g.put(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		problem(w, http.StatusMethodNotAllowed, "Method Not Allowed", "")
	}
}

func (g *Gateway) put(w http.ResponseWriter, r *http.Request) {
	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		problem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", "required header parameter is missing: If-Match")
		return
	}
	body, err := readConfigBody(r)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if ifMatch != etag(g.doc) {
		problem(w, http.StatusPreconditionFailed, "Precondition Failed", "configuration has changed since the last read")
		return
	}
	if errs := validate(body); len(errs) > 0 {
		problem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", strings.Join(errs, "; "))
		return
	}
	g.doc = body
	g.puts = append(g.puts, body)
	redacted, err := redact(body)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("ETag", etag(body))
	writeJSON(w, http.StatusOK, map[string]string{"config": redacted})
}

func (g *Gateway) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, http.StatusMethodNotAllowed, "Method Not Allowed", "")
		return
	}
	body, err := readConfigBody(r)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
		return
	}
	errs := validate(body)
	if errs == nil {
		errs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": len(errs) == 0, "errors": errs})
}

func readConfigBody(r *http.Request) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		return "", err
	}
	var env struct {
		Config *string `json:"config"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return "", fmt.Errorf("invalid request body: %w", err)
	}
	if env.Config == nil {
		return "", fmt.Errorf("invalid request body: config is required")
	}
	return *env.Config, nil
}

// etag is unquoted, as a live gateway's is.
func etag(doc string) string {
	sum := sha256.Sum256([]byte(doc))
	return hex.EncodeToString(sum[:8])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"title": title, "status": status, "detail": detail})
}

func parse(doc string) (map[string]any, error) {
	pure, err := hujson.Standardize([]byte(doc))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(pure, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func redact(doc string) (string, error) {
	tree, err := parse(doc)
	if err != nil {
		return "", err
	}
	for _, section := range []string{"providers", "hooks"} {
		entries, _ := tree[section].(map[string]any)
		for _, e := range entries {
			entry, _ := e.(map[string]any)
			if key, _ := entry["apikey"].(string); key != "" {
				entry["apikey"] = RedactedMarker
			}
		}
	}
	out, err := json.MarshalIndent(tree, "", "  ")
	return string(out), err
}

var (
	topKeys = keys("providers", "grants", "quotas", "hooks", "exporters", "database",
		"connectors", "chat_models", "auto_cost_basis", "flags", "mcp", "temp_grants")
	providerKeys = keys("baseurl", "models", "apikey", "authorization", "auth_mode", "name",
		"description", "compatibility", "cost_basis", "preference", "disabled", "add_headers",
		"model_cost_map")
	hookKeys  = keys("url", "apikey", "authorization", "timeout", "disabled", "fail_policy", "preference")
	quotaKeys = keys("capacity", "rate", "on_exceed")
	grantKeys = keys("src", "dst", "app")
	capKeys   = keys("role", "models", "connectors", "quotas", "send_hooks", "add_headers",
		"filterable_headers", "enable_portal", "enable_chat_ui", "read_metrics", "set_cors",
		"cors_paths", "Access-Control-Allow-Origin", "Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers", "mcp_tools", "mcp_resources", "mcp_templates")
)

const capabilityKey = "tailscale.com/cap/aperture"

func keys(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// validate returns every problem with doc, in a stable order.
func validate(doc string) []string {
	tree, err := parse(doc)
	if err != nil {
		return []string{fmt.Sprintf("parse config: %v", err)}
	}
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	unknown := func(path string, obj map[string]any, known map[string]bool) {
		for _, k := range sortedKeys(obj) {
			if !known[k] {
				add("unknown config key %q", path+k)
			}
		}
	}
	stringArray := func(path string, v any) {
		list, ok := v.([]any)
		if !ok {
			add("%s: must be an array of strings", strings.TrimSuffix(path, "."))
			return
		}
		for _, e := range list {
			if _, ok := e.(string); !ok {
				add("%s: must be an array of strings", strings.TrimSuffix(path, "."))
				return
			}
		}
	}

	unknown("", tree, topKeys)

	providers, _ := tree["providers"].(map[string]any)
	for _, name := range sortedKeys(providers) {
		p, _ := providers[name].(map[string]any)
		unknown("providers."+name+".", p, providerKeys)
		if s, _ := p["baseurl"].(string); s == "" {
			add("provider %s: no baseurl configured", name)
		}
		if v, ok := p["add_headers"]; ok {
			stringArray("providers."+name+".add_headers", v)
		}
		switch mode, _ := p["auth_mode"].(string); mode {
		case "", "passthrough", "none":
		case "override":
			if key, _ := p["apikey"].(string); key == "" {
				add("provider %s: auth_mode \"override\" requires a non-empty apikey", name)
			}
		default:
			add("provider %s: invalid auth_mode %q", name, mode)
		}
	}

	hooks, _ := tree["hooks"].(map[string]any)
	for _, name := range sortedKeys(hooks) {
		h, _ := hooks[name].(map[string]any)
		unknown("hooks."+name+".", h, hookKeys)
		if s, _ := h["url"].(string); s == "" {
			add("hook %s has no url configured", name)
		}
		switch policy, _ := h["fail_policy"].(string); policy {
		case "", "fail_open", "fail_closed":
		default:
			add("hook %s has invalid fail_policy: %q (must be \"fail_open\" or \"fail_closed\")", name, policy)
		}
	}

	quotas, _ := tree["quotas"].(map[string]any)
	for _, name := range sortedKeys(quotas) {
		q, _ := quotas[name].(map[string]any)
		unknown("quotas."+name+".", q, quotaKeys)
		for _, field := range []string{"capacity", "rate"} {
			s, ok := q[field].(string)
			if !ok {
				add("quotas: json: cannot unmarshal %T into Go struct field .%s.%s of type string", q[field], name, field)
			} else if !strings.HasPrefix(s, "$") && !strings.HasPrefix(s, "-$") {
				add("quota %q: invalid %s: dollar amount must start with \"$\" or \"-$\", got %q", name, field, s)
			}
		}
		if onExceed, _ := q["on_exceed"].(string); onExceed != "reject" {
			add("quota %q: on_exceed must be \"reject\", got %q", name, onExceed)
		}
	}

	grants, _ := tree["grants"].([]any)
	admin := false
	for i, g := range grants {
		grant, _ := g.(map[string]any)
		unknown(fmt.Sprintf("grants[%d].", i), grant, grantKeys)
		app, _ := grant["app"].(map[string]any)
		caps, _ := app[capabilityKey].([]any)
		for j, c := range caps {
			cap, _ := c.(map[string]any)
			where := fmt.Sprintf("grants[%d].app.%s[%d].", i, capabilityKey, j)
			unknown(where, cap, capKeys)
			if role, _ := cap["role"].(string); role == "admin" {
				admin = true
			}
			if v, ok := cap["models"]; ok {
				if _, isString := v.(string); !isString {
					add("grants[%d]: parsing %s grants: json: cannot unmarshal %T into Go struct field .%d.models of type string", i, capabilityKey, v, j)
				}
			}
			for _, field := range []string{"connectors", "add_headers", "filterable_headers", "cors_paths"} {
				if v, ok := cap[field]; ok {
					stringArray(where+field, v)
				}
			}
			for _, field := range []string{"enable_portal", "enable_chat_ui", "read_metrics", "set_cors"} {
				if v, ok := cap[field]; ok {
					if _, isBool := v.(bool); !isBool {
						add("grants[%d]: parsing %s grants: json: cannot unmarshal %T into Go struct field .%d.%s of type bool", i, capabilityKey, v, j, field)
					}
				}
			}
		}
	}
	if len(providers) > 0 && len(grants) == 0 {
		add("providers are configured but no grants or temp_grants defined; all access will be denied")
	}
	if (len(providers) > 0 || len(grants) > 0) && !admin {
		add("no grant in grants or temp_grants assigns role:admin; nobody will be able to manage this instance unless admin is granted via tsnet ACL policy")
	}
	return errs
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
