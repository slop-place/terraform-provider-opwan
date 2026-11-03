package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/slop-place/terraform-provider-opwan/internal/client"
)

const (
	TEST_PUBKEY   = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
	TEST_PUBKEY_2 = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
)

var (
	errFirewallNotReset = errors.New("firewall not reset on destroy")
	errBGPNotOff        = errors.New("BGP not turned off on destroy")
	errDNSNotOff        = errors.New("DNS not turned off on destroy")
)

func testProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"opwan": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func providerConfig(endpoint string) string {
	return fmt.Sprintf(`
provider "opwan" {
  endpoint  = %q
  api_token = %q
}
`, endpoint, TEST_TOKEN)
}

func tunnelConfig(endpoint, location, pubkey string) string {
	return providerConfig(endpoint) + fmt.Sprintf(`
resource "opwan_tunnel" "this" {
  location   = %q
  public_key = %q
}
`, location, pubkey)
}

func TestTunnelResource(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: tunnelConfig(server.URL, TEST_LOCATION, TEST_PUBKEY),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_tunnel.this", "id", TEST_FIRST_TUNNEL),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "tunnel_id", TEST_FIRST_TUNNEL),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "number", "1"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "state", "active"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "endpoint", "dfw.wg.opwan.io:51820"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "server_public_key", TEST_SERVER_KEY),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "addresses.0", TEST_ADDRESS+"/32"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "mtu", "1420"),
				),
			},
			{
				ResourceName:      "opwan_tunnel.this",
				ImportState:       true,
				ImportStateId:     FIRST_TUNNEL_ID,
				ImportStateVerify: true,
			},
			{
				Config: tunnelConfig(server.URL, TEST_LOCATION, TEST_PUBKEY_2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("opwan_tunnel.this", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("opwan_tunnel.this", "public_key", TEST_PUBKEY_2),
			},
			{
				Config:      tunnelConfig(server.URL, "yul", TEST_PUBKEY_2),
				ExpectError: regexp.MustCompile("location can't change"),
			},
		},
	})
}

func adoptedTunnelConfig(endpoint, tunnelID, name, pubkey string) string {
	return providerConfig(endpoint) + fmt.Sprintf(`
resource "opwan_tunnel" "this" {
  tunnel_id  = %q
  name       = %q
  public_key = %q
}
`, tunnelID, name, pubkey)
}

func TestTunnelResourceAdoptsByID(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: adoptedTunnelConfig(server.URL, TEST_SECOND_TUNNEL, "Office", TEST_PUBKEY),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_tunnel.this", "id", TEST_SECOND_TUNNEL),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "location", TEST_LOCATION),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "name", "Office"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "number", "2"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "addresses.0", TEST_ADDRESS_2+"/32"),
				),
			},
			{
				ResourceName:      "opwan_tunnel.this",
				ImportState:       true,
				ImportStateId:     TEST_SECOND_TUNNEL,
				ImportStateVerify: true,
			},
			{
				Config: adoptedTunnelConfig(server.URL, TEST_SECOND_TUNNEL, "Lab", TEST_PUBKEY_2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("opwan_tunnel.this", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_tunnel.this", "name", "Lab"),
					resource.TestCheckResourceAttr("opwan_tunnel.this", "public_key", TEST_PUBKEY_2),
				),
			},
		},
	})
}

func TestTunnelResourceRejectsL2TP(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      adoptedTunnelConfig(server.URL, TEST_L2TP_TUNNEL, "Office", TEST_PUBKEY),
			ExpectError: regexp.MustCompile("Not a WireGuard tunnel"),
		}},
	})
}

func TestTunnelResourceNeedsLocationOrID(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{{
			Config: providerConfig(server.URL) + fmt.Sprintf(`
resource "opwan_tunnel" "this" {
  public_key = %q
}
`, TEST_PUBKEY),
			ExpectError: regexp.MustCompile("Missing location"),
		}},
	})
}

func bgpConfig(endpoint, auth string) string {
	return providerConfig(endpoint) + fmt.Sprintf(`
resource "opwan_tunnel_bgp" "this" {
  tunnel_id = %q
  auth      = %q
}
`, TEST_SECOND_TUNNEL, auth)
}

func TestTunnelBGPResource(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		CheckDestroy:             checkBGPOff(server.URL),
		Steps: []resource.TestStep{
			{
				Config: bgpConfig(server.URL, BGP_AUTH_NONE),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_tunnel_bgp.this", "export", BGP_EXPORT_DEFAULT),
					resource.TestCheckResourceAttr("opwan_tunnel_bgp.this", "ttl_security", "true"),
					resource.TestCheckResourceAttr("opwan_tunnel_bgp.this", "asn", "4237915604"),
					resource.TestCheckResourceAttr("opwan_tunnel_bgp.this", "peer", TEST_BGP_PEER),
					resource.TestCheckNoResourceAttr("opwan_tunnel_bgp.this", "key"),
				),
			},
			{
				Config: bgpConfig(server.URL, BGP_AUTH_AO),
				Check:  resource.TestCheckResourceAttr("opwan_tunnel_bgp.this", "key", TEST_BGP_KEY),
			},
			{
				ResourceName:      "opwan_tunnel_bgp.this",
				ImportState:       true,
				ImportStateId:     TEST_SECOND_TUNNEL,
				ImportStateVerify: true,
			},
		},
	})
}

func assignmentConfig(endpoint, body string) string {
	return providerConfig(endpoint) + fmt.Sprintf(`
resource "opwan_address_assignment" "this" {
  ip = %q
  %s
}
`, TEST_ADDRESS, body)
}

func TestAddressAssignmentResource(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: assignmentConfig(server.URL, fmt.Sprintf(`kind = "tunnel"
  tunnel_id = %q`, TEST_SECOND_TUNNEL)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_address_assignment.this", "kind", ASSIGNMENT_TUNNEL),
					resource.TestCheckResourceAttr("opwan_address_assignment.this", "tunnel_id", TEST_SECOND_TUNNEL),
					resource.TestCheckResourceAttr("opwan_address_assignment.this", "location", TEST_LOCATION),
				),
			},
			{
				ResourceName:      "opwan_address_assignment.this",
				ImportState:       true,
				ImportStateId:     TEST_ADDRESS,
				ImportStateVerify: true,
			},
			{
				Config: assignmentConfig(server.URL, `kind = "bgp"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_address_assignment.this", "kind", ASSIGNMENT_BGP),
					resource.TestCheckNoResourceAttr("opwan_address_assignment.this", "tunnel_id"),
				),
			},
		},
	})
}

func TestAddressAssignmentValidation(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)
	tests := map[string]struct {
		body string
		err  string
	}{
		"tunnel without id": {`kind = "tunnel"`, "Missing tunnel_id"},
		"id without tunnel": {
			fmt.Sprintf(`kind = "unassigned"
  tunnel_id = %q`, TEST_SECOND_TUNNEL),
			"tunnel_id needs kind tunnel",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testProviderFactories(),
				Steps: []resource.TestStep{{
					Config:      assignmentConfig(server.URL, tt.body),
					ExpectError: regexp.MustCompile(tt.err),
				}},
			})
		})
	}
}

func TestDNSResource(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		CheckDestroy:             checkDNSOff(server.URL),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(server.URL) + fmt.Sprintf(`
resource "opwan_dns" "this" {
  profile_id = %q
}
`, TEST_DNS_PROFILE),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_dns.this", "id", DNS_ID),
					resource.TestCheckResourceAttr("opwan_dns.this", "profile_name", TEST_DNS_PROFILE_NM),
					resource.TestCheckResourceAttr("opwan_dns.this", "state", "linked"),
					resource.TestCheckResourceAttr("opwan_dns.this", "linked_ips.0", TEST_ADDRESS),
				),
			},
			{
				ResourceName:      "opwan_dns.this",
				ImportState:       true,
				ImportStateId:     DNS_ID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestTunnelResourceRejectsBadKey(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      tunnelConfig(server.URL, TEST_LOCATION, "not-a-key"),
			ExpectError: regexp.MustCompile("base64 of exactly 32 bytes"),
		}},
	})
}

func firewallConfig(endpoint, rules string) string {
	return providerConfig(endpoint) + fmt.Sprintf(`
resource "opwan_firewall" "this" {
  ip             = %q
  default_action = "deny"
  rules          = [%s]
}
`, TEST_ADDRESS, rules)
}

func TestFirewallResource(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)
	webRules := `
    { action = "allow", protocol = "tcp", ports = "443,80" },
    { action = "allow", protocol = "udp", ports = "51000-51100", source = "203.0.113.0/24" },
    { action = "allow", protocol = "icmp" },`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		CheckDestroy:             checkFirewallReset(server.URL),
		Steps: []resource.TestStep{
			{
				Config: firewallConfig(server.URL, webRules),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_firewall.this", "id", TEST_ADDRESS),
					resource.TestCheckResourceAttr("opwan_firewall.this", "default_action", "deny"),
					resource.TestCheckResourceAttr("opwan_firewall.this", "rules.#", "3"),
					resource.TestCheckResourceAttr("opwan_firewall.this", "rules.0.ports", "443,80"),
					resource.TestCheckNoResourceAttr("opwan_firewall.this", "rules.2.ports"),
				),
			},
			{
				Config: firewallConfig(server.URL, webRules),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "opwan_firewall.this",
				ImportState:       true,
				ImportStateId:     TEST_ADDRESS,
				ImportStateVerify: true,
				// opwan stores ports in normal form, so an import reads "80, 443".
				ImportStateVerifyIgnore: []string{"rules.0.ports"},
			},
			{
				Config: firewallConfig(server.URL, `{ action = "deny", protocol = "any", source = "198.51.100.7" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_firewall.this", "rules.#", "1"),
					resource.TestCheckResourceAttr("opwan_firewall.this", "rules.0.source", "198.51.100.7"),
				),
			},
		},
	})
}

func TestFirewallResourceValidation(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)
	tests := map[string]struct {
		rules string
		err   string
	}{
		"tcp port 25": {
			`{ action = "allow", protocol = "tcp", ports = "1-1024" }`,
			"TCP port 25 is always blocked",
		},
		"ports on icmp": {`{ action = "allow", protocol = "icmp", ports = "80" }`, "Ports need tcp or udp"},
		"bad port":      {`{ action = "allow", protocol = "udp", ports = "70000" }`, "is not a port"},
		"bad source": {
			`{ action = "allow", protocol = "any", source = "example.com" }`,
			"not an IPv4 address or CIDR",
		},
		"unknown protocol": {`{ action = "allow", protocol = "sctp" }`, "value must be one of"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testProviderFactories(),
				Steps: []resource.TestStep{{
					Config:      firewallConfig(server.URL, tt.rules),
					ExpectError: regexp.MustCompile(tt.err),
				}},
			})
		})
	}
}

func TestDataSources(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)
	config := tunnelConfig(server.URL, TEST_LOCATION, TEST_PUBKEY) + fmt.Sprintf(`
data "opwan_account" "this" {}
data "opwan_tunnel" "this" {
  depends_on = [opwan_tunnel.this]
}
data "opwan_firewall" "this" {
  ip = %q
}
data "opwan_usage" "october" {
  month = "2026-10"
}
data "opwan_locations" "all" {}
data "opwan_plans" "all" {}
data "opwan_meta" "this" {}
data "opwan_tunnels" "all" {
  depends_on = [opwan_tunnel.this]
}
data "opwan_tunnel" "l2tp" {
  tunnel_id = %q
}
data "opwan_addresses" "all" {}
data "opwan_dns" "this" {}
`, TEST_ADDRESS, TEST_L2TP_TUNNEL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.opwan_account.this", "email", "you@example.com"),
				resource.TestCheckResourceAttr("data.opwan_account.this", "addresses.0.ip", TEST_ADDRESS),
				resource.TestCheckNoResourceAttr("data.opwan_account.this", "subscription"),
				resource.TestCheckResourceAttr("data.opwan_account.this", "usage.month", "2026-10"),
				resource.TestCheckResourceAttr("data.opwan_tunnel.this", "public_key", TEST_PUBKEY),
				resource.TestCheckResourceAttr("data.opwan_tunnel.this", "status.throttled", "none"),
				resource.TestCheckResourceAttr("data.opwan_firewall.this", "default_action", "allow"),
				resource.TestCheckResourceAttr("data.opwan_firewall.this", "rules.#", "0"),
				resource.TestCheckResourceAttr("data.opwan_usage.october", "days.0.date", "2026-10-01"),
				resource.TestCheckNoResourceAttr("data.opwan_usage.october", "live"),
				resource.TestCheckResourceAttr("data.opwan_locations.all", "locations.0.id", TEST_LOCATION),
				resource.TestCheckNoResourceAttr("data.opwan_locations.all", "locations.0.maintenance"),
				resource.TestCheckResourceAttr("data.opwan_plans.all", "add_ons.peak_tiers.0.mbps", "1000"),
				resource.TestCheckResourceAttr("data.opwan_meta.this", "api_version", "0.16.0"),
				resource.TestCheckResourceAttr("data.opwan_account.this", "addresses.1.tunnel_id", TEST_SECOND_TUNNEL),
				resource.TestCheckResourceAttr("data.opwan_tunnel.this", "id", TEST_FIRST_TUNNEL),
				resource.TestCheckResourceAttr("data.opwan_tunnels.all", "tunnels.#", "3"),
				resource.TestCheckResourceAttr("data.opwan_tunnels.all", "max_tunnels", "8"),
				resource.TestCheckResourceAttr("data.opwan_tunnel.l2tp", "type", "l2tp_ipsec"),
				resource.TestCheckResourceAttr("data.opwan_tunnel.l2tp", "ike", "ikev2"),
				resource.TestCheckNoResourceAttr("data.opwan_tunnel.l2tp", "endpoint"),
				resource.TestCheckResourceAttr("data.opwan_addresses.all", "entitlement", "3"),
				resource.TestCheckResourceAttr("data.opwan_addresses.all", "addresses.0.assignment", "unassigned"),
				resource.TestCheckResourceAttr("data.opwan_dns.this", "profiles.0.id", TEST_DNS_PROFILE),
				resource.TestCheckNoResourceAttr("data.opwan_dns.this", "profile"),
				resource.TestCheckResourceAttr("data.opwan_locations.all", "locations.0.tunnel_types.1", "l2tp_ipsec"),
				resource.TestCheckResourceAttr("data.opwan_plans.all", "add_ons.block_sizes.#", "4"),
			),
		}},
	})
}

func TestUnauthenticated(t *testing.T) {
	t.Parallel()
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "opwan" {
  endpoint  = %q
  api_token = "opwan_wrong"
}
data "opwan_account" "this" {}
`, server.URL),
			ExpectError: regexp.MustCompile("OPWAN_API_TOKEN"),
		}},
	})
}

func checkBGPOff(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(endpoint, TEST_TOKEN, "test")
		if err != nil {
			return fmt.Errorf("create a client: %w", err)
		}
		bgp, err := c.GetTunnelBGP(context.Background(), TEST_SECOND_TUNNEL)
		if err != nil {
			return fmt.Errorf("read the BGP: %w", err)
		}
		if bgp.Enabled {
			return errBGPNotOff
		}
		return nil
	}
}

func checkDNSOff(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(endpoint, TEST_TOKEN, "test")
		if err != nil {
			return fmt.Errorf("create a client: %w", err)
		}
		settings, err := c.GetDNS(context.Background())
		if err != nil {
			return fmt.Errorf("read DNS: %w", err)
		}
		if settings.Profile != nil {
			return errDNSNotOff
		}
		return nil
	}
}

func checkFirewallReset(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(endpoint, TEST_TOKEN, "test")
		if err != nil {
			return fmt.Errorf("create a client: %w", err)
		}
		firewall, err := c.GetFirewall(context.Background(), TEST_ADDRESS)
		if err != nil {
			return fmt.Errorf("read the firewall: %w", err)
		}
		if firewall.Default != DEFAULT_ACTION || len(firewall.Rules) != 0 {
			return fmt.Errorf("%w: %+v", errFirewallNotReset, firewall)
		}
		return nil
	}
}
