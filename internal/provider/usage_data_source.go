package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var (
	_            datasource.DataSourceWithConfigure = (*usageDataSource)(nil)
	monthPattern                                    = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
)

type usageDataSource struct {
	client *client.Client
}

type usageModel struct {
	Month          string             `tfsdk:"month"`
	RxBytes        int64              `tfsdk:"rx_bytes"`
	TxBytes        int64              `tfsdk:"tx_bytes"`
	UsedBytes      int64              `tfsdk:"used_bytes"`
	AllowanceBytes int64              `tfsdk:"allowance_bytes"`
	Days           []usageDayModel    `tfsdk:"days"`
	Live           *tunnelStatusModel `tfsdk:"live"`
}

type usageDayModel struct {
	Date    string `tfsdk:"date"`
	RxBytes int64  `tfsdk:"rx_bytes"`
	TxBytes int64  `tfsdk:"tx_bytes"`
}

// NewUsageDataSource returns the opwan_usage data source.
func NewUsageDataSource() datasource.DataSource {
	return &usageDataSource{}
}

func (d *usageDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_usage"
}

func (d *usageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Data used in a calendar month (UTC), per day, and the live throughput. " +
			"Bytes count both directions through the exit; a TB is 10^12 bytes.",
		Attributes: map[string]schema.Attribute{
			"month": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`YYYY-MM` (UTC). Defaults to the current month.",
				Validators:          []validator.String{stringvalidator.RegexMatches(monthPattern, "must be YYYY-MM")},
			},
			"rx_bytes":        computedInt64("Bytes to you."),
			"tx_bytes":        computedInt64("Bytes from you."),
			"used_bytes":      computedInt64("rx + tx, what counts against the allowance."),
			"allowance_bytes": computedInt64("The month's allowance."),
			"days": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "One entry per UTC day with traffic, oldest first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"date":     computedString("`YYYY-MM-DD`."),
						"rx_bytes": computedInt64("Bytes to you."),
						"tx_bytes": computedInt64("Bytes from you."),
					},
				},
			},
			"live": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The tunnel's live state, null without a tunnel.",
				Attributes:          tunnelStatusAttributes(),
			},
		},
	}
}

func (d *usageDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *usageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var month types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("month"), &month)...)
	if resp.Diagnostics.HasError() {
		return
	}

	usage, err := d.client.GetUsage(ctx, month.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read usage", err.Error())
		return
	}

	days := make([]usageDayModel, len(usage.Days))
	for i, day := range usage.Days {
		days[i] = usageDayModel{Date: day.Date.String(), RxBytes: day.RxBytes, TxBytes: day.TxBytes}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, usageModel{
		Month:          usage.Month,
		RxBytes:        usage.RxBytes,
		TxBytes:        usage.TxBytes,
		UsedBytes:      usage.UsedBytes,
		AllowanceBytes: usage.AllowanceBytes,
		Days:           days,
		Live:           tunnelStatusFromAPI(usage.Live),
	})...)
}
