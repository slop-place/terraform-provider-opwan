// Package client is the provider's access to the opwan control plane API.
package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

// DEFAULT_ENDPOINT is the production opwan API.
const DEFAULT_ENDPOINT = "https://app.opwan.io"

// Client calls the opwan API with one API token.
type Client struct {
	api *opwanapi.ClientWithResponses
}

// New returns a client for endpoint. An empty token reaches only the public operations.
func New(endpoint, token, userAgent string) (*Client, error) {
	httpClient := &http.Client{Transport: &rateLimitTransport{next: http.DefaultTransport}}

	api, err := opwanapi.NewClientWithResponses(
		endpoint,
		opwanapi.WithHTTPClient(httpClient),
		opwanapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("User-Agent", userAgent)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create the opwan client: %w", err)
	}
	return &Client{api: api}, nil
}

// GetMe returns the account and everything it has.
func (c *Client) GetMe(ctx context.Context) (*opwanapi.Me, error) {
	resp, err := c.api.GetMeWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the account: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// PutTunnel creates the account's tunnel, or replaces its key.
func (c *Client) PutTunnel(ctx context.Context, req opwanapi.TunnelRequest) (*opwanapi.Tunnel, error) {
	resp, err := c.api.PutTunnelWithResponse(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("write the tunnel: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetFirewall returns the inbound firewall of one of the account's addresses.
func (c *Client) GetFirewall(ctx context.Context, ip string) (*opwanapi.Firewall, error) {
	resp, err := c.api.GetFirewallWithResponse(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("read the firewall of %s: %w", ip, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// PutFirewall replaces the inbound firewall of one of the account's addresses.
func (c *Client) PutFirewall(ctx context.Context, ip string, firewall opwanapi.Firewall) (*opwanapi.Firewall, error) {
	resp, err := c.api.PutFirewallWithResponse(ctx, ip, firewall)
	if err != nil {
		return nil, fmt.Errorf("write the firewall of %s: %w", ip, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetUsage returns the data used in month (`YYYY-MM`), nil for the current month.
func (c *Client) GetUsage(ctx context.Context, month *string) (*opwanapi.Usage, error) {
	resp, err := c.api.GetUsageWithResponse(ctx, &opwanapi.GetUsageParams{Month: month})
	if err != nil {
		return nil, fmt.Errorf("read usage: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// ListLocations returns the exit locations.
func (c *Client) ListLocations(ctx context.Context) ([]opwanapi.Location, error) {
	resp, err := c.api.ListLocationsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200.Locations, nil
}

// ListPlans returns the plans and add-ons on sale.
func (c *Client) ListPlans(ctx context.Context) (*opwanapi.Catalogue, error) {
	resp, err := c.api.ListPlansWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetMeta returns the server's version and terms of service.
func (c *Client) GetMeta(ctx context.Context) (*opwanapi.Meta, error) {
	resp, err := c.api.GetMetaWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the server version: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// TunnelList is the account's tunnels and how many it may have.
type TunnelList struct {
	Tunnels    []opwanapi.TunnelItem
	MaxTunnels int
}

// ListTunnels returns the account's tunnels, by number.
func (c *Client) ListTunnels(ctx context.Context) (*TunnelList, error) {
	resp, err := c.api.ListTunnelsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return &TunnelList{Tunnels: resp.JSON200.Tunnels, MaxTunnels: resp.JSON200.MaxTunnels}, nil
}

// GetTunnel returns one of the account's tunnels.
func (c *Client) GetTunnel(ctx context.Context, id string) (*opwanapi.TunnelItem, error) {
	resp, err := c.api.GetTunnelWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// RenameTunnel sets a tunnel's name.
func (c *Client) RenameTunnel(ctx context.Context, id, name string) (*opwanapi.TunnelItem, error) {
	resp, err := c.api.RenameTunnelWithResponse(ctx, id, opwanapi.RenameTunnelJSONRequestBody{Name: name})
	if err != nil {
		return nil, fmt.Errorf("rename tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// PutTunnelKey sets or replaces a WireGuard tunnel's public key.
func (c *Client) PutTunnelKey(ctx context.Context, id, pubkey string) (*opwanapi.TunnelItem, error) {
	resp, err := c.api.PutTunnelKeyWithResponse(ctx, id, opwanapi.PutTunnelKeyJSONRequestBody{Pubkey: pubkey})
	if err != nil {
		return nil, fmt.Errorf("set the key of tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetTunnelBGP returns a tunnel's BGP session.
func (c *Client) GetTunnelBGP(ctx context.Context, id string) (*opwanapi.TunnelBGP, error) {
	resp, err := c.api.GetTunnelBGPWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read the BGP of tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// PutTunnelBGP turns a tunnel's BGP on or off, or changes its settings.
func (c *Client) PutTunnelBGP(
	ctx context.Context,
	id string,
	req opwanapi.TunnelBGPRequest,
) (*opwanapi.TunnelBGP, error) {
	resp, err := c.api.PutTunnelBGPWithResponse(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("write the BGP of tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetTunnelBGPConfig returns the values a router needs for a tunnel's BGP session, its key included.
func (c *Client) GetTunnelBGPConfig(ctx context.Context, id string) (*opwanapi.TunnelBGPConfig, error) {
	resp, err := c.api.GetTunnelBGPConfigWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read the BGP config of tunnel %s: %w", id, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// AddressList is the account's addresses and how many it pays for.
type AddressList struct {
	Addresses   []opwanapi.Address
	Entitlement int
}

// ListAddresses returns the account's addresses, lowest first.
func (c *Client) ListAddresses(ctx context.Context) (*AddressList, error) {
	resp, err := c.api.ListAddressesWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list addresses: %w", err)
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return &AddressList{Addresses: resp.JSON200.Addresses, Entitlement: resp.JSON200.Entitlement}, nil
}

// AssignAddress routes an address down a tunnel, to BGP, or nowhere.
func (c *Client) AssignAddress(
	ctx context.Context,
	ip string,
	req opwanapi.AssignmentRequest,
) (*opwanapi.Address, error) {
	resp, err := c.api.AssignAddressWithResponse(ctx, ip, req)
	if err != nil {
		return nil, fmt.Errorf("assign %s: %w", ip, err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// GetDNS returns the opdns DNS opt-in.
func (c *Client) GetDNS(ctx context.Context) (*opwanapi.DNSSettings, error) {
	resp, err := c.api.GetDNSWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("read DNS: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// PutDNS turns opdns DNS on with profileID.
func (c *Client) PutDNS(ctx context.Context, profileID string) (*opwanapi.DNSSettings, error) {
	resp, err := c.api.PutDNSWithResponse(ctx, opwanapi.PutDNSJSONRequestBody{ProfileId: profileID})
	if err != nil {
		return nil, fmt.Errorf("write DNS: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}

// DeleteDNS turns opdns DNS off.
func (c *Client) DeleteDNS(ctx context.Context) (*opwanapi.DNSSettings, error) {
	resp, err := c.api.DeleteDNSWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("turn DNS off: %w", err)
	}
	return resp.JSON200, checkResponse(resp.HTTPResponse, resp.Body)
}
