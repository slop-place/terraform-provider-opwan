package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// MAX_TUNNEL_NAME_LENGTH is the longest tunnel name opwan accepts.
const MAX_TUNNEL_NAME_LENGTH = 40

var (
	_ resource.ResourceWithConfigure      = (*tunnelResource)(nil)
	_ resource.ResourceWithImportState    = (*tunnelResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*tunnelResource)(nil)
	_ resource.ResourceWithValidateConfig = (*tunnelResource)(nil)
)

type tunnelResource struct {
	client *client.Client
}

type tunnelResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	TunnelID            types.String `tfsdk:"tunnel_id"`
	Name                types.String `tfsdk:"name"`
	Number              types.Int64  `tfsdk:"number"`
	Plan                types.String `tfsdk:"plan"`
	Location            types.String `tfsdk:"location"`
	PublicKey           types.String `tfsdk:"public_key"`
	State               types.String `tfsdk:"state"`
	CreatedAt           types.String `tfsdk:"created_at"`
	Endpoint            types.String `tfsdk:"endpoint"`
	ServerPublicKey     types.String `tfsdk:"server_public_key"`
	Addresses           types.List   `tfsdk:"addresses"`
	AllowedIPs          types.List   `tfsdk:"allowed_ips"`
	MTU                 types.Int64  `tfsdk:"mtu"`
	PersistentKeepalive types.Int64  `tfsdk:"persistent_keepalive"`
}

// NewTunnelResource returns the opwan_tunnel resource.
func NewTunnelResource() resource.Resource {
	return &tunnelResource{}
}

func (r *tunnelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tunnel"
}

func (r *tunnelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One of the account's WireGuard tunnels: its key and name.\n\n" +
			"Without `tunnel_id` this is the account's first tunnel: creating the resource makes it at `location` " +
			"when the account has none, or takes it over and sets its key. " +
			"With `tunnel_id` it takes over that tunnel; add tunnels in the dashboard, as an API token can't buy one.\n\n" +
			"Changing `public_key` re-keys the tunnel in place. The location can't change once the tunnel exists. " +
			"An API token can't delete a tunnel, so destroying this resource only removes it from the Terraform state.\n\n" +
			"Only the public key is sent to opwan. Generate the key pair with `opwan_wireguard_key`, " +
			"or yourself with `wg genkey | tee private.key | wg pubkey`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The tunnel id, same as `tunnel_id`.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"tunnel_id": schema.StringAttribute{
				MarkdownDescription: "The tunnel to manage (`tun_...`, from `opwan_tunnels`). " +
					"Unset for the account's first tunnel. Changing it replaces the resource.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf(
					"The tunnel's name, shown in the dashboard, 1 to %d characters. Unset keeps opwan's.",
					MAX_TUNNEL_NAME_LENGTH,
				),
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, MAX_TUNNEL_NAME_LENGTH)},
				PlanModifiers: keepState,
			},
			"number": schema.Int64Attribute{
				MarkdownDescription: "1, 2, ... in the account.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"plan": schema.StringAttribute{
				MarkdownDescription: "`base` or `plus`. Change it in the dashboard.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "A location id from the `opwan_locations` data source (`dfw`). " +
					"Needed to create the first tunnel; must be the location chosen at checkout when one was. " +
					"Can't change once the tunnel exists.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepState,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "The client's WireGuard public key, base64 of 32 bytes. Never the private key.",
				Required:            true,
				Validators:          []validator.String{wireguardPublicKey()},
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "`pending_payment` (no active subscription yet, no address), `active`, " +
					"`suspended` (a payment is late, traffic blocked) or `canceled` (the subscription ended).",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the tunnel was created (RFC 3339).",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The exit's WireGuard endpoint, `host:port`.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"server_public_key": schema.StringAttribute{
				MarkdownDescription: "The exit's WireGuard public key. " +
					"Null until the exit has reported it, seconds after the tunnel is created.",
				Computed: true,
			},
			"addresses": schema.ListAttribute{
				MarkdownDescription: "The addresses routed down this tunnel, as /32s (the wg-quick `Address`). " +
					"Empty until the subscription is active.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"allowed_ips": schema.ListAttribute{
				MarkdownDescription: "Suggested `AllowedIPs` of the peer on the client.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"mtu": schema.Int64Attribute{
				MarkdownDescription: "Suggested client MTU.",
				Computed:            true,
			},
			"persistent_keepalive": schema.Int64Attribute{
				MarkdownDescription: "Suggested `PersistentKeepalive` in seconds.",
				Computed:            true,
			},
		},
	}
}

func (r *tunnelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *tunnelResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var tunnelID, location types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(ATTR_TUNNEL_ID), &tunnelID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("location"), &location)...)
	if resp.Diagnostics.HasError() || tunnelID.IsUnknown() || location.IsUnknown() {
		return
	}
	if tunnelID.IsNull() && location.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("location"), "Missing location",
			"Set location to create the account's first tunnel, or tunnel_id to take over an existing one.")
	}
}

func (r *tunnelResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var planLocation, stateLocation types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("location"), &planLocation)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("location"), &stateLocation)...)
	if resp.Diagnostics.HasError() || planLocation.IsUnknown() || planLocation.Equal(stateLocation) {
		return
	}

	resp.Diagnostics.AddAttributeError(path.Root("location"), "The tunnel's location can't change",
		"opwan keeps a tunnel at the location it was created in. The tunnel is in "+stateLocation.ValueString()+
			"; set location back to it. Moving needs opwan support.")
}

func (r *tunnelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tunnelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tunnel *opwanapi.TunnelItem
	if plan.TunnelID.IsUnknown() {
		tunnel = r.createFirst(ctx, plan, &resp.Diagnostics)
	} else {
		tunnel = r.adopt(ctx, plan, &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	tunnel = r.applyName(ctx, plan, tunnel, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelResourceFromAPI(tunnel))...)
}

func (r *tunnelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tunnelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tunnel, err := readTunnel(ctx, r.client, state.TunnelID.ValueString())
	if client.IsNotFound(err) || errors.Is(err, errNoTunnel) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelResourceFromAPI(tunnel))...)
}

func (r *tunnelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state tunnelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tunnel, err := r.client.GetTunnel(ctx, state.TunnelID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel", err.Error())
		return
	}
	tunnel = r.applyKey(ctx, plan, tunnel, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tunnel = r.applyName(ctx, plan, tunnel, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelResourceFromAPI(tunnel))...)
}

func (r *tunnelResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("The opwan tunnel still exists",
		"An API token can't delete a tunnel, so it was only removed from the Terraform state. "+
			"Delete it in the dashboard, or cancel the subscription to end the last one.")
}

func (r *tunnelResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	tunnel, err := readTunnel(ctx, r.client, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import the tunnel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(ATTR_TUNNEL_ID), tunnel.Id)...)
}

func (r *tunnelResource) createFirst(
	ctx context.Context,
	plan tunnelResourceModel,
	diags *diag.Diagnostics,
) *opwanapi.TunnelItem {
	_, err := r.client.PutTunnel(ctx, opwanapi.TunnelRequest{
		Location: plan.Location.ValueString(),
		Pubkey:   plan.PublicKey.ValueString(),
	})
	if client.HasCode(err, client.CODE_LOCATION_LOCKED) {
		diags.AddAttributeError(path.Root("location"), "Wrong location for this account", err.Error())
		return nil
	}
	if err != nil {
		diags.AddError("Unable to write the tunnel", err.Error())
		return nil
	}

	tunnel, err := readTunnel(ctx, r.client, FIRST_TUNNEL_ID)
	if err != nil {
		diags.AddError("Unable to read the tunnel", err.Error())
		return nil
	}
	return tunnel
}

func (r *tunnelResource) adopt(
	ctx context.Context,
	plan tunnelResourceModel,
	diags *diag.Diagnostics,
) *opwanapi.TunnelItem {
	tunnel, err := r.client.GetTunnel(ctx, plan.TunnelID.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root(ATTR_TUNNEL_ID), "Unable to read the tunnel", err.Error())
		return nil
	}
	if string(tunnel.Type) != TUNNEL_TYPE_WIREGUARD {
		diags.AddAttributeError(path.Root(ATTR_TUNNEL_ID), "Not a WireGuard tunnel",
			fmt.Sprintf("Tunnel %s is %s; opwan_tunnel manages WireGuard tunnels only.", tunnel.Id, tunnel.Type))
		return nil
	}
	if !plan.Location.IsUnknown() && plan.Location.ValueString() != tunnel.Location {
		diags.AddAttributeError(path.Root("location"), "Wrong location for this tunnel",
			fmt.Sprintf("Tunnel %s is in %s; set location to it, or leave it unset.", tunnel.Id, tunnel.Location))
		return nil
	}
	return r.applyKey(ctx, plan, tunnel, diags)
}

func (r *tunnelResource) applyKey(
	ctx context.Context,
	plan tunnelResourceModel,
	tunnel *opwanapi.TunnelItem,
	diags *diag.Diagnostics,
) *opwanapi.TunnelItem {
	key := plan.PublicKey.ValueString()
	if tunnel.CustomerPubkey != nil && *tunnel.CustomerPubkey == key {
		return tunnel
	}
	updated, err := r.client.PutTunnelKey(ctx, tunnel.Id, key)
	if err != nil {
		diags.AddAttributeError(path.Root("public_key"), "Unable to set the tunnel's key", err.Error())
		return nil
	}
	return updated
}

func (r *tunnelResource) applyName(
	ctx context.Context,
	plan tunnelResourceModel,
	tunnel *opwanapi.TunnelItem,
	diags *diag.Diagnostics,
) *opwanapi.TunnelItem {
	if plan.Name.IsUnknown() || plan.Name.ValueString() == tunnel.Name {
		return tunnel
	}
	updated, err := r.client.RenameTunnel(ctx, tunnel.Id, plan.Name.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("name"), "Unable to rename the tunnel", err.Error())
		return nil
	}
	return updated
}

func tunnelResourceFromAPI(tunnel *opwanapi.TunnelItem) tunnelResourceModel {
	item := tunnelItemFromAPI(tunnel)
	return tunnelResourceModel{
		ID:                  types.StringValue(item.ID),
		TunnelID:            types.StringValue(item.ID),
		Name:                types.StringValue(item.Name),
		Number:              types.Int64Value(item.Number),
		Plan:                types.StringValue(item.Plan),
		Location:            types.StringValue(item.Location),
		PublicKey:           types.StringPointerValue(item.PublicKey),
		State:               types.StringValue(item.State),
		CreatedAt:           types.StringValue(item.CreatedAt),
		Endpoint:            types.StringPointerValue(item.Endpoint),
		ServerPublicKey:     types.StringPointerValue(item.ServerPublicKey),
		Addresses:           stringListValue(item.Addresses),
		AllowedIPs:          stringListValue(item.AllowedIPs),
		MTU:                 types.Int64Value(item.MTU),
		PersistentKeepalive: types.Int64PointerValue(item.PersistentKeepalive),
	}
}
