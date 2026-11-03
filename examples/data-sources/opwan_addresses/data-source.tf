data "opwan_addresses" "all" {}

output "unassigned" {
  value = [for a in data.opwan_addresses.all.addresses : a.ip if a.assignment == "unassigned"]
}
