resource "opwan_address_assignment" "web" {
  ip        = "72.244.89.11"
  kind      = "tunnel"
  tunnel_id = opwan_tunnel.office.tunnel_id
}

# Floats to whichever BGP tunnel announces it.
resource "opwan_address_assignment" "anycast" {
  ip   = "72.244.89.12"
  kind = "bgp"
}
