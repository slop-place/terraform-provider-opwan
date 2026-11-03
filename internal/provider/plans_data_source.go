package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*plansDataSource)(nil)

type plansDataSource struct {
	client *client.Client
}

type plansModel struct {
	Currency string      `tfsdk:"currency"`
	Plans    []planModel `tfsdk:"plans"`
	AddOns   addOnsModel `tfsdk:"add_ons"`
}

type planModel struct {
	ID             string `tfsdk:"id"`
	Name           string `tfsdk:"name"`
	PriceCents     int64  `tfsdk:"price_cents"`
	CeilMbps       int64  `tfsdk:"ceil_mbps"`
	RateMbps       int64  `tfsdk:"rate_mbps"`
	SlowMbps       int64  `tfsdk:"slow_mbps"`
	AllowanceBytes int64  `tfsdk:"allowance_bytes"`
}

type addOnsModel struct {
	ExtraIPPriceCents   int64           `tfsdk:"extra_ip_price_cents"`
	MaxExtraIPs         int64           `tfsdk:"max_extra_ips"`
	MonthlyTBPriceCents int64           `tfsdk:"monthly_tb_price_cents"`
	MaxMonthlyTB        int64           `tfsdk:"max_monthly_tb"`
	ExtraTBPriceCents   int64           `tfsdk:"extra_tb_price_cents"`
	MaxExtraTB          int64           `tfsdk:"max_extra_tb"`
	PeakPlan            string          `tfsdk:"peak_plan"`
	PeakTiers           []peakTierModel `tfsdk:"peak_tiers"`
	MaxTunnels          int64           `tfsdk:"max_tunnels"`
	BlockSizes          []int64         `tfsdk:"block_sizes"`
}

type peakTierModel struct {
	Mbps       int64 `tfsdk:"mbps"`
	PriceCents int64 `tfsdk:"price_cents"`
}

// NewPlansDataSource returns the opwan_plans data source.
func NewPlansDataSource() datasource.DataSource {
	return &plansDataSource{}
}

func (d *plansDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_plans"
}

func (d *plansDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The plans and add-ons on sale. " +
			"Prices are in cents of `currency`, tax exclusive. Needs no API token.",
		Attributes: map[string]schema.Attribute{
			"currency": computedString("`usd`."),
			"plans": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The plans.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              computedString("`base` or `plus`."),
						"name":            computedString("Display name."),
						"price_cents":     computedInt64("Per month."),
						"ceil_mbps":       computedInt64("Speed ceiling."),
						"rate_mbps":       computedInt64("Guaranteed rate under contention."),
						"slow_mbps":       computedInt64("The ceiling once the month's allowance is used."),
						"allowance_bytes": computedInt64("Data per calendar month, both directions."),
					},
				},
			},
			"add_ons": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The add-ons.",
				Attributes: map[string]schema.Attribute{
					"extra_ip_price_cents": computedInt64("Per extra address per month."),
					"max_extra_ips":        computedInt64("Most extra addresses at checkout; afterwards they are bought in blocks."),
					"max_tunnels":          computedInt64("Tunnels per account, each billed at its plan's price."),
					"block_sizes": schema.ListAttribute{
						Computed:            true,
						ElementType:         types.Int64Type,
						MarkdownDescription: "The address block sizes an account may buy at once (1, 2, 4, 8).",
					},
					"monthly_tb_price_cents": computedInt64("Monthly data, per TB per month."),
					"max_monthly_tb":         computedInt64("Most monthly data a subscription can have, TB."),
					"extra_tb_price_cents":   computedInt64("One-time data, per TB, added to the current month."),
					"max_extra_tb":           computedInt64("Most TB one data add-on purchase can buy."),
					"peak_plan":              computedString("The plan the peak speed tiers apply to."),
					"peak_tiers": schema.ListNestedAttribute{
						Computed:            true,
						MarkdownDescription: "Peak speed tiers, slowest first. The first is the plan's own ceiling at 0 cents.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"mbps":        computedInt64("Speed ceiling, Mbps."),
								"price_cents": computedInt64("Per month on top of the plan."),
							},
						},
					},
				},
			},
		},
	}
}

func (d *plansDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *plansDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	catalogue, err := d.client.ListPlans(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list plans", err.Error())
		return
	}

	plans := make([]planModel, len(catalogue.Plans))
	for i, plan := range catalogue.Plans {
		plans[i] = planModel{
			ID:             string(plan.Id),
			Name:           plan.Name,
			PriceCents:     int64(plan.PriceCents),
			CeilMbps:       int64(plan.CeilMbps),
			RateMbps:       int64(plan.RateMbps),
			SlowMbps:       int64(plan.SlowMbps),
			AllowanceBytes: plan.AllowanceBytes,
		}
	}

	addOns := catalogue.AddOns
	tiers := make([]peakTierModel, len(addOns.PeakTiers))
	for i, tier := range addOns.PeakTiers {
		tiers[i] = peakTierModel{Mbps: int64(tier.Mbps), PriceCents: int64(tier.PriceCents)}
	}

	blockSizes := make([]int64, len(addOns.BlockSizes))
	for i, size := range addOns.BlockSizes {
		blockSizes[i] = int64(size)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plansModel{
		Currency: string(catalogue.Currency),
		Plans:    plans,
		AddOns: addOnsModel{
			ExtraIPPriceCents:   int64(addOns.ExtraIpPriceCents),
			MaxExtraIPs:         int64(addOns.MaxExtraIps),
			MonthlyTBPriceCents: int64(addOns.MonthlyTbPriceCents),
			MaxMonthlyTB:        int64(addOns.MaxMonthlyTb),
			ExtraTBPriceCents:   int64(addOns.ExtraTbPriceCents),
			MaxExtraTB:          int64(addOns.MaxExtraTb),
			PeakPlan:            string(addOns.PeakPlan),
			PeakTiers:           tiers,
			MaxTunnels:          int64(addOns.MaxTunnels),
			BlockSizes:          blockSizes,
		},
	})...)
}
