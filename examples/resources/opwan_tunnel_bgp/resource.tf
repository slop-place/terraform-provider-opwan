resource "opwan_tunnel_bgp" "office" {
  tunnel_id = opwan_tunnel.office.tunnel_id
  export    = "default"
  auth      = "ao"
}
