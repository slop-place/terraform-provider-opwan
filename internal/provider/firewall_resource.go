package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// Firewall values as the API spells them.
const (
	ACTION_ALLOW   = string(opwanapi.FirewallRuleActionAllow)
	ACTION_DENY    = string(opwanapi.FirewallRuleActionDeny)
	PROTOCOL_TCP   = string(opwanapi.Tcp)
	PROTOCOL_UDP   = string(opwanapi.Udp)
	PROTOCOL_ICMP  = string(opwanapi.Icmp)
	PROTOCOL_ANY   = string(opwanapi.Any)
	DEFAULT_ACTION = opwanapi.FirewallDefaultAllow
)

var (
	_ resource.ResourceWithConfigure      = (*firewallResource)(nil)
	_ resource.ResourceWithImportState    = (*firewallResource)(nil)
	_ resource.ResourceWithValidateConfig = (*firewallResource)(nil)
)

func firewallRuleType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"action":   types.StringType,
		"protocol": types.StringType,
		"ports":    types.StringType,
		"source":   types.StringType,
	}}
}

type firewallResource struct {
	client *client.Client
}

type firewallResourceModel struct {
	ID            types.String        `tfsdk:"id"`
	IP            types.String        `tfsdk:"ip"`
	DefaultAction types.String        `tfsdk:"default_action"`
	Rules         []firewallRuleModel `tfsdk:"rules"`
}

type firewallRuleModel struct {
	Action   types.String `tfsdk:"action"`
	Protocol types.String `tfsdk:"protocol"`
	Ports    types.String `tfsdk:"ports"`
	Source   types.String `tfsdk:"source"`
}

// NewFirewallResource returns the opwan_firewall resource.
func NewFirewallResource() resource.Resource {
	return &firewallResource{}
}

func (r *firewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

func (r *firewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The inbound firewall of one of the account's addresses. " +
			"Rules are checked in order and the first match wins. TCP port 25 is always blocked, whatever the rules say. " +
			"Destroying this resource restores the firewall of a new address: allow everything, no rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The address, same as `ip`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip": schema.StringAttribute{
				MarkdownDescription: "One of the account's addresses (dotted IPv4).",
				Required:            true,
				Validators:          []validator.String{ipv4Address()},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"default_action": schema.StringAttribute{
				MarkdownDescription: "What happens to inbound traffic no rule matches: `allow` or `deny`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(ACTION_ALLOW, ACTION_DENY)},
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: fmt.Sprintf(
					"The rules, checked in order, first match wins. At most %d.",
					MAX_FIREWALL_RULES,
				),
				Optional: true,
				Computed: true,
				Default: listdefault.StaticValue(
					types.ListValueMust(firewallRuleType(), nil),
				),
				Validators: []validator.List{listvalidator.SizeAtMost(MAX_FIREWALL_RULES)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							MarkdownDescription: "`allow` or `deny`.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.OneOf(ACTION_ALLOW, ACTION_DENY)},
						},
						"protocol": schema.StringAttribute{
							MarkdownDescription: "`tcp`, `udp`, `icmp` or `any`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(PROTOCOL_TCP, PROTOCOL_UDP, PROTOCOL_ICMP, PROTOCOL_ANY),
							},
						},
						"ports": schema.StringAttribute{
							MarkdownDescription: fmt.Sprintf(
								"Ports and ranges separated by commas (`\"80, 443, 8000-8100\"`, at most %d), `tcp` and `udp` only. "+
									"Unset for every port. A TCP rule can't include port %d: split ranges around it (`\"1-24, 26-1024\"`).",
								MAX_PORT_ITEMS_PER_RULE,
								BLOCKED_SMTP_PORT,
							),
							Optional:   true,
							Validators: []validator.String{portList()},
						},
						"source": schema.StringAttribute{
							MarkdownDescription: "An IPv4 address or CIDR the traffic must come from. Unset for anywhere.",
							Optional:            true,
							Validators:          []validator.String{ipv4Source()},
						},
					},
				},
			},
		},
	}
}

func (r *firewallResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *firewallResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var rules types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("rules"), &rules)...)
	if resp.Diagnostics.HasError() || rules.IsNull() || rules.IsUnknown() {
		return
	}

	for i, element := range rules.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		var rule firewallRuleModel
		resp.Diagnostics.Append(object.As(ctx, &rule, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		validateRule(rule, path.Root("rules").AtListIndex(i), resp)
	}
}

func validateRule(rule firewallRuleModel, rulePath path.Path, resp *resource.ValidateConfigResponse) {
	if rule.Protocol.IsUnknown() || rule.Ports.IsUnknown() || rule.Ports.IsNull() {
		return
	}
	protocol := rule.Protocol.ValueString()
	if protocol != PROTOCOL_TCP && protocol != PROTOCOL_UDP {
		resp.Diagnostics.AddAttributeError(rulePath.AtName("ports"), "Ports need tcp or udp",
			fmt.Sprintf("A %s rule matches every port: remove ports, or use tcp or udp.", protocol))
		return
	}
	if protocol != PROTOCOL_TCP {
		return
	}
	ranges, err := parsePorts(rule.Ports.ValueString())
	if err != nil {
		return
	}
	for _, r := range ranges {
		if r.contains(BLOCKED_SMTP_PORT) {
			resp.Diagnostics.AddAttributeError(
				rulePath.AtName("ports"),
				"TCP port 25 is always blocked",
				fmt.Sprintf(
					"opwan refuses a TCP rule naming port %d. Split the range around it, e.g. \"1-24, 26-1024\".",
					BLOCKED_SMTP_PORT,
				),
			)
			return
		}
	}
}

func (r *firewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *firewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	firewall, err := r.client.GetFirewall(ctx, state.IP.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the firewall", err.Error())
		return
	}

	state.applyFirewall(firewall)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *firewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *firewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.PutFirewall(
		ctx,
		state.IP.ValueString(),
		opwanapi.Firewall{Default: DEFAULT_ACTION, Rules: []opwanapi.FirewallRule{}},
	)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to reset the firewall", err.Error())
	}
}

func (r *firewallResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), req.ID)...)
}

func (r *firewallResource) put(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var model firewallResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	firewall, err := r.client.PutFirewall(ctx, model.IP.ValueString(), model.toAPI())
	if err != nil {
		diags.AddError("Unable to write the firewall", err.Error())
		return
	}
	model.applyFirewall(firewall)
	diags.Append(state.Set(ctx, model)...)
}

func (m *firewallResourceModel) toAPI() opwanapi.Firewall {
	rules := make([]opwanapi.FirewallRule, len(m.Rules))
	for i, rule := range m.Rules {
		rules[i] = opwanapi.FirewallRule{
			Action: opwanapi.FirewallRuleAction(rule.Action.ValueString()),
			Proto:  opwanapi.FirewallRuleProto(rule.Protocol.ValueString()),
			Ports:  rule.Ports.ValueStringPointer(),
			Source: rule.Source.ValueStringPointer(),
		}
	}
	return opwanapi.Firewall{Default: opwanapi.FirewallDefault(m.DefaultAction.ValueString()), Rules: rules}
}

// applyFirewall takes the stored firewall, keeping the model's own spelling of ports and sources
// opwan stored in a normal form, so "443,80" in config doesn't show as a change to "80, 443".
func (m *firewallResourceModel) applyFirewall(firewall *opwanapi.Firewall) {
	rules := make([]firewallRuleModel, len(firewall.Rules))
	for i, apiRule := range firewall.Rules {
		rules[i] = ruleFromAPI(apiRule)
		if i < len(m.Rules) && rulesEquivalent(m.Rules[i], rules[i]) {
			rules[i] = m.Rules[i]
		}
	}

	m.ID = m.IP
	m.DefaultAction = types.StringValue(string(firewall.Default))
	m.Rules = rules
}

func ruleFromAPI(rule opwanapi.FirewallRule) firewallRuleModel {
	return firewallRuleModel{
		Action:   types.StringValue(string(rule.Action)),
		Protocol: types.StringValue(string(rule.Proto)),
		Ports:    types.StringPointerValue(rule.Ports),
		Source:   types.StringPointerValue(rule.Source),
	}
}

func rulesEquivalent(a, b firewallRuleModel) bool {
	return a.Action.Equal(b.Action) &&
		a.Protocol.Equal(b.Protocol) &&
		optionalEquivalent(a.Ports, b.Ports, portsEquivalent) &&
		optionalEquivalent(a.Source, b.Source, sourcesEquivalent)
}

func optionalEquivalent(a, b types.String, equivalent func(string, string) bool) bool {
	if a.IsNull() || b.IsNull() {
		return a.IsNull() && b.IsNull()
	}
	return equivalent(a.ValueString(), b.ValueString())
}
