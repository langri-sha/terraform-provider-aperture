// Package provider implements the terraform-provider-aperture plugin.
package provider

import (
	"context"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/langri-sha/aperture/internal/aperture"
)

// New returns a providerserver factory.
func New(version, commit string) func() provider.Provider {
	return func() provider.Provider {
		return &apertureProvider{version: version, commit: commit}
	}
}

type apertureProvider struct {
	version string
	commit  string
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *apertureProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "aperture"
	resp.Version = p.version
}

func (p *apertureProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for Aperture by Tailscale. Authentication is by Tailscale identity at the network layer — the caller must be on the tailnet with the admin role granted in Aperture's configuration.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Required:    true,
				Description: "Full base URL of the Aperture admin API including the /aperture path prefix, e.g. https://ai.<tailnet>.ts.net/aperture.",
				// Validators run in order: URL format check must pass before protocol and path checks.
				Validators: []validator.String{
					endpointURLValidator{},
					endpointProtocolValidator{},
					endpointPathValidator{},
				},
			},
		},
	}
}

func (p *apertureProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := aperture.NewClient(aperture.ClientConfig{
		Endpoint:  data.Endpoint.ValueString(),
		UserAgent: "terraform-provider-aperture/" + p.version,
	})

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *apertureProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newConfigResource,
	}
}

func (p *apertureProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

// endpointURLValidator validates that the endpoint is a valid URL.
type endpointURLValidator struct{}

func (v endpointURLValidator) Description(ctx context.Context) string {
	return "endpoint must be a valid URL"
}

func (v endpointURLValidator) MarkdownDescription(ctx context.Context) string {
	return "endpoint must be a valid URL"
}

func (v endpointURLValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	_, err := url.Parse(value)
	if err != nil {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			req.Path,
			"Invalid Endpoint URL",
			"endpoint must be a valid URL",
		))
	}
}

// endpointProtocolValidator validates that the endpoint uses https:// protocol.
type endpointProtocolValidator struct{}

func (v endpointProtocolValidator) Description(ctx context.Context) string {
	return "endpoint must use https:// protocol"
}

func (v endpointProtocolValidator) MarkdownDescription(ctx context.Context) string {
	return "endpoint must use https:// protocol"
}

func (v endpointProtocolValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	parsed, err := url.Parse(value)
	if err != nil {
		// Let the URL validator handle this
		return
	}

	if parsed.Scheme != "https" {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			req.Path,
			"Invalid Endpoint Protocol",
			"endpoint must use https:// protocol, not "+parsed.Scheme+"://",
		))
	}
}

// endpointPathValidator validates that the endpoint path ends with /aperture.
type endpointPathValidator struct{}

func (v endpointPathValidator) Description(ctx context.Context) string {
	return "endpoint path must end with /aperture"
}

func (v endpointPathValidator) MarkdownDescription(ctx context.Context) string {
	return "endpoint path must end with /aperture"
}

func (v endpointPathValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	parsed, err := url.Parse(value)
	if err != nil {
		// Let the URL validator handle this
		return
	}

	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/aperture") {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
			req.Path,
			"Invalid Endpoint Path",
			"endpoint path must end with /aperture, got "+parsed.Path,
		))
	}
}
