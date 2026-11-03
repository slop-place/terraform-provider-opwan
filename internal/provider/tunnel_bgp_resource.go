package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// BGP values as the API spells them.
const (
	BGP_EXPORT_DEFAULT = string(opwanapi.TunnelBGPRequestExportDefault)
	BGP_EXPORT_FULL    = string(opwanapi.TunnelBGPRequestExportFull)
	BGP_AUTH_NONE      = string(opwanapi.TunnelBGPRequestAuthNone)
	BGP_AUTH_MD5       = string(opwanapi.TunnelBGPRequestAuthMd5)
	BGP_AUTH_AO        = string(opwanapi.TunnelBGPRequestAuthAo)
)

var (
	_ resource.ResourceWithConfigure   = (*tunnelBGPResource)(nil)
	_ resource.ResourceWithImportState = (*tunnelBGPResource)(nil)
)

type tunnelBGPResource struct {
	client *client.Client
}

type tunnelBGPResourceModel struct {
	ID          types.String `tfsdk:"id"`
	TunnelID    types.String `tfsdk:"tunnel_id"`
	Export      types.String `tfsdk:"export"`
	Auth        types.String `tfsdk:"auth"`
	TTLSecurity types.Bool   `tfsdk:"ttl_security"`
	ASN         types.Int64  `tfsdk:"asn"`
	OurASN      types.Int64  `tfsdk:"our_asn"`
	Local       types.String `tfsdk:"local"`
	Peer        types.String `tfsdk:"peer"`
	HoldTime    types.Int64  `tfsdk:"hold_time"`
	Key         types.String `tfsdk:"key"`
}

// NewTunnelBGPResource returns the opwan_tunnel_bgp resource.
func NewTunnelBGPResource() resource.Resource {
	return &tunnelBGPResource{}
}

func (r *tunnelBGPResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tunnel_bgp"
}

func (r *tunnelBGPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Customer BGP over one tunnel (free). The resource existing turns BGP on; " +
			"destroying it turns BGP off. Addresses assigned to BGP (`opwan_address_assignment`) " +
			"go to whichever of the account's BGP tunnels announces them.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The tunnel id, same as `tunnel_id`.",
				Computed:            true,
				PlanModifiers:       keepString,
			},
			"tunnel_id": schema.StringAttribute{
				MarkdownDescription: "The tunnel to run BGP over (`tun_...`).",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"export": schema.StringAttribute{
				MarkdownDescription: "What opwan sends you: `default` (a default route) or `full` (the full table and a default).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(BGP_EXPORT_DEFAULT),
				Validators:          []validator.String{stringvalidator.OneOf(BGP_EXPORT_DEFAULT, BGP_EXPORT_FULL)},
			},
			"auth": schema.StringAttribute{
				MarkdownDescription: "Session authentication: `none`, `md5` (TCP-MD5) or `ao` (TCP-AO, recommended). " +
					"opwan makes the key; it is in `key`.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString(BGP_AUTH_NONE),
				Validators: []validator.String{stringvalidator.OneOf(BGP_AUTH_NONE, BGP_AUTH_MD5, BGP_AUTH_AO)},
			},
			"ttl_security": schema.BoolAttribute{
				MarkdownDescription: "GTSM (TTL 255, one hop). Keep true unless your BGP speaker is " +
					"one hop behind the router terminating the tunnel.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"asn": schema.Int64Attribute{
				MarkdownDescription: "Your account's AS number, chosen by opwan, the same on every tunnel.",
				Computed:            true,
				PlanModifiers:       keepInt,
			},
			"our_asn": schema.Int64Attribute{
				MarkdownDescription: "opwan's AS number.",
				Computed:            true,
				PlanModifiers:       keepInt,
			},
			"local": schema.StringAttribute{
				MarkdownDescription: "opwan's session address (your neighbor).",
				Computed:            true,
				PlanModifiers:       keepString,
			},
			"peer": schema.StringAttribute{
				MarkdownDescription: "Your session address, to configure on your end of the tunnel.",
				Computed:            true,
				PlanModifiers:       keepString,
			},
			"hold_time": schema.Int64Attribute{
				MarkdownDescription: "The hold time opwan offers, seconds.",
				Computed:            true,
				PlanModifiers:       keepInt,
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "The TCP-MD5 password or TCP-AO key (HMAC-SHA-256, key id 0 both ways). " +
					"Null when `auth` is `none`.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func (r *tunnelBGPResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *tunnelBGPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *tunnelBGPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *tunnelBGPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tunnelBGPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bgp, err := r.client.GetTunnelBGP(ctx, state.TunnelID.ValueString())
	if client.IsNotFound(err) || (err == nil && !bgp.Enabled) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the tunnel's BGP", err.Error())
		return
	}
	r.applyBGP(ctx, &state, bgp, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *tunnelBGPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tunnelBGPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.PutTunnelBGP(ctx, state.TunnelID.ValueString(), opwanapi.TunnelBGPRequest{Enabled: false})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to turn the tunnel's BGP off", err.Error())
	}
}

func (r *tunnelBGPResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(ATTR_TUNNEL_ID), req.ID)...)
}

func (r *tunnelBGPResource) put(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var model tunnelBGPResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	export := opwanapi.TunnelBGPRequestExport(model.Export.ValueString())
	auth := opwanapi.TunnelBGPRequestAuth(model.Auth.ValueString())
	bgp, err := r.client.PutTunnelBGP(ctx, model.TunnelID.ValueString(), opwanapi.TunnelBGPRequest{
		Enabled:     true,
		Export:      &export,
		Auth:        &auth,
		TtlSecurity: model.TTLSecurity.ValueBoolPointer(),
	})
	if err != nil {
		diags.AddError("Unable to write the tunnel's BGP", err.Error())
		return
	}
	r.applyBGP(ctx, &model, bgp, diags)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, model)...)
}

// applyBGP takes the session's settings and, when it has authentication, its key.
func (r *tunnelBGPResource) applyBGP(
	ctx context.Context,
	model *tunnelBGPResourceModel,
	bgp *opwanapi.TunnelBGP,
	diags *diag.Diagnostics,
) {
	model.ID = model.TunnelID
	model.Auth = types.StringValue(string(bgp.Auth))
	model.TTLSecurity = types.BoolValue(bgp.TtlSecurity)
	model.ASN = types.Int64PointerValue(bgp.Asn)
	model.OurASN = types.Int64Value(int64(bgp.OurAsn))
	model.Local = types.StringPointerValue(bgp.Local)
	model.Peer = types.StringPointerValue(bgp.Peer)
	model.HoldTime = types.Int64Value(int64(bgp.HoldTime))
	if bgp.Export != nil {
		model.Export = types.StringValue(string(*bgp.Export))
	}

	model.Key = types.StringNull()
	if string(bgp.Auth) == BGP_AUTH_NONE {
		return
	}
	config, err := r.client.GetTunnelBGPConfig(ctx, model.TunnelID.ValueString())
	if err != nil {
		diags.AddError("Unable to read the tunnel's BGP key", err.Error())
		return
	}
	model.Key = types.StringPointerValue(config.Key)
}
