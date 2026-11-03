package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// FIRST_TUNNEL_ID names the account's first tunnel where a tunnel id is expected (import, older state).
const FIRST_TUNNEL_ID = "tunnel"

// ATTR_TUNNEL_ID is the attribute naming a tunnel in resources and data sources.
const ATTR_TUNNEL_ID = "tunnel_id"

// TUNNEL_TYPE_WIREGUARD is the only tunnel type opwan_tunnel manages.
const TUNNEL_TYPE_WIREGUARD = string(opwanapi.Wireguard)

var (
	errNoTunnel = errors.New("the account has no tunnel")
)

// TunnelItemModel is one tunnel as the data sources show it.
type TunnelItemModel struct {
	ID                  string              `tfsdk:"id"`
	Number              int64               `tfsdk:"number"`
	Name                string              `tfsdk:"name"`
	Type                string              `tfsdk:"type"`
	Plan                string              `tfsdk:"plan"`
	PeakMbps            int64               `tfsdk:"peak_mbps"`
	Location            string              `tfsdk:"location"`
	State               string              `tfsdk:"state"`
	PublicKey           *string             `tfsdk:"public_key"`
	CreatedAt           string              `tfsdk:"created_at"`
	Addresses           []string            `tfsdk:"addresses"`
	AllowanceBytes      int64               `tfsdk:"allowance_bytes"`
	UsedBytes           int64               `tfsdk:"used_bytes"`
	Server              string              `tfsdk:"server"`
	MTU                 int64               `tfsdk:"mtu"`
	Endpoint            *string             `tfsdk:"endpoint"`
	ServerPublicKey     *string             `tfsdk:"server_public_key"`
	AllowedIPs          []string            `tfsdk:"allowed_ips"`
	PersistentKeepalive *int64              `tfsdk:"persistent_keepalive"`
	IKE                 *string             `tfsdk:"ike"`
	L2TPUsername        *string             `tfsdk:"l2tp_username"`
	IPsecID             *string             `tfsdk:"ipsec_id"`
	IPsecServerID       *string             `tfsdk:"ipsec_server_id"`
	CustomerEndpoint    *string             `tfsdk:"customer_endpoint"`
	BGP                 tunnelBGPBriefModel `tfsdk:"bgp"`
	Status              tunnelStatusModel   `tfsdk:"status"`
}

type tunnelBGPBriefModel struct {
	Enabled bool    `tfsdk:"enabled"`
	ASN     *int64  `tfsdk:"asn"`
	Export  *string `tfsdk:"export"`
	State   *string `tfsdk:"state"`
}

func tunnelItemAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":     computedString("The tunnel id (`tun_...`)."),
		"number": computedInt64("1, 2, ... in the account."),
		"name":   computedString("The tunnel's name, shown in the dashboard."),
		"type":   computedString("`wireguard` or `l2tp_ipsec`."),
		"plan":   computedString("`base` or `plus`."),
		"peak_mbps": computedInt64(
			"The speed ceiling in effect, Mbps.",
		),
		"location": computedString("The tunnel's location id."),
		"state": computedString(
			"`pending_payment`, `setup_required` (a WireGuard tunnel without a key yet), " +
				"`active`, `suspended` or `canceled`.",
		),
		"public_key": computedString("The client's WireGuard public key, null for L2TP/IPsec or before one is set."),
		"created_at": computedString("When the tunnel was created (RFC 3339)."),
		"addresses":  computedStringList("The addresses routed down this tunnel, as /32s."),
		"allowance_bytes": computedInt64(
			"The account's data this month, pooled across its tunnels.",
		),
		"used_bytes": computedInt64("This tunnel's share of the month's data, both directions."),
		"server":     computedString("The host name the router connects to."),
		"mtu":        computedInt64("Suggested client MTU."),
		"endpoint":   computedString("The exit's WireGuard endpoint, `host:port`. Null for L2TP/IPsec."),
		"server_public_key": computedString(
			"The exit's WireGuard public key, null until the exit has reported it, and for L2TP/IPsec.",
		),
		"allowed_ips": computedStringList(
			"Suggested `AllowedIPs` of the peer on the client. Empty for L2TP/IPsec.",
		),
		"persistent_keepalive": computedInt64(
			"Suggested `PersistentKeepalive` in seconds. Null for L2TP/IPsec.",
		),
		"ike":               computedString("`ikev2` or `ikev1` for L2TP/IPsec, else null."),
		"l2tp_username":     computedString("The L2TP user name, null for WireGuard."),
		"ipsec_id":          computedString("The IKEv2 identity the router sends (its local ID)."),
		"ipsec_server_id":   computedString("opwan's IKE identity, the router's remote ID."),
		"customer_endpoint": computedString("The registered fixed public IPv4 (L2TP/IPsec over IKEv1)."),
		"bgp": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Customer BGP over this tunnel, in brief.",
			Attributes: map[string]schema.Attribute{
				"enabled": computedBool("BGP is on."),
				"asn":     computedInt64("The account's AS number, null until the account first turns BGP on."),
				"export":  computedString("`default` or `full`, null while BGP is off."),
				"state": computedString(
					"`established`, `connecting`, `down`, `limit` or `disabled`; " +
						"null while BGP is off or before the first report.",
				),
			},
		},
		"status": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "The tunnel's live state, as the exit last reported it.",
			Attributes:          tunnelStatusAttributes(),
		},
	}
}

func tunnelItemFromAPI(tunnel *opwanapi.TunnelItem) TunnelItemModel {
	model := TunnelItemModel{
		ID:               tunnel.Id,
		Number:           int64(tunnel.Number),
		Name:             tunnel.Name,
		Type:             string(tunnel.Type),
		Plan:             string(tunnel.Plan),
		PeakMbps:         int64(tunnel.PeakMbps),
		Location:         tunnel.Location,
		State:            string(tunnel.State),
		PublicKey:        tunnel.CustomerPubkey,
		CreatedAt:        tunnel.CreatedAt.Format(time.RFC3339),
		Addresses:        nonNil(tunnel.Addresses),
		AllowanceBytes:   tunnel.AllowanceBytes,
		UsedBytes:        tunnel.UsedBytes,
		Server:           tunnel.Setup.Server,
		MTU:              int64(tunnel.Setup.Mtu),
		AllowedIPs:       []string{},
		IKE:              (*string)(tunnel.Ike),
		L2TPUsername:     tunnel.L2tpUsername,
		IPsecID:          tunnel.IpsecId,
		IPsecServerID:    tunnel.Setup.IpsecServerId,
		CustomerEndpoint: tunnel.Endpoint,
		BGP: tunnelBGPBriefModel{
			Enabled: tunnel.Bgp.Enabled,
			ASN:     tunnel.Bgp.Asn,
			Export:  tunnel.Bgp.Export,
			State:   (*string)(tunnel.Bgp.State),
		},
		Status: *tunnelStatusFromAPI(&tunnel.Status),
	}
	if wg := tunnel.Setup.Wireguard; wg != nil {
		keepalive := int64(wg.PersistentKeepalive)
		model.Endpoint = &wg.Endpoint
		model.ServerPublicKey = wg.ServerPubkey
		model.AllowedIPs = nonNil(wg.AllowedIps)
		model.PersistentKeepalive = &keepalive
	}
	return model
}

// readTunnel returns the tunnel with id, or the account's first tunnel for FIRST_TUNNEL_ID or an empty id.
func readTunnel(ctx context.Context, c *client.Client, id string) (*opwanapi.TunnelItem, error) {
	if id != "" && id != FIRST_TUNNEL_ID {
		tunnel, err := c.GetTunnel(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("tunnel %s: %w", id, err)
		}
		return tunnel, nil
	}
	list, err := c.ListTunnels(ctx)
	if err != nil {
		return nil, fmt.Errorf("the first tunnel: %w", err)
	}
	return firstTunnel(list.Tunnels)
}

func firstTunnel(tunnels []opwanapi.TunnelItem) (*opwanapi.TunnelItem, error) {
	var first *opwanapi.TunnelItem
	for i := range tunnels {
		if first == nil || tunnels[i].Number < first.Number {
			first = &tunnels[i]
		}
	}
	if first == nil {
		return nil, errNoTunnel
	}
	return first, nil
}
