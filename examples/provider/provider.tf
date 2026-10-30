terraform {
  required_providers {
    opwan = {
      source = "slop-place/opwan"
    }
  }
}

# The token can also come from OPWAN_API_TOKEN.
provider "opwan" {
  api_token = var.opwan_api_token
}

variable "opwan_api_token" {
  type      = string
  sensitive = true
}
