# The account's first tunnel, created at the location when the account has none.
resource "opwan_tunnel" "home" {
  location   = "dfw"
  public_key = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
}

# A tunnel added in the dashboard, taken over by id.
resource "opwan_tunnel" "office" {
  tunnel_id  = "tun_3f9a1c2b4d5e6f708192"
  name       = "Office"
  public_key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
}
