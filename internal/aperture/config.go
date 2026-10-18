package aperture

// Config is the typed wire form of an Aperture HuJSON configuration
// document. Field names mirror upstream JSON keys verbatim
// (`baseurl`, `apikey`, `add_headers`, `on_exceed`, ...) so the
// Terraform schema can map one-to-one.
//
// Only the keys we actively model are typed; everything else is
// preserved through Extras for forward compatibility. Extras lets a
// future Aperture version add fields without breaking existing
// state — Terraform won't claim ownership of fields it doesn't know
// about.
type Config struct {
	Providers     map[string]Provider `json:"providers,omitempty"`
	Grants        []Grant             `json:"grants,omitempty"`
	Quotas        map[string]Quota    `json:"quotas,omitempty"`
	Hooks         map[string]Hook     `json:"hooks,omitempty"`
	AutoCostBasis *bool               `json:"auto_cost_basis,omitempty"`
}

// Provider is one entry in the top-level `providers` map.
type Provider struct {
	BaseURL       string            `json:"baseurl"`
	Models        []string          `json:"models"`
	APIKey        string            `json:"apikey,omitempty"`
	Authorization string            `json:"authorization,omitempty"`
	Name          string            `json:"name,omitempty"`
	Description   string            `json:"description,omitempty"`
	CostBasis     string            `json:"cost_basis,omitempty"`
	Preference    *int64            `json:"preference,omitempty"`
	Disabled      *bool             `json:"disabled,omitempty"`
	AddHeaders    map[string]string `json:"add_headers,omitempty"`
}

// Grant is one entry in the top-level `grants` array. It uses
// Tailscale's grant shape verbatim — see
// https://tailscale.com/docs/aperture/how-to/grant-model-access.
type Grant struct {
	Src []string `json:"src"`
	App GrantApp `json:"app"`
}

// GrantApp groups capabilities under the upstream-required key
// "tailscale.com/cap/aperture".
type GrantApp struct {
	Aperture []GrantCapability `json:"tailscale.com/cap/aperture"`
}

// GrantCapability is one capability entry. Upstream allows either
// {role: ...} or {models: ...} per entry; we model both as
// optional pointers and let the marshaller omit absent fields.
type GrantCapability struct {
	Role   string `json:"role,omitempty"`
	Models string `json:"models,omitempty"`
}

// Quota is one entry in the top-level `quotas` map.
//
// Capacity is a dollar amount such as "$10.00"; Rate adds a refill period
// such as "$5.00/day". The gateway rejects bare numbers.
type Quota struct {
	Capacity string `json:"capacity"`
	Rate     string `json:"rate"`
	OnExceed string `json:"on_exceed,omitempty"`
}

// Hook is one entry in the top-level `hooks` map.
type Hook struct {
	URL           string `json:"url"`
	APIKey        string `json:"apikey,omitempty"`
	Authorization string `json:"authorization,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	Disabled      *bool  `json:"disabled,omitempty"`
	FailPolicy    string `json:"fail_policy,omitempty"`
	Preference    *int64 `json:"preference,omitempty"`
}
