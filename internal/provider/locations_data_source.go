package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
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
}

type maintenanceModel struct {
	StartsAt   string `tfsdk:"starts_at"`
	EndsAt     string `tfsdk:"ends_at"`
	Reason     string `tfsdk:"reason"`
	InProgress bool   `tfsdk:"in_progress"`
}

func NewLocationsDataSource() datasource.DataSource {
	return &locationsDataSource{}
}

func (d *locationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
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
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Location id (`dfw`), used as `opwan_tunnel.location`."},
						"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Display name."},
						"city":         schema.StringAttribute{Computed: true, MarkdownDescription: "City."},
						"country":      schema.StringAttribute{Computed: true, MarkdownDescription: "ISO 3166-1 alpha-2 country code (`US`)."},
						"network_gbps": schema.Int64Attribute{Computed: true, MarkdownDescription: "Network capacity in Gbps."},
						"test_ip":      schema.StringAttribute{Computed: true, MarkdownDescription: "An address that answers ping, to measure latency. Null: none."},
						"ping_host":    schema.StringAttribute{Computed: true, MarkdownDescription: "Host name of the HTTPS latency test (`GET https://<ping_host>/ping`). Null: none."},
						"available":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Has a free address for a new customer now."},
						"maintenance": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: "The maintenance window in progress, else the next one. Null: none ahead.",
							Attributes: map[string]schema.Attribute{
								"starts_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339."},
								"ends_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339."},
								"reason":      schema.StringAttribute{Computed: true, MarkdownDescription: "What is being done."},
								"in_progress": schema.BoolAttribute{Computed: true, MarkdownDescription: "The window has started."},
							},
						},
					},
				},
			},
		},
	}
}

func (d *locationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
