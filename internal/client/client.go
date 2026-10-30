// Package client is the provider's access to the opwan control plane API.
package client

import (
	"context"
	"net/http"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

const DEFAULT_ENDPOINT = "https://app.opwan.io"

type Client struct {
	api *opwanapi.ClientWithResponses
}

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
		return nil, err
	}
	return &Client{api: api}, nil
}

func (c *Client) GetMe(ctx context.Context) (*opwanapi.Me, error) {
	resp, err := c.api.GetMeWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) PutTunnel(ctx context.Context, req opwanapi.TunnelRequest) (*opwanapi.Tunnel, error) {
	resp, err := c.api.PutTunnelWithResponse(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) GetFirewall(ctx context.Context, ip string) (*opwanapi.Firewall, error) {
	resp, err := c.api.GetFirewallWithResponse(ctx, ip)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) PutFirewall(ctx context.Context, ip string, firewall opwanapi.Firewall) (*opwanapi.Firewall, error) {
	resp, err := c.api.PutFirewallWithResponse(ctx, ip, firewall)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) GetUsage(ctx context.Context, month *string) (*opwanapi.Usage, error) {
	resp, err := c.api.GetUsageWithResponse(ctx, &opwanapi.GetUsageParams{Month: month})
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) ListLocations(ctx context.Context) ([]opwanapi.Location, error) {
	resp, err := c.api.ListLocationsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200.Locations, nil
}

func (c *Client) ListPlans(ctx context.Context) (*opwanapi.Catalogue, error) {
	resp, err := c.api.ListPlansWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

func (c *Client) GetMeta(ctx context.Context) (*opwanapi.Meta, error) {
	resp, err := c.api.GetMetaWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
