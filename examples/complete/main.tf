# A tunnel, a locked-down firewall on every address, and a wg-quick config file.
# The private key stays local: only its public half goes to opwan.

terraform {
  required_providers {
    opwan = {
      source = "slop-place/opwan"
    }
    local = {
      source = "hashicorp/local"
    }
  }
}

provider "opwan" {}

variable "location" {
  type    = string
  default = "dfw"
}

variable "public_key" {
  type        = string
  description = "Output of `wg pubkey < private.key`."
}

variable "private_key_path" {
  type    = string
  default = "private.key"
}

resource "opwan_tunnel" "this" {
  location   = var.location
  public_key = var.public_key
}

data "opwan_account" "this" {}

resource "opwan_firewall" "this" {
  for_each = toset([for a in data.opwan_account.this.addresses : a.ip])

  ip             = each.value
  default_action = "deny"
  rules = [
    { action = "allow", protocol = "tcp", ports = "22, 80, 443" },
    { action = "allow", protocol = "icmp" },
  ]
}

resource "local_sensitive_file" "wg_quick" {
  filename        = "opwan.conf"
  file_permission = "0600"
  content         = <<-EOT
    [Interface]
    PrivateKey = ${trimspace(file(var.private_key_path))}
    Address = ${join(", ", opwan_tunnel.this.addresses)}
    MTU = ${opwan_tunnel.this.mtu}

    [Peer]
    PublicKey = ${opwan_tunnel.this.server_public_key}
    Endpoint = ${opwan_tunnel.this.endpoint}
    AllowedIPs = ${join(", ", opwan_tunnel.this.allowed_ips)}
    PersistentKeepalive = ${opwan_tunnel.this.persistent_keepalive}
  EOT
}
