data "opwan_usage" "october" {
  month = "2026-10"
}

output "used_percent" {
  value = 100 * data.opwan_usage.october.used_bytes / data.opwan_usage.october.allowance_bytes
}
