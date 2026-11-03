data "opwan_dns" "this" {}

resource "opwan_dns" "this" {
  profile_id = one([for p in data.opwan_dns.this.profiles : p.id if p.name == "Family"])
}
