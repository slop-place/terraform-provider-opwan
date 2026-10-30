package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*tunnelDataSource)(nil)

type tunnelDataSource struct {
	client *client.Client
}

type tunnelDataSourceModel struct {
	ID                  string            `tfsdk:"id"`
	Location            string            `tfsdk:"location"`
	PublicKey           string            `tfsdk:"public_key"`
	State               string            `tfsdk:"state"`
	CreatedAt           string            `tfsdk:"created_at"`
	Endpoint            string            `tfsdk:"endpoint"`
	ServerPublicKey     *string           `tfsdk:"server_public_key"`
	Addresses           []string          `tfsdk:"addresses"`
	AllowedIPs          []string          `tfsdk:"allowed_ips"`
	MTU                 int64             `tfsdk:"mtu"`
	PersistentKeepalive int64             `tfsdk:"persistent_keepalive"`
	Status              tunnelStatusModel `tfsdk:"status"`
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
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account's tunnel: its client configuration " +
			"(everything a WireGuard config needs besides the private key) and live status. " +
			"Fails when the account has no tunnel.",
		Attributes: map[string]schema.Attribute{
			"id":         computedString("Always `" + TUNNEL_ID + "`."),
			"location":   computedString("The tunnel's location id."),
			"public_key": computedString("The client's WireGuard public key."),
			"state":      computedString("`pending_payment`, `active`, `suspended` or `canceled`."),
			"created_at": computedString("When the tunnel was created (RFC 3339)."),
			"endpoint":   computedString("The exit's WireGuard endpoint, `host:port`."),
			"server_public_key": computedString(
				"The exit's WireGuard public key, null until the exit has reported it.",
			),
			"addresses": computedStringList(
				"The tunnel's addresses as /32s, empty until the subscription is active.",
			),
			"allowed_ips":          computedStringList("Suggested `AllowedIPs` of the peer on the client."),
			"mtu":                  computedInt64("Suggested client MTU."),
			"persistent_keepalive": computedInt64("Suggested `PersistentKeepalive` in seconds."),
			"status": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The tunnel's live state, as the exit last reported it.",
				Attributes:          tunnelStatusAttributes(),
			},
		},
	}
}

func (d *tunnelDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *tunnelDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	me, err := d.client.GetMe(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel", err.Error())
		return
	}
	tunnel := me.Tunnel
	if tunnel == nil {
		resp.Diagnostics.AddError(
			"The account has no tunnel",
			"Create one with the opwan_tunnel resource or in the dashboard.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelDataSourceModel{
		ID:                  TUNNEL_ID,
		Location:            tunnel.Location,
		PublicKey:           tunnel.CustomerPubkey,
		State:               string(tunnel.State),
		CreatedAt:           tunnel.CreatedAt.Format(time.RFC3339),
		Endpoint:            tunnel.Config.Endpoint,
		ServerPublicKey:     tunnel.Config.ServerPubkey,
		Addresses:           nonNil(tunnel.Config.Addresses),
		AllowedIPs:          nonNil(tunnel.Config.AllowedIps),
		MTU:                 int64(tunnel.Config.Mtu),
		PersistentKeepalive: int64(tunnel.Config.PersistentKeepalive),
		Status:              *tunnelStatusFromAPI(&tunnel.Status),
	})...)
}

// nonNil keeps an empty JSON array an empty list rather than null in the state.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
