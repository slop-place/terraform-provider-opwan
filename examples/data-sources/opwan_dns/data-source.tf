data "opwan_dns" "this" {}

output "dns_profiles" {
  value = data.opwan_dns.this.profiles
}
