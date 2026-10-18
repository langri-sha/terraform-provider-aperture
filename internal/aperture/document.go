package aperture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// Document is a configuration document held as a generic JSON tree.
//
// The gateway replaces its whole configuration on every write, and its
// schema keeps growing. Writing a typed Config alone would delete every
// key the provider does not model. A Document keeps the tree the gateway
// returned, and Apply overlays the typed Config onto it: keys the Config
// models are set from it (or removed when it omits them), every other key
// is left exactly as the gateway sent it, at any depth.
//
// A field tagged `aperture:"preserve"` is also left alone when the Config
// omits it, for sections the Config carries only when asked to.
//
// An empty string stands for "not set" in the typed Config, so a key the
// gateway holds as "" is left alone when the Config omits it.
//
// Arrays have no identity in the document, so unmodelled keys inside an
// array element follow the element's position.
type Document struct {
	tree map[string]any
}

// ParseDocument parses a HuJSON configuration document. Comments and
// trailing commas are accepted and not retained.
func ParseDocument(s string) (*Document, error) {
	pure, err := hujson.Standardize([]byte(s))
	if err != nil {
		return nil, fmt.Errorf("aperture: standardize hujson: %w", err)
	}
	tree, err := decodeTree(pure)
	if err != nil {
		return nil, fmt.Errorf("aperture: unmarshal config: %w", err)
	}
	return &Document{tree: tree}, nil
}

// NewDocument returns an empty document.
func NewDocument() *Document { return &Document{tree: map[string]any{}} }

// Config returns the typed projection of the document.
func (d *Document) Config() (*Config, error) {
	raw, err := json.Marshal(d.tree)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("aperture: unmarshal config: %w", err)
	}
	return &cfg, nil
}

// Apply overlays c onto the document.
func (d *Document) Apply(c *Config) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	desired, err := decodeTree(raw)
	if err != nil {
		return err
	}
	d.tree = overlayStruct(d.tree, desired, reflect.TypeOf(*c))
	return nil
}

// String renders the document as indented JSON, which is a subset of
// HuJSON.
func (d *Document) String() (string, error) {
	out, err := json.MarshalIndent(d.tree, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeTree(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree map[string]any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	if tree == nil {
		tree = map[string]any{}
	}
	return tree, nil
}

func overlay(base, desired any, t reflect.Type) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		d, ok := desired.(map[string]any)
		if !ok {
			return desired
		}
		b, _ := base.(map[string]any)
		return overlayStruct(b, d, t)
	case reflect.Map:
		d, ok := desired.(map[string]any)
		if !ok {
			return desired
		}
		b, _ := base.(map[string]any)
		out := make(map[string]any, len(d))
		for k, v := range d {
			out[k] = overlay(b[k], v, t.Elem())
		}
		return out
	case reflect.Slice:
		d, ok := desired.([]any)
		if !ok || !isStructSlice(t) {
			return desired
		}
		b, _ := base.([]any)
		out := make([]any, len(d))
		for i, v := range d {
			var prior any
			if i < len(b) {
				prior = b[i]
			}
			out[i] = overlay(prior, v, t.Elem())
		}
		return out
	default:
		return desired
	}
}

func isStructSlice(t reflect.Type) bool {
	e := t.Elem()
	for e.Kind() == reflect.Pointer {
		e = e.Elem()
	}
	return e.Kind() == reflect.Struct
}

func overlayStruct(base, desired map[string]any, t reflect.Type) map[string]any {
	out := make(map[string]any, len(base)+len(desired))
	for k, v := range base {
		out[k] = v
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := jsonName(f)
		if name == "" {
			continue
		}
		v, ok := desired[name]
		if !ok {
			if f.Tag.Get("aperture") == "preserve" {
				continue
			}
			if prior, isString := base[name].(string); !isString || prior != "" {
				delete(out, name)
			}
			continue
		}
		out[name] = overlay(base[name], v, f.Type)
	}
	return out
}

func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" || !f.IsExported() {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return f.Name
	}
	return name
}

// Credential locates an apikey the document holds under a provider or hook.
type Credential struct {
	Section string // "providers" or "hooks"
	Name    string
}

func (c Credential) String() string { return c.Section + "." + c.Name + ".apikey" }

// Credentials lists the non-empty apikeys in the document, ordered by
// section then name. A gateway redacts them on read, so any it returns
// stands for a value the caller cannot see.
func (d *Document) Credentials() []Credential {
	var out []Credential
	for _, section := range []string{"hooks", "providers"} {
		entries, _ := d.tree[section].(map[string]any)
		for _, name := range sortedKeys(entries) {
			entry, _ := entries[name].(map[string]any)
			if key, _ := entry["apikey"].(string); key != "" {
				out = append(out, Credential{Section: section, Name: name})
			}
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
