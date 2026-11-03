package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// ATTR_KIND is the attribute naming where an address goes.
const ATTR_KIND = "kind"

// Assignment kinds as the API spells them.
const (
	ASSIGNMENT_TUNNEL     = string(opwanapi.AssignmentKindTunnel)
	ASSIGNMENT_BGP        = string(opwanapi.AssignmentKindBgp)
	ASSIGNMENT_UNASSIGNED = string(opwanapi.AssignmentKindUnassigned)
)

var (
	_ resource.ResourceWithConfigure      = (*addressAssignmentResource)(nil)
	_ resource.ResourceWithImportState    = (*addressAssignmentResource)(nil)
	_ resource.ResourceWithValidateConfig = (*addressAssignmentResource)(nil)
)

type addressAssignmentResource struct {
	client *client.Client
}

type addressAssignmentModel struct {
	ID       types.String `tfsdk:"id"`
	IP       types.String `tfsdk:"ip"`
	Kind     types.String `tfsdk:"kind"`
	TunnelID types.String `tfsdk:"tunnel_id"`
	Block    types.String `tfsdk:"block"`
	Location types.String `tfsdk:"location"`
}

// NewAddressAssignmentResource returns the opwan_address_assignment resource.
func NewAddressAssignmentResource() resource.Resource {
	return &addressAssignmentResource{}
}

func (r *addressAssignmentResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_assignment"
}

func (r *addressAssignmentResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	keepState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Where one of the account's addresses is routed: down a tunnel, to BGP, or nowhere. " +
			"Moving an address is instant and free, and its firewall follows it. " +
			"An address bought in a block moves with its whole block, so manage one address per block. " +
			"Destroying this resource leaves the address where it is.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The address, same as `ip`.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"ip": schema.StringAttribute{
				MarkdownDescription: "One of the account's addresses (dotted IPv4).",
				Required:            true,
				Validators:          []validator.String{ipv4Address()},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "`tunnel` (routed down `tunnel_id`), " +
					"`bgp` (goes to whichever of the account's BGP tunnels announces it, dropped while none does) " +
					"or `unassigned` (kept, traffic to it dropped).",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(ASSIGNMENT_TUNNEL, ASSIGNMENT_BGP, ASSIGNMENT_UNASSIGNED),
				},
			},
			"tunnel_id": schema.StringAttribute{
				MarkdownDescription: "The tunnel to route the address down (`tun_...`). Only with `kind = \"tunnel\"`.",
				Optional:            true,
			},
			"block": schema.StringAttribute{
				MarkdownDescription: "The block the address was bought in (`72.244.89.16/30`), null for a single address.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "The address's location id.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
		},
	}
}

func (r *addressAssignmentResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *addressAssignmentResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var kind, tunnelID types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(ATTR_KIND), &kind)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(ATTR_TUNNEL_ID), &tunnelID)...)
	if resp.Diagnostics.HasError() || kind.IsUnknown() || kind.IsNull() || tunnelID.IsUnknown() {
		return
	}

	routedToTunnel := kind.ValueString() == ASSIGNMENT_TUNNEL
	if routedToTunnel && tunnelID.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root(ATTR_TUNNEL_ID), "Missing tunnel_id",
			"Set tunnel_id to the tunnel the address goes down.")
	}
	if !routedToTunnel && !tunnelID.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root(ATTR_TUNNEL_ID), "tunnel_id needs kind tunnel",
			fmt.Sprintf("An address of kind %s isn't routed down one tunnel: remove tunnel_id.", kind.ValueString()))
	}
}

func (r *addressAssignmentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *addressAssignmentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *addressAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state addressAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := r.client.ListAddresses(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list the addresses", err.Error())
		return
	}
	for i := range list.Addresses {
		if list.Addresses[i].Ip == state.IP.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, assignmentFromAPI(&list.Addresses[i]))...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *addressAssignmentResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *addressAssignmentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), req.ID)...)
}

func (r *addressAssignmentResource) put(
	ctx context.Context,
	plan tfsdk.Plan,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	var model addressAssignmentModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	address, err := r.client.AssignAddress(ctx, model.IP.ValueString(), model.toAPI())
	if err != nil {
		diags.AddError("Unable to assign the address", err.Error())
		return
	}
	diags.Append(state.Set(ctx, assignmentFromAPI(address))...)
}

func (m *addressAssignmentModel) toAPI() opwanapi.AssignmentRequest {
	if m.Kind.ValueString() == ASSIGNMENT_TUNNEL {
		return opwanapi.AssignmentRequest{Tunnel: m.TunnelID.ValueStringPointer()}
	}
	kind := opwanapi.AssignmentRequestKind(m.Kind.ValueString())
	return opwanapi.AssignmentRequest{Kind: &kind}
}

// assignmentFromAPI keeps tunnel_id only for kind tunnel: for bgp the API names the tunnel announcing it now.
func assignmentFromAPI(address *opwanapi.Address) addressAssignmentModel {
	model := addressAssignmentModel{
		ID:       types.StringValue(address.Ip),
		IP:       types.StringValue(address.Ip),
		Kind:     types.StringValue(string(address.Assignment.Kind)),
		TunnelID: types.StringNull(),
		Block:    types.StringPointerValue(address.Block),
		Location: types.StringValue(address.Location),
	}
	if address.Assignment.Kind == opwanapi.AssignmentKindTunnel {
		model.TunnelID = types.StringPointerValue(address.Assignment.Tunnel)
	}
	return model
}
