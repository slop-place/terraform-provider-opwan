# Changelog

## 0.2.0 (2026-11-03)

Support for opwan API 0.16.0: more than one tunnel per account, BGP, address routing and opdns DNS.

### New resources

`opwan_tunnel_bgp`, `opwan_address_assignment`, `opwan_dns`.

### New data sources

`opwan_tunnels`, `opwan_addresses`, `opwan_dns`.

### Changes

- `opwan_tunnel` takes `tunnel_id` to manage any WireGuard tunnel, and `name`. `location` is only needed to create the first tunnel.
- `opwan_tunnel` (resource and data source) `id` is now the tunnel id (`tun_...`), not `tunnel`. Importing `tunnel` still picks the first tunnel.
- `opwan_tunnel` data source takes `tunnel_id` and shows the plan, type, usage, L2TP/IPsec setup and BGP state.
- `opwan_account` addresses show where each is routed and its block.
- `opwan_locations` shows `tunnel_types`; `opwan_plans` shows `max_tunnels` and `block_sizes`.

## 0.1.0 (2026-10-30)

First release.

### Resources

`opwan_tunnel`, `opwan_firewall`, `opwan_wireguard_key`.

### Data sources

`opwan_account`, `opwan_tunnel`, `opwan_firewall`, `opwan_usage`,
`opwan_locations`, `opwan_plans`, `opwan_meta`.
