package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

var _ datasource.DataSourceWithConfigure = (*addressesDataSource)(nil)

type addressesDataSource struct {
	client *client.Client
}

type addressesModel struct {
	Entitlement int64          `tfsdk:"entitlement"`
	Addresses   []addressModel `tfsdk:"addresses"`
}

type addressModel struct {
	IP         string  `tfsdk:"ip"`
	Location   string  `tfsdk:"location"`
	Assignment string  `tfsdk:"assignment"`
	TunnelID   *string `tfsdk:"tunnel_id"`
	Block      *string `tfsdk:"block"`
	AssignedAt *string `tfsdk:"assigned_at"`
}

func addressAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"ip":         computedString("The address (dotted IPv4)."),
		"location":   computedString("The address's location id."),
		"assignment": computedString("Where it goes: `tunnel`, `bgp` or `unassigned`."),
		"tunnel_id": computedString(
			"The tunnel it's routed down, or for `bgp` the one announcing it now. Null when none.",
		),
		"block":       computedString("The block it was bought in (`72.244.89.16/30`), null for a single address."),
		"assigned_at": computedString("When the account got it (RFC 3339)."),
	}
}

func addressesFromAPI(addresses []opwanapi.Address) []addressModel {
	models := make([]addressModel, len(addresses))
	for i, address := range addresses {
		models[i] = addressModel{
			IP:         address.Ip,
			Location:   address.Location,
			Assignment: string(address.Assignment.Kind),
			TunnelID:   address.Assignment.Tunnel,
			Block:      address.Block,
			AssignedAt: formatTime(address.AssignedAt),
		}
	}
	return models
}

// NewAddressesDataSource returns the opwan_addresses data source.
func NewAddressesDataSource() datasource.DataSource {
	return &addressesDataSource{}
}

func (d *addressesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_addresses"
}

func (d *addressesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Every address of the account, lowest first, with where each is routed.",
		Attributes: map[string]schema.Attribute{
			"entitlement": computedInt64(
				"How many addresses the account pays for (one per tunnel plus the extra addresses).",
			),
			"addresses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The addresses, lowest first.",
				NestedObject:        schema.NestedAttributeObject{Attributes: addressAttributes()},
			},
		},
	}
}

func (d *addressesDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *addressesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.client.ListAddresses(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list the addresses", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressesModel{
		Entitlement: int64(list.Entitlement),
		Addresses:   addressesFromAPI(list.Addresses),
	})...)
}
