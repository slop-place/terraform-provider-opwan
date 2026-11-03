data "opwan_tunnels" "all" {}

output "tunnel_ids" {
  value = { for t in data.opwan_tunnels.all.tunnels : t.name => t.id }
}
