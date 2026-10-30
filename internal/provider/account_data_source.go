package provider

import (
	"time"

	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

var _ datasource.DataSourceWithConfigure = (*accountDataSource)(nil)

type accountDataSource struct {
	client *client.Client
}

type accountModel struct {
	ID           string             `tfsdk:"id"`
	Email        string             `tfsdk:"email"`
	Status       string             `tfsdk:"status"`
	CreatedAt    string             `tfsdk:"created_at"`
	Location     *string            `tfsdk:"location"`
	Addresses    []addressModel     `tfsdk:"addresses"`
	Subscription *subscriptionModel `tfsdk:"subscription"`
	Usage        monthUsageModel    `tfsdk:"usage"`
}

type addressModel struct {
	IP       string `tfsdk:"ip"`
	Location string `tfsdk:"location"`
}

type subscriptionModel struct {
	Plan               string  `tfsdk:"plan"`
	Status             string  `tfsdk:"status"`
	ExtraIPs           int64   `tfsdk:"extra_ips"`
	MonthlyTB          int64   `tfsdk:"monthly_tb"`
	MonthlyTBCounted   int64   `tfsdk:"monthly_tb_counted"`
	ExtraTB            int64   `tfsdk:"extra_tb"`
	PeakMbps           int64   `tfsdk:"peak_mbps"`
	CancelAtPeriodEnd  bool    `tfsdk:"cancel_at_period_end"`
	CurrentPeriodStart *string `tfsdk:"current_period_start"`
	CurrentPeriodEnd   *string `tfsdk:"current_period_end"`
	SuspendAt          *string `tfsdk:"suspend_at"`
	ReleaseAt          *string `tfsdk:"release_at"`
}

type monthUsageModel struct {
	Month          string `tfsdk:"month"`
	RxBytes        int64  `tfsdk:"rx_bytes"`
	TxBytes        int64  `tfsdk:"tx_bytes"`
	UsedBytes      int64  `tfsdk:"used_bytes"`
	AllowanceBytes int64  `tfsdk:"allowance_bytes"`
	Throttled      string `tfsdk:"throttled"`
}

// NewAccountDataSource returns the opwan_account data source.
func NewAccountDataSource() datasource.DataSource {
	return &accountDataSource{}
}

func (d *accountDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *accountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account the API token belongs to: its addresses, subscription and this month's usage.",
		Attributes: map[string]schema.Attribute{
			"id":         computedString("The account id (UUID)."),
			"email":      computedString("The op account's email."),
			"status":     computedString("`active`, `suspended` or `closed`."),
			"created_at": computedString("When the account was created (RFC 3339)."),
			"location":   computedString("The location the account chose, null before it chose one."),
			"addresses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The account's public IPv4 addresses.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip":       computedString("The address (dotted IPv4)."),
						"location": computedString("The address's location id."),
					},
				},
			},
			"subscription": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The subscription, null without one.",
				Attributes: map[string]schema.Attribute{
					"plan": computedString("`base` or `plus`."),
					"status": computedString(
						"The Stripe subscription status (`active`, `past_due`, `canceled`, ...).",
					),
					"extra_ips":            computedInt64("Extra addresses on top of the first."),
					"monthly_tb":           computedInt64("Monthly data add-on, TB, billed every month."),
					"monthly_tb_counted":   computedInt64("Monthly data in this month's allowance, TB."),
					"extra_tb":             computedInt64("One-time data bought for the current month, TB."),
					"peak_mbps":            computedInt64("The speed ceiling, Mbps."),
					"cancel_at_period_end": computedBool("The subscription ends with the current period."),
					"current_period_start": computedString("Start of the billing period (RFC 3339)."),
					"current_period_end":   computedString("End of the billing period (RFC 3339)."),
					"suspend_at": computedString(
						"While a payment is late, when the tunnel will be suspended.",
					),
					"release_at": computedString("After cancellation, when the addresses will be released."),
				},
			},
			"usage": schema.SingleNestedAttribute{
				Computed: true,
				MarkdownDescription: "Data used this month (UTC). " +
					"For daily figures or another month use the `opwan_usage` data source.",
				Attributes: map[string]schema.Attribute{
					"month":           computedString("`YYYY-MM`."),
					"rx_bytes":        computedInt64("Bytes to you."),
					"tx_bytes":        computedInt64("Bytes from you."),
					"used_bytes":      computedInt64("rx + tx, what counts against the allowance."),
					"allowance_bytes": computedInt64("The plan's allowance plus this month's data add-ons."),
					"throttled": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: THROTTLED_DESCRIPTION,
					},
				},
			},
		},
	}
}

func (d *accountDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *accountDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	me, err := d.client.GetMe(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the account", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, accountFromAPI(me))...)
}

func accountFromAPI(me *opwanapi.Me) accountModel {
	addresses := make([]addressModel, len(me.Addresses))
	for i, address := range me.Addresses {
		addresses[i] = addressModel{IP: address.Ip, Location: address.Location}
	}

	return accountModel{
		ID:           me.Account.Id.String(),
		Email:        me.Account.Email,
		Status:       string(me.Account.Status),
		CreatedAt:    me.Account.CreatedAt.Format(time.RFC3339),
		Location:     me.Account.Location,
		Addresses:    addresses,
		Subscription: subscriptionFromAPI(me.Subscription),
		Usage: monthUsageModel{
			Month:          me.Usage.Month,
			RxBytes:        me.Usage.RxBytes,
			TxBytes:        me.Usage.TxBytes,
			UsedBytes:      me.Usage.UsedBytes,
			AllowanceBytes: me.Usage.AllowanceBytes,
			Throttled:      string(me.Usage.Throttled),
		},
	}
}

func subscriptionFromAPI(sub *opwanapi.Subscription) *subscriptionModel {
	if sub == nil {
		return nil
	}
	return &subscriptionModel{
		Plan:               string(sub.Plan),
		Status:             string(sub.Status),
		ExtraIPs:           int64(sub.ExtraIps),
		MonthlyTB:          int64(sub.MonthlyTb),
		MonthlyTBCounted:   int64(sub.MonthlyTbCounted),
		ExtraTB:            int64(sub.ExtraTb),
		PeakMbps:           int64(sub.PeakMbps),
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
		CurrentPeriodStart: formatTime(sub.CurrentPeriodStart),
		CurrentPeriodEnd:   formatTime(sub.CurrentPeriodEnd),
		SuspendAt:          formatTime(sub.SuspendAt),
		ReleaseAt:          formatTime(sub.ReleaseAt),
	}
}
