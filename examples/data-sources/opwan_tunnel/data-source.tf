data "opwan_tunnel" "this" {}

output "tunnel_up" {
  value = data.opwan_tunnel.this.status.up
}
