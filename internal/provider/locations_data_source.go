package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

var _ datasource.DataSourceWithConfigure = (*locationsDataSource)(nil)

type locationsDataSource struct {
	client *client.Client
}

type locationsModel struct {
	Locations []locationModel `tfsdk:"locations"`
}

type locationModel struct {
	ID          string            `tfsdk:"id"`
	Name        string            `tfsdk:"name"`
	City        string            `tfsdk:"city"`
	Country     string            `tfsdk:"country"`
	NetworkGbps int64             `tfsdk:"network_gbps"`
	TestIP      *string           `tfsdk:"test_ip"`
	PingHost    *string           `tfsdk:"ping_host"`
	Available   bool              `tfsdk:"available"`
	Maintenance *maintenanceModel `tfsdk:"maintenance"`
	TunnelTypes []string          `tfsdk:"tunnel_types"`
}

type maintenanceModel struct {
	StartsAt   string `tfsdk:"starts_at"`
	EndsAt     string `tfsdk:"ends_at"`
	Reason     string `tfsdk:"reason"`
	InProgress bool   `tfsdk:"in_progress"`
}

// NewLocationsDataSource returns the opwan_locations data source.
func NewLocationsDataSource() datasource.DataSource {
	return &locationsDataSource{}
}

func (d *locationsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_locations"
}

func (d *locationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The exit locations. Needs no API token.",
		Attributes: map[string]schema.Attribute{
			"locations": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every location.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           computedString("Location id (`dfw`), used as `opwan_tunnel.location`."),
						"name":         computedString("Display name."),
						"city":         computedString("City."),
						"country":      computedString("ISO 3166-1 alpha-2 country code (`US`)."),
						"network_gbps": computedInt64("Network capacity in Gbps."),
						"test_ip":      computedString("An address that answers ping, to measure latency. Null: none."),
						"ping_host": computedString(
							"Host name of the HTTPS latency test (`GET https://<ping_host>/ping`). Null: none.",
						),
						"available":    computedBool("Has a free address for a new customer now."),
						"tunnel_types": computedStringList("The tunnel types offered here now: `wireguard`, `l2tp_ipsec`."),
						"maintenance": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: "The maintenance window in progress, else the next one. Null: none ahead.",
							Attributes: map[string]schema.Attribute{
								"starts_at":   computedString("RFC 3339."),
								"ends_at":     computedString("RFC 3339."),
								"reason":      computedString("What is being done."),
								"in_progress": computedBool("The window has started."),
							},
						},
					},
				},
			},
		},
	}
}

func (d *locationsDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *locationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiLocations, err := d.client.ListLocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list locations", err.Error())
		return
	}

	locations := make([]locationModel, len(apiLocations))
	for i, location := range apiLocations {
		locations[i] = locationModel{
			ID:          location.Id,
			Name:        location.Name,
			City:        location.City,
			Country:     location.Country,
			NetworkGbps: int64(location.NetworkGbps),
			TestIP:      location.TestIp,
			PingHost:    location.PingHost,
			Available:   location.Available,
			TunnelTypes: tunnelTypeStrings(location.TunnelTypes),
		}
		if m := location.Maintenance; m != nil {
			locations[i].Maintenance = &maintenanceModel{
				StartsAt:   m.StartsAt.Format(time.RFC3339),
				EndsAt:     m.EndsAt.Format(time.RFC3339),
				Reason:     m.Reason,
				InProgress: m.InProgress,
			}
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, locationsModel{Locations: locations})...)
}

func tunnelTypeStrings(types []opwanapi.TunnelType) []string {
	values := make([]string, len(types))
	for i, t := range types {
		values[i] = string(t)
	}
	return values
}
