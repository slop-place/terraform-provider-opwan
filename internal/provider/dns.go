package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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

// DNS_ID is the id of the account's one opdns DNS opt-in.
const DNS_ID = "dns"

// MAX_DNS_PROFILE_ID_LENGTH is the longest opdns profile id opwan accepts.
const MAX_DNS_PROFILE_ID_LENGTH = 64

var (
	_ resource.ResourceWithConfigure     = (*dnsResource)(nil)
	_ resource.ResourceWithImportState   = (*dnsResource)(nil)
	_ datasource.DataSourceWithConfigure = (*dnsDataSource)(nil)
)

type dnsResource struct {
	client *client.Client
}

type dnsResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProfileID   types.String `tfsdk:"profile_id"`
	ProfileName types.String `tfsdk:"profile_name"`
	State       types.String `tfsdk:"state"`
	Resolver    types.String `tfsdk:"resolver"`
	LinkedIPs   types.List   `tfsdk:"linked_ips"`
}

// NewDNSResource returns the opwan_dns resource.
func NewDNSResource() resource.Resource {
	return &dnsResource{}
}

func (r *dnsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns"
}

func (r *dnsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "opdns DNS: links the account's static addresses to an opdns profile, " +
			"so lookups from them to `resolver` are filtered by it. The tunnel configuration doesn't change: " +
			"point devices that use a static address at `resolver`. Needs an opdns account on the same op account. " +
			"Destroying this resource turns opdns DNS off.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `" + DNS_ID + "`.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"profile_id": schema.StringAttribute{
				MarkdownDescription: "An opdns profile id, from the `opwan_dns` data source's `profiles`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, MAX_DNS_PROFILE_ID_LENGTH)},
			},
			"profile_name": schema.StringAttribute{
				MarkdownDescription: "The profile's name.",
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "`pending` (opdns not updated yet, opwan keeps trying), `linked` or `error`.",
				Computed:            true,
			},
			"resolver": schema.StringAttribute{
				MarkdownDescription: "opdns's resolver address.",
				Computed:            true,
				PlanModifiers:       keepState,
			},
			"linked_ips": schema.ListAttribute{
				MarkdownDescription: "The addresses opdns has linked to the profile now.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (r *dnsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *dnsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *dnsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.put(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *dnsResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	settings, err := r.client.GetDNS(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS", err.Error())
		return
	}
	if settings.Profile == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dnsResourceFromAPI(settings))...)
}

func (r *dnsResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, err := r.client.DeleteDNS(ctx); err != nil {
		resp.Diagnostics.AddError("Unable to turn DNS off", err.Error())
	}
}

func (r *dnsResource) ImportState(
	ctx context.Context,
	_ resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), DNS_ID)...)
}

func (r *dnsResource) put(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, diags *diag.Diagnostics) {
	var model dnsResourceModel
	diags.Append(plan.Get(ctx, &model)...)
	if diags.HasError() {
		return
	}

	settings, err := r.client.PutDNS(ctx, model.ProfileID.ValueString())
	if err != nil {
		diags.AddError("Unable to turn DNS on", err.Error())
		return
	}
	diags.Append(state.Set(ctx, dnsResourceFromAPI(settings))...)
}

func dnsResourceFromAPI(settings *opwanapi.DNSSettings) dnsResourceModel {
	model := dnsResourceModel{
		ID:          types.StringValue(DNS_ID),
		ProfileID:   types.StringNull(),
		ProfileName: types.StringNull(),
		State:       types.StringValue(string(settings.State)),
		Resolver:    types.StringValue(settings.Resolver),
		LinkedIPs:   stringListValue(settings.LinkedIps),
	}
	if settings.Profile != nil {
		model.ProfileID = types.StringValue(settings.Profile.Id)
		model.ProfileName = types.StringValue(settings.Profile.Name)
	}
	return model
}

type dnsDataSource struct {
	client *client.Client
}

type dnsDataSourceModel struct {
	Available    bool              `tfsdk:"available"`
	OpdnsAccount *bool             `tfsdk:"opdns_account"`
	Resolver     string            `tfsdk:"resolver"`
	State        string            `tfsdk:"state"`
	Profile      *dnsProfileModel  `tfsdk:"profile"`
	Profiles     []dnsProfileModel `tfsdk:"profiles"`
	LinkedIPs    []string          `tfsdk:"linked_ips"`
	Error        *string           `tfsdk:"error"`
}

type dnsProfileModel struct {
	ID   string `tfsdk:"id"`
	Name string `tfsdk:"name"`
}

// NewDNSDataSource returns the opwan_dns data source.
func NewDNSDataSource() datasource.DataSource {
	return &dnsDataSource{}
}

func (d *dnsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_dns"
}

func (d *dnsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	profileAttributes := map[string]dsschema.Attribute{
		"id":   computedString("The profile id, for `opwan_dns.profile_id`."),
		"name": computedString("The profile's name."),
	}
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "The opdns DNS opt-in and the opdns profiles to pick from.",
		Attributes: map[string]dsschema.Attribute{
			"available": computedBool("This server offers opdns DNS."),
			"opdns_account": computedBool(
				"The op account has an opdns account; null when opdns couldn't be asked just now.",
			),
			"resolver": computedString("opdns's resolver address."),
			"state":    computedString("`off`, `pending`, `linked` or `error`."),
			"profile": dsschema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The profile picked, null while opdns DNS is off.",
				Attributes:          profileAttributes,
			},
			"profiles": dsschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The opdns profiles to pick from, empty without an opdns account.",
				NestedObject:        dsschema.NestedAttributeObject{Attributes: profileAttributes},
			},
			"linked_ips": computedStringList("The addresses opdns has linked to the profile now."),
			"error":      computedString("The last problem, in plain words. Null: none."),
		},
	}
}

func (d *dnsDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *dnsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	settings, err := d.client.GetDNS(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS", err.Error())
		return
	}

	profiles := make([]dnsProfileModel, len(settings.Profiles))
	for i, profile := range settings.Profiles {
		profiles[i] = dnsProfileModel{ID: profile.Id, Name: profile.Name}
	}
	var picked *dnsProfileModel
	if settings.Profile != nil {
		profile := dnsProfileModel{ID: settings.Profile.Id, Name: settings.Profile.Name}
		picked = &profile
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dnsDataSourceModel{
		Available:    settings.Available,
		OpdnsAccount: settings.OpdnsAccount,
		Resolver:     settings.Resolver,
		State:        string(settings.State),
		Profile:      picked,
		Profiles:     profiles,
		LinkedIPs:    nonNil(settings.LinkedIps),
		Error:        settings.Error,
	})...)
}
