package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*firewallDataSource)(nil)

type firewallDataSource struct {
	client *client.Client
}

type firewallDataSourceModel struct {
	ID            string              `tfsdk:"id"`
	IP            string              `tfsdk:"ip"`
	DefaultAction string              `tfsdk:"default_action"`
	Rules         []firewallRuleModel `tfsdk:"rules"`
}

// NewFirewallDataSource returns the opwan_firewall data source.
func NewFirewallDataSource() datasource.DataSource {
	return &firewallDataSource{}
}

func (d *firewallDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

func (d *firewallDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The inbound firewall of one of the account's addresses.",
		Attributes: map[string]schema.Attribute{
			"id": computedString("The address, same as `ip`."),
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of the account's addresses (dotted IPv4).",
				Validators:          []validator.String{ipv4Address()},
			},
			"default_action": computedString("What happens to inbound traffic no rule matches: `allow` or `deny`."),
			"rules": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The rules in order, first match wins.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action":   computedString("`allow` or `deny`."),
						"protocol": computedString("`tcp`, `udp`, `icmp` or `any`."),
						"ports": computedString(
							"Ports and ranges in normal form (`\"80, 443\"`), null for every port.",
						),
						"source": computedString(
							"The IPv4 address or CIDR the traffic must come from, null for anywhere.",
						),
					},
				},
			},
		},
	}
}

func (d *firewallDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *firewallDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var ip types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ip"), &ip)...)
	if resp.Diagnostics.HasError() {
		return
	}

	firewall, err := d.client.GetFirewall(ctx, ip.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the firewall", err.Error())
		return
	}

	rules := make([]firewallRuleModel, len(firewall.Rules))
	for i, rule := range firewall.Rules {
		rules[i] = ruleFromAPI(rule)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, firewallDataSourceModel{
		ID:            ip.ValueString(),
		IP:            ip.ValueString(),
		DefaultAction: string(firewall.Default),
		Rules:         rules,
	})...)
}
