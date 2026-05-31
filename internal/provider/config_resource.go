package provider

import (
	"context"
	"errors"
	"fmt"

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
	ID            types.String                `tfsdk:"id"`
	ETag          types.String                `tfsdk:"etag"`
	Providers     map[string]providerEntry    `tfsdk:"providers"`
	Grants        []grantEntry                `tfsdk:"grants"`
	Quotas        map[string]quotaEntry       `tfsdk:"quotas"`
	Hooks         map[string]hookEntry        `tfsdk:"hooks"`
	AutoCostBasis types.Bool                  `tfsdk:"auto_cost_basis"`
}

type providerEntry struct {
	BaseURL       types.String      `tfsdk:"baseurl"`
	Models        []types.String    `tfsdk:"models"`
	APIKey        types.String      `tfsdk:"apikey"`
	Authorization types.String      `tfsdk:"authorization"`
	Name          types.String      `tfsdk:"name"`
	Description   types.String      `tfsdk:"description"`
	CostBasis     types.String      `tfsdk:"cost_basis"`
	Preference    types.Int64       `tfsdk:"preference"`
	Disabled      types.Bool        `tfsdk:"disabled"`
	AddHeaders    map[string]string `tfsdk:"add_headers"`
}

type grantEntry struct {
	Src          []types.String    `tfsdk:"src"`
	Capabilities []capabilityEntry `tfsdk:"capabilities"`
}

type capabilityEntry struct {
	Role   types.String `tfsdk:"role"`
	Models types.String `tfsdk:"models"`
}

type quotaEntry struct {
	Capacity types.Float64 `tfsdk:"capacity"`
	Rate     types.Float64 `tfsdk:"rate"`
	OnExceed types.String  `tfsdk:"on_exceed"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"providers": schema.MapNestedAttribute{
				Optional:    true,
				Description: "LLM provider configurations, keyed by provider name (openai, anthropic, google, ...).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"baseurl":       schema.StringAttribute{Required: true},
						"models":        schema.ListAttribute{ElementType: types.StringType, Required: true},
						"apikey":        schema.StringAttribute{Optional: true, Sensitive: true},
						"authorization": schema.StringAttribute{Optional: true, Sensitive: true},
						"name":          schema.StringAttribute{Optional: true},
						"description":   schema.StringAttribute{Optional: true},
						"cost_basis":    schema.StringAttribute{Optional: true},
						"preference":    schema.Int64Attribute{Optional: true},
						"disabled":      schema.BoolAttribute{Optional: true},
						"add_headers":   schema.MapAttribute{ElementType: types.StringType, Optional: true, Sensitive: true},
					},
				},
			},
			"grants": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"src": schema.ListAttribute{ElementType: types.StringType, Required: true},
						"capabilities": schema.ListNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"role":   schema.StringAttribute{Optional: true, Description: "user | admin"},
									"models": schema.StringAttribute{Optional: true, Description: "Glob in provider/model form, e.g. anthropic/**"},
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
						"capacity":  schema.Float64Attribute{Required: true},
						"rate":      schema.Float64Attribute{Required: true},
						"on_exceed": schema.StringAttribute{Optional: true},
					},
				},
			},
			"hooks": schema.MapNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"url":           schema.StringAttribute{Required: true},
						"apikey":        schema.StringAttribute{Optional: true, Sensitive: true},
						"authorization": schema.StringAttribute{Optional: true, Sensitive: true},
						"timeout":       schema.StringAttribute{Optional: true},
						"disabled":      schema.BoolAttribute{Optional: true},
						"fail_policy":   schema.StringAttribute{Optional: true},
						"preference":    schema.Int64Attribute{Optional: true},
					},
				},
			},
			"auto_cost_basis": schema.BoolAttribute{Optional: true},
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

	// Aperture always has a config — Create against an empty backend
	// would 404. Read first so we can If-Match the PUT and so users
	// can `terraform import` and then `terraform apply` symmetrically.
	_, etag, err := r.client.GetConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("read existing aperture config", err.Error())
		return
	}

	if !r.applyPlan(ctx, &plan, etag, &resp.Diagnostics) {
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

// applyPlan is the shared body of Create and Update: validate via
// the API, PUT, and stamp the result back into the plan model.
// Returns false on any error (after recording diagnostics).
func (r *configResource) applyPlan(ctx context.Context, plan *configResourceModel, etag string, diags *diag.Diagnostics) bool {
	doc, err := toAPIConfig(plan)
	if err != nil {
		diags.AddError("render aperture config", err.Error())
		return false
	}
	body, err := aperture.MarshalConfigDocument(doc)
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
			diags.AddError(
				"aperture configuration changed externally",
				"Re-run `terraform apply -refresh-only` (or `terraform plan`) to pick up the latest state, then apply again.",
			)
			return false
		}
		diags.AddError("write aperture config", err.Error())
		return false
	}
	plan.ID = types.StringValue(singletonID)
	plan.ETag = types.StringValue(newEtag)
	return true
}

// preserveSensitiveFromPrior copies sensitive values from prior
// state into a freshly-Read model. Aperture's GET redacts apikeys
// and authorization headers; accepting the redaction marker would
// either leak it into state or cause spurious diffs on every plan.
// We trust the user's HCL as the source of truth for those fields.
func preserveSensitiveFromPrior(server, prior *configResourceModel) {
	for name, sp := range server.Providers {
		if pp, ok := prior.Providers[name]; ok {
			if !pp.APIKey.IsNull() {
				sp.APIKey = pp.APIKey
			}
			if !pp.Authorization.IsNull() {
				sp.Authorization = pp.Authorization
			}
			if len(pp.AddHeaders) > 0 {
				sp.AddHeaders = pp.AddHeaders
			}
			server.Providers[name] = sp
		}
	}
	for name, sh := range server.Hooks {
		if ph, ok := prior.Hooks[name]; ok {
			if !ph.APIKey.IsNull() {
				sh.APIKey = ph.APIKey
			}
			if !ph.Authorization.IsNull() {
				sh.Authorization = ph.Authorization
			}
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
			if !p.Name.IsNull() {
				ap.Name = p.Name.ValueString()
			}
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
				ap.AddHeaders = p.AddHeaders
			}
			out.Providers[name] = ap
		}
	}

	if len(m.Grants) > 0 {
		out.Grants = make([]aperture.Grant, 0, len(m.Grants))
		for _, g := range m.Grants {
			caps := make([]aperture.GrantCapability, 0, len(g.Capabilities))
			for _, c := range g.Capabilities {
				if c.Role.IsNull() && c.Models.IsNull() {
					return nil, fmt.Errorf("grant capability must set role or models")
				}
				caps = append(caps, aperture.GrantCapability{
					Role:   c.Role.ValueString(),
					Models: c.Models.ValueString(),
				})
			}
			out.Grants = append(out.Grants, aperture.Grant{
				Src: stringsFromTypedList(g.Src),
				App: aperture.GrantApp{Aperture: caps},
			})
		}
	}

	if len(m.Quotas) > 0 {
		out.Quotas = make(map[string]aperture.Quota, len(m.Quotas))
		for name, q := range m.Quotas {
			out.Quotas[name] = aperture.Quota{
				Capacity: q.Capacity.ValueFloat64(),
				Rate:     q.Rate.ValueFloat64(),
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

	return out, nil
}

// fromAPIConfig is the inverse of toAPIConfig.
func fromAPIConfig(c *aperture.Config) configResourceModel {
	m := configResourceModel{}

	if len(c.Providers) > 0 {
		m.Providers = make(map[string]providerEntry, len(c.Providers))
		for name, p := range c.Providers {
			m.Providers[name] = providerEntry{
				BaseURL:       types.StringValue(p.BaseURL),
				Models:        typedListFromStrings(p.Models),
				APIKey:        nullableString(p.APIKey),
				Authorization: nullableString(p.Authorization),
				Name:          nullableString(p.Name),
				Description:   nullableString(p.Description),
				CostBasis:     nullableString(p.CostBasis),
				Preference:    nullableInt64(p.Preference),
				Disabled:      nullableBool(p.Disabled),
				AddHeaders:    p.AddHeaders,
			}
		}
	}
	if len(c.Grants) > 0 {
		m.Grants = make([]grantEntry, 0, len(c.Grants))
		for _, g := range c.Grants {
			caps := make([]capabilityEntry, 0, len(g.App.Aperture))
			for _, cap := range g.App.Aperture {
				caps = append(caps, capabilityEntry{
					Role:   nullableString(cap.Role),
					Models: nullableString(cap.Models),
				})
			}
			m.Grants = append(m.Grants, grantEntry{
				Src:          typedListFromStrings(g.Src),
				Capabilities: caps,
			})
		}
	}
	if len(c.Quotas) > 0 {
		m.Quotas = make(map[string]quotaEntry, len(c.Quotas))
		for name, q := range c.Quotas {
			m.Quotas[name] = quotaEntry{
				Capacity: types.Float64Value(q.Capacity),
				Rate:     types.Float64Value(q.Rate),
				OnExceed: nullableString(q.OnExceed),
			}
		}
	}
	if len(c.Hooks) > 0 {
		m.Hooks = make(map[string]hookEntry, len(c.Hooks))
		for name, h := range c.Hooks {
			m.Hooks[name] = hookEntry{
				URL:           types.StringValue(h.URL),
				APIKey:        nullableString(h.APIKey),
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
