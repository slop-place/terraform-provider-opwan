package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

// Environment variables the provider reads its settings from.
const (
	ENV_API_TOKEN = "OPWAN_API_TOKEN" //nolint:gosec // the name of an environment variable, not a credential
	ENV_ENDPOINT  = "OPWAN_ENDPOINT"
)

var _ provider.Provider = (*opwanProvider)(nil)

type opwanProvider struct {
	version string
}

type providerModel struct {
	APIToken types.String `tfsdk:"api_token"`
	Endpoint types.String `tfsdk:"endpoint"`
}

// New returns the opwan provider factory for version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &opwanProvider{version: version}
	}
}

func (p *opwanProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "opwan"
	resp.Version = p.version
}

func (p *opwanProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage an [opwan](https://opwan.io) tunnel and the inbound firewalls of its addresses.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "An opwan API token (`opwan_...`) from the dashboard's Settings → API tokens. " +
					"Reading needs a `read` token; managing the tunnel and firewalls needs `write`. " +
					"Defaults to `" + ENV_API_TOKEN + "`. " +
					"The `opwan_locations`, `opwan_plans` and `opwan_meta` data sources work without one.",
				Optional:  true,
				Sensitive: true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The opwan API's base URL. " +
					"Defaults to `" + ENV_ENDPOINT + "`, else `" + client.DEFAULT_ENDPOINT + "`.",
				Optional: true,
			},
		},
	}
}

func (p *opwanProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("api_token"), "Unknown opwan API token",
			"The api_token is not known until apply. Set it to a value known at plan time, or use "+ENV_API_TOKEN+".")
	}
	if config.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown opwan endpoint",
			"The endpoint is not known until apply. Set it to a value known at plan time, or use "+ENV_ENDPOINT+".")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	token := valueOrEnv(config.APIToken, ENV_API_TOKEN, "")
	endpoint := valueOrEnv(config.Endpoint, ENV_ENDPOINT, client.DEFAULT_ENDPOINT)

	c, err := client.New(endpoint, token, "terraform-provider-opwan/"+p.version)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create the opwan client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func valueOrEnv(value types.String, envKey, fallback string) string {
	if !value.IsNull() {
		return value.ValueString()
	}
	if env := os.Getenv(envKey); env != "" {
		return env
	}
	return fallback
}

func (p *opwanProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTunnelResource,
		NewFirewallResource,
		NewWireguardKeyResource,
	}
}

func (p *opwanProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAccountDataSource,
		NewTunnelDataSource,
		NewFirewallDataSource,
		NewUsageDataSource,
		NewLocationsDataSource,
		NewPlansDataSource,
		NewMetaDataSource,
	}
}
