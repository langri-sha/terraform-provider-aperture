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
	BaseURL       string         `json:"baseurl"`
	Models        []string       `json:"models"`
	APIKey        string         `json:"apikey,omitempty"`
	Authorization string         `json:"authorization,omitempty"`
	AuthMode      string         `json:"auth_mode,omitempty"`
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Compatibility *Compatibility `json:"compatibility,omitempty"`
	CostBasis     string         `json:"cost_basis,omitempty"`
	ModelCostMap  []ModelCost    `json:"model_cost_map,omitempty"`
	Preference    *int64         `json:"preference,omitempty"`
	Disabled      *bool          `json:"disabled,omitempty"`
	AddHeaders    []string       `json:"add_headers,omitempty"`
}

// Compatibility is a provider's API compatibility flags. An unset flag
// takes the gateway's default, which for openai_chat depends on the other
// flags, so false and unset are different.
type Compatibility struct {
	OpenAIChat            *bool `json:"openai_chat,omitempty"`
	OpenAIResponses       *bool `json:"openai_responses,omitempty"`
	AnthropicMessages     *bool `json:"anthropic_messages,omitempty"`
	GeminiGenerateContent *bool `json:"gemini_generate_content,omitempty"`
	BedrockModelInvoke    *bool `json:"bedrock_model_invoke,omitempty"`
	GoogleGenerateContent *bool `json:"google_generate_content,omitempty"`
	GoogleRawPredict      *bool `json:"google_raw_predict,omitempty"`
	BedrockConverse       *bool `json:"bedrock_converse,omitempty"`
}

// ModelCost maps model names that match a glob onto a known model for
// pricing. The gateway uses the first entry that matches.
type ModelCost struct {
	Match      string   `json:"match"`
	As         string   `json:"as"`
	Adjustment *float64 `json:"adjustment,omitempty"`
}

// Grant is one entry in the top-level `grants` array. It uses
// Tailscale's grant shape verbatim — see
// https://tailscale.com/docs/aperture/how-to/grant-model-access.
type Grant struct {
	Src []string `json:"src"`
	Dst []string `json:"dst,omitempty"`
	App GrantApp `json:"app"`
}

// GrantApp groups capabilities under the upstream-required key
// "tailscale.com/cap/aperture".
type GrantApp struct {
	Aperture []GrantCapability `json:"tailscale.com/cap/aperture"`
}

// GrantCapability is one capability entry. Each entry sets whichever of
// these a grant needs; the marshaller omits the rest. The CORS response
// header fields (Access-Control-Allow-*) are not modelled, and are kept
// as the gateway holds them.
type GrantCapability struct {
	Role              string   `json:"role,omitempty"`
	Models            string   `json:"models,omitempty"`
	Connectors        []string `json:"connectors,omitempty"`
	AddHeaders        []string `json:"add_headers,omitempty"`
	FilterableHeaders []string `json:"filterable_headers,omitempty"`
	EnablePortal      *bool    `json:"enable_portal,omitempty"`
	ReadMetrics       *bool    `json:"read_metrics,omitempty"`
	SetCORS           *bool    `json:"set_cors,omitempty"`
	CORSPaths         []string `json:"cors_paths,omitempty"`
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
