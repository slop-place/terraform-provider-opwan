package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*tunnelsDataSource)(nil)

type tunnelsDataSource struct {
	client *client.Client
}

type tunnelsModel struct {
	MaxTunnels int64             `tfsdk:"max_tunnels"`
	Tunnels    []TunnelItemModel `tfsdk:"tunnels"`
}

// NewTunnelsDataSource returns the opwan_tunnels data source.
func NewTunnelsDataSource() datasource.DataSource {
	return &tunnelsDataSource{}
}

func (d *tunnelsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tunnels"
}

func (d *tunnelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Every tunnel of the account, by number. Each has its own plan, type, usage and configuration.",
		Attributes: map[string]schema.Attribute{
			"max_tunnels": computedInt64("How many tunnels an account may have."),
			"tunnels": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The tunnels, by number.",
				NestedObject:        schema.NestedAttributeObject{Attributes: tunnelItemAttributes()},
			},
		},
	}
}

func (d *tunnelsDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *tunnelsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.client.ListTunnels(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list the tunnels", err.Error())
		return
	}

	tunnels := make([]TunnelItemModel, len(list.Tunnels))
	for i := range list.Tunnels {
		tunnels[i] = tunnelItemFromAPI(&list.Tunnels[i])
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelsModel{
		MaxTunnels: int64(list.MaxTunnels),
		Tunnels:    tunnels,
	})...)
}
