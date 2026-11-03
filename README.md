# terraform-provider-opwan

Terraform provider for [opwan](https://opwan.io): a static public IPv4 address over WireGuard.

## What it manages

| Type | Name | API |
| --- | --- | --- |
| Resource | `opwan_tunnel` | `PUT /v1/tunnel`, `GET`/`PATCH /v1/tunnels/{id}`, `PUT /v1/tunnels/{id}/key` |
| Resource | `opwan_tunnel_bgp` | `GET`/`PUT /v1/tunnels/{id}/bgp`, `GET /v1/tunnels/{id}/bgp/config` |
| Resource | `opwan_address_assignment` | `GET /v1/addresses`, `PUT /v1/addresses/{ip}/assignment` |
| Resource | `opwan_firewall` | `GET`/`PUT /v1/addresses/{ip}/firewall` |
| Resource | `opwan_dns` | `GET`/`PUT`/`DELETE /v1/dns` |
| Data source | `opwan_account` | `GET /v1/me` |
| Data source | `opwan_tunnel` | `GET /v1/tunnels`, `GET /v1/tunnels/{id}` |
| Data source | `opwan_tunnels` | `GET /v1/tunnels` |
| Data source | `opwan_addresses` | `GET /v1/addresses` |
| Data source | `opwan_dns` | `GET /v1/dns` |
| Data source | `opwan_firewall` | `GET /v1/addresses/{ip}/firewall` |
| Data source | `opwan_usage` | `GET /v1/usage` |
| Data source | `opwan_locations` | `GET /v1/locations` (no token) |
| Data source | `opwan_plans` | `GET /v1/plans` (no token) |
| Data source | `opwan_meta` | `GET /v1/meta` (no token) |

Checkout, subscription and tunnel plan changes, adding or deleting tunnels, buying or releasing addresses, billing and API tokens are session-only in the opwan API, so an API token can't manage them.

Docs: [`docs/`](docs). Full example: [`examples/complete`](examples/complete).

## Usage

```hcl
provider "opwan" {} # reads OPWAN_API_TOKEN

resource "opwan_tunnel" "home" {
  location   = "dfw"
  public_key = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
}
```

## Development

Tools come from `mise`.

```sh
mise install
mise run test          # unit tests and Terraform runs against a fake opwan server
mise run lint          # go vet and golangci-lint
mise run fetch-spec    # pull the latest api/openapi.json
mise run generate      # regenerate internal/opwanapi and docs/
mise run install       # build into GOBIN
```

The API client in `internal/opwanapi` is generated from `api/openapi.json` with oapi-codegen. Don't edit it by hand.

To try a local build, point Terraform at it with a `dev_overrides` block for `registry.terraform.io/slop-place/opwan` in `~/.terraformrc`.
