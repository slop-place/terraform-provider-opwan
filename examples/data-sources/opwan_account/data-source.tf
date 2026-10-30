data "opwan_account" "this" {}

output "addresses" {
  value = [for a in data.opwan_account.this.addresses : a.ip]
}
