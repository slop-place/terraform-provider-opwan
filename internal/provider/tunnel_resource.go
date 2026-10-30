package provider

import (
	"context"

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

// TUNNEL_ID is the id of the account's one tunnel.
const TUNNEL_ID = "tunnel"

var (
	_ resource.ResourceWithConfigure   = (*tunnelResource)(nil)
	_ resource.ResourceWithImportState = (*tunnelResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*tunnelResource)(nil)
)

type tunnelResource struct {
	client *client.Client
}

type tunnelResourceModel struct {
	ID                  types.String `tfsdk:"id"`
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
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account's WireGuard tunnel. An account has one tunnel: " +
			"creating this resource when one exists takes it over and sets its key. " +
			"Changing `public_key` re-keys the tunnel in place. The location can't change once the tunnel exists. " +
			"opwan has no way to delete a tunnel, so destroying this resource only removes it from the Terraform state.\n\n" +
			"Only the public key is sent to opwan. Generate the key pair with `opwan_wireguard_key`, " +
			"or yourself with `wg genkey | tee private.key | wg pubkey`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `" + TUNNEL_ID + "`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "A location id from the `opwan_locations` data source (`dfw`). " +
					"Must be the location chosen at checkout when one was. Can't change once the tunnel exists.",
				Required: true,
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
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The exit's WireGuard endpoint, `host:port`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_public_key": schema.StringAttribute{
				MarkdownDescription: "The exit's WireGuard public key. " +
					"Null until the exit has reported it, seconds after the tunnel is created.",
				Computed: true,
			},
			"addresses": schema.ListAttribute{
				MarkdownDescription: "The tunnel's addresses as /32s (the wg-quick `Address`). " +
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
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *tunnelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *tunnelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	me, err := r.client.GetMe(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel", err.Error())
		return
	}
	if me.Tunnel == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tunnelResourceFromAPI(me.Tunnel))...)
}

func (r *tunnelResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("The opwan tunnel still exists",
		"opwan has no API to delete a tunnel, so it was only removed from the Terraform state. "+
			"It keeps its addresses until the subscription ends; cancel the subscription in the dashboard to end it.")
}

func (r *tunnelResource) ImportState(
	ctx context.Context,
	_ resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), TUNNEL_ID)...)
}

func (r *tunnelResource) put(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var model tunnelResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	tunnel, err := r.client.PutTunnel(ctx, opwanapi.TunnelRequest{
		Location: model.Location.ValueString(),
		Pubkey:   model.PublicKey.ValueString(),
	})
	if client.HasCode(err, client.CODE_LOCATION_LOCKED) {
		diags.AddAttributeError(path.Root("location"), "Wrong location for this account", err.Error())
		return
	}
	if err != nil {
		diags.AddError("Unable to write the tunnel", err.Error())
		return
	}
	diags.Append(state.Set(ctx, tunnelResourceFromAPI(tunnel))...)
}

func tunnelResourceFromAPI(tunnel *opwanapi.Tunnel) tunnelResourceModel {
	return tunnelResourceModel{
		ID:                  types.StringValue(TUNNEL_ID),
		Location:            types.StringValue(tunnel.Location),
		PublicKey:           types.StringValue(tunnel.CustomerPubkey),
		State:               types.StringValue(string(tunnel.State)),
		CreatedAt:           timeValue(tunnel.CreatedAt),
		Endpoint:            types.StringValue(tunnel.Config.Endpoint),
		ServerPublicKey:     types.StringPointerValue(tunnel.Config.ServerPubkey),
		Addresses:           stringListValue(tunnel.Config.Addresses),
		AllowedIPs:          stringListValue(tunnel.Config.AllowedIps),
		MTU:                 types.Int64Value(int64(tunnel.Config.Mtu)),
		PersistentKeepalive: types.Int64Value(int64(tunnel.Config.PersistentKeepalive)),
	}
}
