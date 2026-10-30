data "opwan_locations" "all" {}

output "available_locations" {
  value = [for l in data.opwan_locations.all.locations : l.id if l.available]
}
