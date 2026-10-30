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
