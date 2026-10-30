package provider

import (
	"context"
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

var testProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"opwan": providerserver.NewProtocol6WithError(New("test")()),
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
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: tunnelConfig(server.URL, TEST_LOCATION, TEST_PUBKEY),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opwan_tunnel.this", "id", TUNNEL_ID),
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
				ImportStateId:     TUNNEL_ID,
				ImportStateVerify: true,
			},
			{
				Config: tunnelConfig(server.URL, TEST_LOCATION, TEST_PUBKEY_2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("opwan_tunnel.this", plancheck.ResourceActionUpdate)},
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

func TestTunnelResourceRejectsBadKey(t *testing.T) {
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
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
	server := newFakeOpwan(t)
	webRules := `
    { action = "allow", protocol = "tcp", ports = "443,80" },
    { action = "allow", protocol = "udp", ports = "51000-51100", source = "203.0.113.0/24" },
    { action = "allow", protocol = "icmp" },`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
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
	server := newFakeOpwan(t)
	tests := map[string]struct {
		rules string
		err   string
	}{
		"tcp port 25":      {`{ action = "allow", protocol = "tcp", ports = "1-1024" }`, "TCP port 25 is always blocked"},
		"ports on icmp":    {`{ action = "allow", protocol = "icmp", ports = "80" }`, "Ports need tcp or udp"},
		"bad port":         {`{ action = "allow", protocol = "udp", ports = "70000" }`, "is not a port"},
		"bad source":       {`{ action = "allow", protocol = "any", source = "example.com" }`, "not an IPv4 address or CIDR"},
		"unknown protocol": {`{ action = "allow", protocol = "sctp" }`, "value must be one of"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testProviderFactories,
				Steps: []resource.TestStep{{
					Config:      firewallConfig(server.URL, tt.rules),
					ExpectError: regexp.MustCompile(tt.err),
				}},
			})
		})
	}
}

func TestDataSources(t *testing.T) {
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
`, TEST_ADDRESS)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
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
				resource.TestCheckResourceAttr("data.opwan_meta.this", "api_version", "0.9.0"),
			),
		}},
	})
}

func TestUnauthenticated(t *testing.T) {
	server := newFakeOpwan(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
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

func checkFirewallReset(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(endpoint, TEST_TOKEN, "test")
		if err != nil {
			return err
		}
		firewall, err := c.GetFirewall(context.Background(), TEST_ADDRESS)
		if err != nil {
			return err
		}
		if firewall.Default != DEFAULT_ACTION || len(firewall.Rules) != 0 {
			return fmt.Errorf("firewall not reset on destroy: %+v", firewall)
		}
		return nil
	}
}
