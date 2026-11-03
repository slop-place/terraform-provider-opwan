package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*tunnelDataSource)(nil)

type tunnelDataSource struct {
	client *client.Client
}

type tunnelDataSourceModel struct {
	TunnelItemModel

	TunnelID types.String `tfsdk:"tunnel_id"`
}

// NewTunnelDataSource returns the opwan_tunnel data source.
func NewTunnelDataSource() datasource.DataSource {
	return &tunnelDataSource{}
}

func (d *tunnelDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tunnel"
}

func (d *tunnelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := tunnelItemAttributes()
	attributes["tunnel_id"] = schema.StringAttribute{
		MarkdownDescription: "The tunnel to read (`tun_...`). Unset for the account's first tunnel.",
		Optional:            true,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One of the account's tunnels: its client configuration " +
			"(everything a router needs besides its secrets) and live status. " +
			"Fails when the tunnel doesn't exist.",
		Attributes: attributes,
	}
}

func (d *tunnelDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *tunnelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config tunnelDataSourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(ATTR_TUNNEL_ID), &config.TunnelID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tunnel, err := readTunnel(ctx, d.client, config.TunnelID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel", err.Error())
		return
	}
	config.TunnelItemModel = tunnelItemFromAPI(tunnel)
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// nonNil keeps an empty JSON array an empty list rather than null in the state.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
