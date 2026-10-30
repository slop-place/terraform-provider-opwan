data "opwan_account" "this" {}

resource "opwan_firewall" "web" {
  ip             = data.opwan_account.this.addresses[0].ip
  default_action = "deny"

  rules = [
    { action = "allow", protocol = "tcp", ports = "80, 443" },
    { action = "allow", protocol = "udp", ports = "51000-51100", source = "203.0.113.0/24" },
    { action = "allow", protocol = "icmp" },
  ]
}
