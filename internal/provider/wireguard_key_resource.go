package provider

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*wireguardKeyResource)(nil)

type wireguardKeyResource struct{}

type wireguardKeyModel struct {
	ID         types.String `tfsdk:"id"`
	Keepers    types.Map    `tfsdk:"keepers"`
	PrivateKey types.String `tfsdk:"private_key"`
	PublicKey  types.String `tfsdk:"public_key"`
}

// NewWireguardKeyResource returns the opwan_wireguard_key resource.
func NewWireguardKeyResource() resource.Resource {
	return &wireguardKeyResource{}
}

func (r *wireguardKeyResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_wireguard_key"
}

func (r *wireguardKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A WireGuard key pair generated locally, like `wg genkey`. Nothing is sent to opwan: " +
			"pass `public_key` to `opwan_tunnel`. The private key is kept in the Terraform state (marked sensitive), " +
			"so protect the state the same way you would the key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The public key.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"keepers": schema.MapAttribute{
				MarkdownDescription: "Arbitrary values that, when changed, generate a new key pair. Use it to rotate the key.",
				ElementType:         types.StringType,
				Optional:            true,
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"private_key": schema.StringAttribute{
				MarkdownDescription: "The private key, base64.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "The public key, base64.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *wireguardKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model wireguardKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keyPair, err := generateWireguardKey()
	if err != nil {
		resp.Diagnostics.AddError("Unable to generate a WireGuard key", err.Error())
		return
	}
	model.ID = types.StringValue(keyPair.public)
	model.PrivateKey = types.StringValue(keyPair.private)
	model.PublicKey = types.StringValue(keyPair.public)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *wireguardKeyResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// Update only sees keepers going from null to an empty map or back; every real change replaces.
func (r *wireguardKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model wireguardKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *wireguardKeyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// Curve25519 private key clamping, RFC 7748 section 5.
const (
	CLAMP_FIRST_BYTE_MASK = 0b1111_1000
	CLAMP_LAST_BYTE_MASK  = 0b0111_1111
	CLAMP_LAST_BYTE_SET   = 0b0100_0000
)

type wireguardKeyPair struct {
	private, public string
}

// generateWireguardKey makes a key pair in the form `wg genkey | wg pubkey` prints.
func generateWireguardKey() (wireguardKeyPair, error) {
	scalar := make([]byte, WIREGUARD_KEY_BYTES)
	if _, err := rand.Read(scalar); err != nil {
		return wireguardKeyPair{}, fmt.Errorf("read random bytes: %w", err)
	}
	clampCurve25519(scalar)

	key, err := ecdh.X25519().NewPrivateKey(scalar)
	if err != nil {
		return wireguardKeyPair{}, fmt.Errorf("derive the public key: %w", err)
	}
	return wireguardKeyPair{
		private: base64.StdEncoding.EncodeToString(scalar),
		public:  base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()),
	}, nil
}

func clampCurve25519(scalar []byte) {
	last := len(scalar) - 1
	scalar[0] &= CLAMP_FIRST_BYTE_MASK
	scalar[last] = (scalar[last] & CLAMP_LAST_BYTE_MASK) | CLAMP_LAST_BYTE_SET
}
