package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/langri-sha/aperture/internal/aperture"
)

// aperture_config is the singleton resource representing the
// entire Aperture configuration document. There is exactly one of
// these per gateway; declaring multiple resource instances pointed
// at the same provider endpoint will stomp each other.

const singletonID = "default"

func newConfigResource() resource.Resource { return &configResource{} }

type configResource struct {
	client *aperture.Client
}

type configResourceModel struct {
	ID            types.String             `tfsdk:"id"`
	ETag          types.String             `tfsdk:"etag"`
	Providers     map[string]providerEntry `tfsdk:"providers"`
	Grants        []grantEntry             `tfsdk:"grants"`
	Quotas        map[string]quotaEntry    `tfsdk:"quotas"`
	Hooks         map[string]hookEntry     `tfsdk:"hooks"`
	AutoCostBasis types.Bool               `tfsdk:"auto_cost_basis"`
	Exporters     jsontypes.Normalized     `tfsdk:"exporters"`
	Database      jsontypes.Normalized     `tfsdk:"database"`
	Connectors    jsontypes.Normalized     `tfsdk:"connectors"`
	ChatModels    jsontypes.Normalized     `tfsdk:"chat_models"`
	Flags         jsontypes.Normalized     `tfsdk:"flags"`
	MCP           jsontypes.Normalized     `tfsdk:"mcp"`
	TempGrants    jsontypes.Normalized     `tfsdk:"temp_grants"`
}

type providerEntry struct {
	BaseURL       types.String        `tfsdk:"baseurl"`
	Models        []types.String      `tfsdk:"models"`
	APIKey        types.String        `tfsdk:"apikey"`
	Authorization types.String        `tfsdk:"authorization"`
	AuthMode      types.String        `tfsdk:"auth_mode"`
	Name          types.String        `tfsdk:"name"`
	Description   types.String        `tfsdk:"description"`
	Compatibility *compatibilityEntry `tfsdk:"compatibility"`
	CostBasis     types.String        `tfsdk:"cost_basis"`
	ModelCostMap  []modelCostEntry    `tfsdk:"model_cost_map"`
	Preference    types.Int64         `tfsdk:"preference"`
	Disabled      types.Bool          `tfsdk:"disabled"`
	AddHeaders    []types.String      `tfsdk:"add_headers"`
}

type compatibilityEntry struct {
	OpenAIChat            types.Bool `tfsdk:"openai_chat"`
	OpenAIResponses       types.Bool `tfsdk:"openai_responses"`
	AnthropicMessages     types.Bool `tfsdk:"anthropic_messages"`
	GeminiGenerateContent types.Bool `tfsdk:"gemini_generate_content"`
	BedrockModelInvoke    types.Bool `tfsdk:"bedrock_model_invoke"`
	GoogleGenerateContent types.Bool `tfsdk:"google_generate_content"`
	GoogleRawPredict      types.Bool `tfsdk:"google_raw_predict"`
	BedrockConverse       types.Bool `tfsdk:"bedrock_converse"`
}

type modelCostEntry struct {
	Match      types.String  `tfsdk:"match"`
	As         types.String  `tfsdk:"as"`
	Adjustment types.Float64 `tfsdk:"adjustment"`
}

type grantEntry struct {
	Src          []types.String    `tfsdk:"src"`
	Dst          []types.String    `tfsdk:"dst"`
	Capabilities []capabilityEntry `tfsdk:"capabilities"`
}

type capabilityEntry struct {
	Role              types.String      `tfsdk:"role"`
	Models            types.String      `tfsdk:"models"`
	Connectors        []types.String    `tfsdk:"connectors"`
	Quotas            []grantQuotaEntry `tfsdk:"quotas"`
	SendHooks         []sendHookEntry   `tfsdk:"send_hooks"`
	AddHeaders        []types.String    `tfsdk:"add_headers"`
	FilterableHeaders []types.String    `tfsdk:"filterable_headers"`
	EnablePortal      types.Bool        `tfsdk:"enable_portal"`
	ReadMetrics       types.Bool        `tfsdk:"read_metrics"`
	SetCORS           types.Bool        `tfsdk:"set_cors"`
	CORSPaths         []types.String    `tfsdk:"cors_paths"`
}

type grantQuotaEntry struct {
	Bucket     types.String   `tfsdk:"bucket"`
	Overdrafts []types.String `tfsdk:"overdrafts"`
}

type sendHookEntry struct {
	Name   types.String   `tfsdk:"name"`
	Events []types.String `tfsdk:"events"`
	Send   []types.String `tfsdk:"send"`
}

type quotaEntry struct {
	Capacity types.String `tfsdk:"capacity"`
	Rate     types.String `tfsdk:"rate"`
	OnExceed types.String `tfsdk:"on_exceed"`
}

type hookEntry struct {
	URL           types.String `tfsdk:"url"`
	APIKey        types.String `tfsdk:"apikey"`
	Authorization types.String `tfsdk:"authorization"`
	Timeout       types.String `tfsdk:"timeout"`
	Disabled      types.Bool   `tfsdk:"disabled"`
	FailPolicy    types.String `tfsdk:"fail_policy"`
	Preference    types.Int64  `tfsdk:"preference"`
}

func (r *configResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *configResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The singleton Aperture configuration document. Field names mirror the upstream HuJSON keys verbatim — see https://tailscale.com/docs/aperture/configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always \"default\" — Aperture has one configuration per gateway.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"etag": schema.StringAttribute{
				Computed:    true,
				Description: "Opaque ETag from the gateway, used as If-Match on subsequent updates.",
			},
			"providers": schema.MapNestedAttribute{
				Optional:    true,
				Description: "LLM provider configurations, keyed by provider name (openai, anthropic, google, ...).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"baseurl":       schema.StringAttribute{Required: true},
						"models":        schema.ListAttribute{ElementType: types.StringType, Required: true},
						"apikey":        schema.StringAttribute{Optional: true, Sensitive: true},
						"authorization": schema.StringAttribute{Optional: true, Description: "How the credential is sent: bearer, x-api-key, x-goog-api-key, hec or cf-aig-authorization."},
						"auth_mode": schema.StringAttribute{
							Optional:    true,
							Description: "Source of the upstream credential: \"\" or \"override\" injects apikey, \"passthrough\" forwards the client's own credential and falls back to apikey, \"none\" sends no credential.",
						},
						"name":        schema.StringAttribute{Optional: true},
						"description": schema.StringAttribute{Optional: true},
						"compatibility": schema.SingleNestedAttribute{
							Optional:    true,
							Description: "API formats the provider supports. A flag left unset takes the gateway's default, so set openai_chat explicitly to keep or disable it next to another flag.",
							Attributes: map[string]schema.Attribute{
								"openai_chat":             schema.BoolAttribute{Optional: true},
								"openai_responses":        schema.BoolAttribute{Optional: true},
								"anthropic_messages":      schema.BoolAttribute{Optional: true},
								"gemini_generate_content": schema.BoolAttribute{Optional: true},
								"bedrock_model_invoke":    schema.BoolAttribute{Optional: true},
								"google_generate_content": schema.BoolAttribute{Optional: true},
								"google_raw_predict":      schema.BoolAttribute{Optional: true},
								"bedrock_converse":        schema.BoolAttribute{Optional: true},
							},
						},
						"cost_basis": schema.StringAttribute{Optional: true},
						"model_cost_map": schema.ListNestedAttribute{
							Optional:    true,
							Description: "Prices unknown model names as known ones. The first matching entry wins.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"match":      schema.StringAttribute{Required: true, Description: "Glob over the model name, in path.Match syntax."},
									"as":         schema.StringAttribute{Required: true, Description: "Model name to price it as."},
									"adjustment": schema.Float64Attribute{Optional: true, Description: "Price multiplier; 1.5 marks up 50%. Defaults to 1."},
								},
							},
						},
						"preference":  schema.Int64Attribute{Optional: true},
						"disabled":    schema.BoolAttribute{Optional: true},
						"add_headers": schema.ListAttribute{ElementType: types.StringType, Optional: true, Sensitive: true, Description: "Headers added to every upstream request, each as \"Header-Name: value\"."},
					},
				},
			},
			"grants": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"src": schema.ListAttribute{ElementType: types.StringType, Required: true},
						"dst": schema.ListAttribute{ElementType: types.StringType, Optional: true, Description: "Destinations, needed only when the grant is meant to be copied into a tailnet policy file."},
						"capabilities": schema.ListNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"role":   schema.StringAttribute{Optional: true, Description: "user | admin"},
									"models": schema.StringAttribute{Optional: true, Description: "Glob in provider/model form, e.g. anthropic/**"},
									"connectors": schema.ListAttribute{
										ElementType: types.StringType, Optional: true,
										Description: "Connector access, as globs in connectorID/category/resource form or label:<name>, e.g. aperture/**.",
									},
									"quotas": schema.ListNestedAttribute{
										Optional:    true,
										Description: "Quota buckets charged when the grant matches. A bucket takes effect only when a grant attaches it.",
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"bucket":     schema.StringAttribute{Required: true, Description: "Name of a bucket in the top-level quotas, templates included, e.g. daily:<user>."},
												"overdrafts": schema.ListAttribute{ElementType: types.StringType, Optional: true, Description: "Buckets to borrow from, in order, when this one is exhausted."},
											},
										},
									},
									"send_hooks": schema.ListNestedAttribute{
										Optional:    true,
										Description: "Hooks the grant triggers. A hook in the top-level hooks does nothing until a grant sends to it.",
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"name":   schema.StringAttribute{Required: true, Description: "Key of a hook in the top-level hooks."},
												"events": schema.ListAttribute{ElementType: types.StringType, Required: true, Description: "pre_request, entire_request or tool_call_entire_request."},
												"send":   schema.ListAttribute{ElementType: types.StringType, Required: true, Description: "Data to include: tools, user_message, request_body, response_body, raw_responses or quotas."},
											},
										},
									},
									"add_headers": schema.ListAttribute{
										ElementType: types.StringType, Optional: true, Sensitive: true,
										Description: "Headers prepended to upstream requests when the grant matches, each as \"Header-Name: value\".",
									},
									"filterable_headers": schema.ListAttribute{
										ElementType: types.StringType, Optional: true,
										Description: "Indexed header names matched users can filter dashboards by; [\"*\"] grants every configured one.",
									},
									"enable_portal": schema.BoolAttribute{Optional: true, Description: "Access to the web portal, available by default. Set false to disable it for this grant."},
									"read_metrics":  schema.BoolAttribute{Optional: true, Description: "Access to the /metrics Prometheus endpoint."},
									"set_cors":      schema.BoolAttribute{Optional: true, Description: "Inject CORS response headers; needs cors_paths."},
									"cors_paths": schema.ListAttribute{
										ElementType: types.StringType, Optional: true,
										Description: "URL path globs that activate CORS headers, e.g. /v1/**.",
									},
								},
							},
						},
					},
				},
			},
			"quotas": schema.MapNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"capacity":  schema.StringAttribute{Required: true, Description: "Maximum balance as a dollar amount, e.g. \"$10.00\"."},
						"rate":      schema.StringAttribute{Required: true, Description: "Refill rate as a dollar amount per period (min, hour, day, week or month), e.g. \"$5.00/day\"."},
						"on_exceed": schema.StringAttribute{Optional: true, Description: "Action when a request would overdraw the bucket. The only supported value is \"reject\"."},
					},
				},
			},
			"hooks": schema.MapNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"url":           schema.StringAttribute{Required: true},
						"apikey":        schema.StringAttribute{Optional: true, Sensitive: true},
						"authorization": schema.StringAttribute{Optional: true, Description: "How the credential is sent: bearer, x-api-key, x-goog-api-key, hec or cf-aig-authorization."},
						"timeout":       schema.StringAttribute{Optional: true},
						"disabled":      schema.BoolAttribute{Optional: true},
						"fail_policy":   schema.StringAttribute{Optional: true},
						"preference":    schema.Int64Attribute{Optional: true},
					},
				},
			},
			"auto_cost_basis": schema.BoolAttribute{Optional: true},
			"exporters":       opaqueSectionAttribute("LLM session log export configuration, as JSON (use jsonencode)."),
			"database":        opaqueSectionAttribute("Data storage and capture retention settings, as JSON (use jsonencode)."),
			"connectors":      opaqueSectionAttribute("Outbound integrations (MCP servers and HTTP APIs), as JSON (use jsonencode)."),
			"chat_models":     opaqueSectionAttribute("Models offered in the built-in chat, as JSON (use jsonencode)."),
			"flags":           opaqueSectionAttribute("Experimental feature flags, as JSON (use jsonencode)."),
			"mcp":             opaqueSectionAttribute("Deprecated MCP server proxying configuration, as JSON. Use connectors instead."),
			"temp_grants":     opaqueSectionAttribute("Deprecated access control syntax, as JSON. Use grants instead."),
		},
	}
}

func (r *configResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*aperture.Client)
	if !ok {
		resp.Diagnostics.AddError("provider data type", "expected *aperture.Client")
		return
	}
	r.client = c
}

func (r *configResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan configResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applyPlan(ctx, &plan, "", &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *configResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior configResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, etag, err := r.client.GetConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("read aperture config", err.Error())
		return
	}
	doc, err := aperture.ParseConfigDocument(body)
	if err != nil {
		resp.Diagnostics.AddError("parse aperture config", err.Error())
		return
	}

	state := fromAPIConfig(doc)
	preserveSensitiveFromPrior(&state, &prior)
	leaveUnmanagedSectionsNull(&state, &prior)
	state.ID = types.StringValue(singletonID)
	state.ETag = types.StringValue(etag)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *configResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior configResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applyPlan(ctx, &plan, prior.ETag.ValueString(), &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *configResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Aperture has no notion of "no config" — the gateway always
	// serves whatever document is currently stored. Removing the
	// resource from terraform shouldn't wipe a working gateway. Drop
	// the state row and warn the operator.
	resp.Diagnostics.AddWarning(
		"aperture_config destroy is a state-only operation",
		"Aperture has no admin endpoint to delete a configuration; the current document remains live on the gateway. To wipe it, PUT an empty/minimal config manually via aperture-cli or curl.",
	)
}

func (r *configResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Singleton — accept anything for ergonomics but force the
	// canonical id so the post-import Read produces a stable state.
	id := req.ID
	if id == "" {
		id = singletonID
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// applyPlan is the shared body of Create and Update: read the gateway's
// current document, overlay the plan onto it so keys the provider does
// not model survive, validate, PUT, and stamp the result back into the
// plan model. Aperture always has a document, so Create is an overlay
// onto whatever is there. expectedETag is the ETag the plan was made
// against; empty means any. Returns false on any error (after recording
// diagnostics).
func (r *configResource) applyPlan(ctx context.Context, plan *configResourceModel, expectedETag string, diags *diag.Diagnostics) bool {
	current, etag, err := r.client.GetConfig(ctx)
	if err != nil {
		diags.AddError("read existing aperture config", err.Error())
		return false
	}
	if expectedETag != "" && etag != expectedETag {
		diags.AddError("aperture configuration changed externally", changedExternally)
		return false
	}

	doc, err := aperture.ParseDocument(current)
	if err != nil {
		diags.AddError("parse aperture config", err.Error())
		return false
	}
	cfg, err := toAPIConfig(plan)
	if err != nil {
		diags.AddError("render aperture config", err.Error())
		return false
	}
	if !checkCredentialsManaged(doc, cfg, diags) {
		return false
	}
	if err := doc.Apply(cfg); err != nil {
		diags.AddError("render aperture config", err.Error())
		return false
	}
	body, err := doc.String()
	if err != nil {
		diags.AddError("marshal aperture config", err.Error())
		return false
	}

	vr, err := r.client.ValidateConfig(ctx, body)
	if err != nil {
		diags.AddError("validate aperture config", err.Error())
		return false
	}
	if !vr.Valid {
		for _, e := range vr.Errors {
			diags.AddError("invalid aperture config", e)
		}
		return false
	}

	_, newEtag, err := r.client.SetConfig(ctx, body, etag)
	if err != nil {
		if errors.Is(err, aperture.ErrPreconditionFailed) {
			diags.AddError("aperture configuration changed externally", changedExternally)
			return false
		}
		diags.AddError("write aperture config", err.Error())
		return false
	}
	plan.ID = types.StringValue(singletonID)
	plan.ETag = types.StringValue(newEtag)
	return true
}

// checkCredentialsManaged fails when applying cfg would overwrite an
// apikey the gateway holds that cfg does not set. The gateway redacts
// apikeys on read, so the provider cannot keep one by writing back what it
// read, and omitting it would delete the key.
func checkCredentialsManaged(doc *aperture.Document, cfg *aperture.Config, diags *diag.Diagnostics) bool {
	ok := true
	for _, c := range doc.Credentials() {
		var planned string
		var present bool
		switch c.Section {
		case "providers":
			p, found := cfg.Providers[c.Name]
			planned, present = p.APIKey, found
		case "hooks":
			h, found := cfg.Hooks[c.Name]
			planned, present = h.APIKey, found
		}
		if present && planned == "" {
			ok = false
			diags.AddError(
				"aperture credential not managed",
				fmt.Sprintf("The gateway holds an apikey for %s that the provider cannot read back, so it cannot keep it while applying. Set apikey in the configuration to write it, or remove %s.%s to delete it.", c, c.Section, c.Name),
			)
		}
	}
	return ok
}

const changedExternally = "Re-run `terraform apply -refresh-only` (or `terraform plan`) to pick up the latest state, then apply again."

// preserveSensitiveFromPrior carries values the gateway never returns in
// the clear from prior state into a freshly read model. GET redacts
// every apikey, and the redaction marker is opaque, so apikeys come from
// prior state or stay null; they are never read from the gateway.
// add_headers values may carry credentials too, and are kept from prior
// state when there is one.
func preserveSensitiveFromPrior(server, prior *configResourceModel) {
	for name, sp := range server.Providers {
		if pp, ok := prior.Providers[name]; ok {
			sp.APIKey = pp.APIKey
			if len(pp.AddHeaders) > 0 {
				sp.AddHeaders = pp.AddHeaders
			}
			server.Providers[name] = sp
		}
	}
	for name, sh := range server.Hooks {
		if ph, ok := prior.Hooks[name]; ok {
			sh.APIKey = ph.APIKey
			server.Hooks[name] = sh
		}
	}
}

// toAPIConfig converts the Terraform-typed model into the
// aperture-typed wire form.
func toAPIConfig(m *configResourceModel) (*aperture.Config, error) {
	out := &aperture.Config{}

	if len(m.Providers) > 0 {
		out.Providers = make(map[string]aperture.Provider, len(m.Providers))
		for name, p := range m.Providers {
			ap := aperture.Provider{
				BaseURL: p.BaseURL.ValueString(),
				Models:  stringsFromTypedList(p.Models),
			}
			if !p.APIKey.IsNull() {
				ap.APIKey = p.APIKey.ValueString()
			}
			if !p.Authorization.IsNull() {
				ap.Authorization = p.Authorization.ValueString()
			}
			if !p.AuthMode.IsNull() {
				ap.AuthMode = p.AuthMode.ValueString()
			}
			if !p.Name.IsNull() {
				ap.Name = p.Name.ValueString()
			}
			ap.Compatibility = compatibilityToAPI(p.Compatibility)
			ap.ModelCostMap = modelCostMapToAPI(p.ModelCostMap)
			if !p.Description.IsNull() {
				ap.Description = p.Description.ValueString()
			}
			if !p.CostBasis.IsNull() {
				ap.CostBasis = p.CostBasis.ValueString()
			}
			if !p.Preference.IsNull() {
				v := p.Preference.ValueInt64()
				ap.Preference = &v
			}
			if !p.Disabled.IsNull() {
				v := p.Disabled.ValueBool()
				ap.Disabled = &v
			}
			if len(p.AddHeaders) > 0 {
				ap.AddHeaders = stringsFromTypedList(p.AddHeaders)
			}
			out.Providers[name] = ap
		}
	}

	if len(m.Grants) > 0 {
		out.Grants = make([]aperture.Grant, 0, len(m.Grants))
		for _, g := range m.Grants {
			caps := make([]aperture.GrantCapability, 0, len(g.Capabilities))
			for _, c := range g.Capabilities {
				caps = append(caps, aperture.GrantCapability{
					Role:              c.Role.ValueString(),
					Models:            c.Models.ValueString(),
					Connectors:        optionalStrings(c.Connectors),
					Quotas:            grantQuotasToAPI(c.Quotas),
					SendHooks:         sendHooksToAPI(c.SendHooks),
					AddHeaders:        optionalStrings(c.AddHeaders),
					FilterableHeaders: optionalStrings(c.FilterableHeaders),
					EnablePortal:      boolPointer(c.EnablePortal),
					ReadMetrics:       boolPointer(c.ReadMetrics),
					SetCORS:           boolPointer(c.SetCORS),
					CORSPaths:         optionalStrings(c.CORSPaths),
				})
			}
			out.Grants = append(out.Grants, aperture.Grant{
				Src: stringsFromTypedList(g.Src),
				Dst: optionalStrings(g.Dst),
				App: aperture.GrantApp{Aperture: caps},
			})
		}
	}

	if len(m.Quotas) > 0 {
		out.Quotas = make(map[string]aperture.Quota, len(m.Quotas))
		for name, q := range m.Quotas {
			out.Quotas[name] = aperture.Quota{
				Capacity: q.Capacity.ValueString(),
				Rate:     q.Rate.ValueString(),
				OnExceed: q.OnExceed.ValueString(),
			}
		}
	}

	if len(m.Hooks) > 0 {
		out.Hooks = make(map[string]aperture.Hook, len(m.Hooks))
		for name, h := range m.Hooks {
			ah := aperture.Hook{URL: h.URL.ValueString()}
			if !h.APIKey.IsNull() {
				ah.APIKey = h.APIKey.ValueString()
			}
			if !h.Authorization.IsNull() {
				ah.Authorization = h.Authorization.ValueString()
			}
			if !h.Timeout.IsNull() {
				ah.Timeout = h.Timeout.ValueString()
			}
			if !h.Disabled.IsNull() {
				v := h.Disabled.ValueBool()
				ah.Disabled = &v
			}
			if !h.FailPolicy.IsNull() {
				ah.FailPolicy = h.FailPolicy.ValueString()
			}
			if !h.Preference.IsNull() {
				v := h.Preference.ValueInt64()
				ah.Preference = &v
			}
			out.Hooks[name] = ah
		}
	}

	if !m.AutoCostBasis.IsNull() {
		v := m.AutoCostBasis.ValueBool()
		out.AutoCostBasis = &v
	}

	for _, s := range opaqueSections {
		a := s.attr(m)
		if a.IsNull() || a.IsUnknown() {
			continue
		}
		if !json.Valid([]byte(a.ValueString())) {
			return nil, fmt.Errorf("a JSON section is not valid JSON: %s", a.ValueString())
		}
		*s.raw(out) = json.RawMessage(a.ValueString())
	}

	return out, nil
}

// fromAPIConfig is the inverse of toAPIConfig, except that it leaves
// apikey null: see preserveSensitiveFromPrior.
func fromAPIConfig(c *aperture.Config) configResourceModel {
	m := configResourceModel{}

	if len(c.Providers) > 0 {
		m.Providers = make(map[string]providerEntry, len(c.Providers))
		for name, p := range c.Providers {
			m.Providers[name] = providerEntry{
				BaseURL:       types.StringValue(p.BaseURL),
				Models:        typedListFromStrings(p.Models),
				Authorization: nullableString(p.Authorization),
				AuthMode:      nullableString(p.AuthMode),
				Name:          nullableString(p.Name),
				Description:   nullableString(p.Description),
				Compatibility: compatibilityFromAPI(p.Compatibility),
				CostBasis:     nullableString(p.CostBasis),
				ModelCostMap:  modelCostMapFromAPI(p.ModelCostMap),
				Preference:    nullableInt64(p.Preference),
				Disabled:      nullableBool(p.Disabled),
				AddHeaders:    typedListFromStrings(p.AddHeaders),
			}
		}
	}
	if len(c.Grants) > 0 {
		m.Grants = make([]grantEntry, 0, len(c.Grants))
		for _, g := range c.Grants {
			caps := make([]capabilityEntry, 0, len(g.App.Aperture))
			for _, cap := range g.App.Aperture {
				caps = append(caps, capabilityEntry{
					Role:              nullableString(cap.Role),
					Models:            nullableString(cap.Models),
					Connectors:        optionalTypedList(cap.Connectors),
					Quotas:            grantQuotasFromAPI(cap.Quotas),
					SendHooks:         sendHooksFromAPI(cap.SendHooks),
					AddHeaders:        optionalTypedList(cap.AddHeaders),
					FilterableHeaders: optionalTypedList(cap.FilterableHeaders),
					EnablePortal:      nullableBool(cap.EnablePortal),
					ReadMetrics:       nullableBool(cap.ReadMetrics),
					SetCORS:           nullableBool(cap.SetCORS),
					CORSPaths:         optionalTypedList(cap.CORSPaths),
				})
			}
			m.Grants = append(m.Grants, grantEntry{
				Src:          typedListFromStrings(g.Src),
				Dst:          optionalTypedList(g.Dst),
				Capabilities: caps,
			})
		}
	}
	if len(c.Quotas) > 0 {
		m.Quotas = make(map[string]quotaEntry, len(c.Quotas))
		for name, q := range c.Quotas {
			m.Quotas[name] = quotaEntry{
				Capacity: types.StringValue(q.Capacity),
				Rate:     types.StringValue(q.Rate),
				OnExceed: nullableString(q.OnExceed),
			}
		}
	}
	if len(c.Hooks) > 0 {
		m.Hooks = make(map[string]hookEntry, len(c.Hooks))
		for name, h := range c.Hooks {
			m.Hooks[name] = hookEntry{
				URL:           types.StringValue(h.URL),
				Authorization: nullableString(h.Authorization),
				Timeout:       nullableString(h.Timeout),
				Disabled:      nullableBool(h.Disabled),
				FailPolicy:    nullableString(h.FailPolicy),
				Preference:    nullableInt64(h.Preference),
			}
		}
	}
	if c.AutoCostBasis != nil {
		m.AutoCostBasis = types.BoolValue(*c.AutoCostBasis)
	}
	for _, s := range opaqueSections {
		if raw := *s.raw(c); len(raw) > 0 {
			*s.attr(&m) = jsontypes.NewNormalizedValue(string(raw))
		}
	}
	return m
}

func stringsFromTypedList(in []types.String) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.ValueString())
	}
	return out
}

func typedListFromStrings(in []string) []types.String {
	out := make([]types.String, 0, len(in))
	for _, s := range in {
		out = append(out, types.StringValue(s))
	}
	return out
}

func nullableString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func nullableBool(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

func nullableInt64(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}

func compatibilityToAPI(c *compatibilityEntry) *aperture.Compatibility {
	if c == nil {
		return nil
	}
	flag := func(b types.Bool) *bool {
		if b.IsNull() {
			return nil
		}
		v := b.ValueBool()
		return &v
	}
	return &aperture.Compatibility{
		OpenAIChat:            flag(c.OpenAIChat),
		OpenAIResponses:       flag(c.OpenAIResponses),
		AnthropicMessages:     flag(c.AnthropicMessages),
		GeminiGenerateContent: flag(c.GeminiGenerateContent),
		BedrockModelInvoke:    flag(c.BedrockModelInvoke),
		GoogleGenerateContent: flag(c.GoogleGenerateContent),
		GoogleRawPredict:      flag(c.GoogleRawPredict),
		BedrockConverse:       flag(c.BedrockConverse),
	}
}

func compatibilityFromAPI(c *aperture.Compatibility) *compatibilityEntry {
	if c == nil {
		return nil
	}
	return &compatibilityEntry{
		OpenAIChat:            nullableBool(c.OpenAIChat),
		OpenAIResponses:       nullableBool(c.OpenAIResponses),
		AnthropicMessages:     nullableBool(c.AnthropicMessages),
		GeminiGenerateContent: nullableBool(c.GeminiGenerateContent),
		BedrockModelInvoke:    nullableBool(c.BedrockModelInvoke),
		GoogleGenerateContent: nullableBool(c.GoogleGenerateContent),
		GoogleRawPredict:      nullableBool(c.GoogleRawPredict),
		BedrockConverse:       nullableBool(c.BedrockConverse),
	}
}

func modelCostMapToAPI(in []modelCostEntry) []aperture.ModelCost {
	if len(in) == 0 {
		return nil
	}
	out := make([]aperture.ModelCost, 0, len(in))
	for _, e := range in {
		mc := aperture.ModelCost{Match: e.Match.ValueString(), As: e.As.ValueString()}
		if !e.Adjustment.IsNull() {
			v := e.Adjustment.ValueFloat64()
			mc.Adjustment = &v
		}
		out = append(out, mc)
	}
	return out
}

func modelCostMapFromAPI(in []aperture.ModelCost) []modelCostEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]modelCostEntry, 0, len(in))
	for _, mc := range in {
		e := modelCostEntry{Match: types.StringValue(mc.Match), As: types.StringValue(mc.As), Adjustment: types.Float64Null()}
		if mc.Adjustment != nil {
			e.Adjustment = types.Float64Value(*mc.Adjustment)
		}
		out = append(out, e)
	}
	return out
}

// optionalStrings is nil for an empty list, which the wire form omits.
func optionalStrings(in []types.String) []string {
	if len(in) == 0 {
		return nil
	}
	return stringsFromTypedList(in)
}

// optionalTypedList is nil for an empty list, which Terraform reads as
// null rather than an empty list.
func optionalTypedList(in []string) []types.String {
	if len(in) == 0 {
		return nil
	}
	return typedListFromStrings(in)
}

func boolPointer(b types.Bool) *bool {
	if b.IsNull() {
		return nil
	}
	v := b.ValueBool()
	return &v
}

func grantQuotasToAPI(in []grantQuotaEntry) []aperture.GrantQuota {
	if len(in) == 0 {
		return nil
	}
	out := make([]aperture.GrantQuota, 0, len(in))
	for _, q := range in {
		out = append(out, aperture.GrantQuota{Bucket: q.Bucket.ValueString(), Overdrafts: optionalStrings(q.Overdrafts)})
	}
	return out
}

func grantQuotasFromAPI(in []aperture.GrantQuota) []grantQuotaEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]grantQuotaEntry, 0, len(in))
	for _, q := range in {
		out = append(out, grantQuotaEntry{Bucket: types.StringValue(q.Bucket), Overdrafts: optionalTypedList(q.Overdrafts)})
	}
	return out
}

func sendHooksToAPI(in []sendHookEntry) []aperture.SendHook {
	if len(in) == 0 {
		return nil
	}
	out := make([]aperture.SendHook, 0, len(in))
	for _, h := range in {
		out = append(out, aperture.SendHook{Name: h.Name.ValueString(), Events: stringsFromTypedList(h.Events), Send: stringsFromTypedList(h.Send)})
	}
	return out
}

func sendHooksFromAPI(in []aperture.SendHook) []sendHookEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]sendHookEntry, 0, len(in))
	for _, h := range in {
		out = append(out, sendHookEntry{Name: types.StringValue(h.Name), Events: typedListFromStrings(h.Events), Send: typedListFromStrings(h.Send)})
	}
	return out
}

func opaqueSectionAttribute(description string) schema.Attribute {
	return schema.StringAttribute{
		Optional:   true,
		CustomType: jsontypes.NormalizedType{},
		Description: description + " Left unset, the gateway's section is neither read nor changed;" +
			" once set, it replaces the whole section, and removing it from the configuration leaves the section as it is.",
	}
}

// opaqueSection ties a JSON section of the model to its field in the wire
// form.
type opaqueSection struct {
	attr func(*configResourceModel) *jsontypes.Normalized
	raw  func(*aperture.Config) *json.RawMessage
}

var opaqueSections = []opaqueSection{
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.Exporters }, func(c *aperture.Config) *json.RawMessage { return &c.Exporters }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.Database }, func(c *aperture.Config) *json.RawMessage { return &c.Database }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.Connectors }, func(c *aperture.Config) *json.RawMessage { return &c.Connectors }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.ChatModels }, func(c *aperture.Config) *json.RawMessage { return &c.ChatModels }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.Flags }, func(c *aperture.Config) *json.RawMessage { return &c.Flags }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.MCP }, func(c *aperture.Config) *json.RawMessage { return &c.MCP }},
	{func(m *configResourceModel) *jsontypes.Normalized { return &m.TempGrants }, func(c *aperture.Config) *json.RawMessage { return &c.TempGrants }},
}

// leaveUnmanagedSectionsNull nulls the JSON sections that prior state does
// not manage, so a section nobody declared reads back as unset rather
// than as a diff.
func leaveUnmanagedSectionsNull(server, prior *configResourceModel) {
	for _, s := range opaqueSections {
		if s.attr(prior).IsNull() {
			*s.attr(server) = jsontypes.NewNormalizedNull()
		}
	}
}
