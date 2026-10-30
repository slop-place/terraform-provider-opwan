package provider

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// RFC 7748 section 6.1, Alice's key pair.
const (
	RFC7748_PRIVATE = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	RFC7748_PUBLIC  = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"
)

func TestClampedKeyKeepsPublicKey(t *testing.T) {
	scalar, _ := hex.DecodeString(RFC7748_PRIVATE)
	clampCurve25519(scalar)
	key, err := ecdh.X25519().NewPrivateKey(scalar)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(key.PublicKey().Bytes()); got != RFC7748_PUBLIC {
		t.Fatalf("public key %s, want %s", got, RFC7748_PUBLIC)
	}
}

func TestGenerateWireguardKey(t *testing.T) {
	privateKey, publicKey, err := generateWireguardKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{privateKey, publicKey} {
		if err := wireguardPublicKey().(stringCheck).check(key); err != nil {
			t.Errorf("%q: %v", key, err)
		}
	}
	scalar, _ := base64.StdEncoding.DecodeString(privateKey)
	if scalar[0]&7 != 0 || scalar[31]&128 != 0 || scalar[31]&64 == 0 {
		t.Errorf("private key is not clamped like wg genkey: %x", scalar)
	}
}

func wireguardKeyConfig(rotation string) string {
	return `
resource "opwan_wireguard_key" "this" {
  keepers = { rotation = "` + rotation + `" }
}
`
}

func TestWireguardKeyResource(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: wireguardKeyConfig("1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("opwan_wireguard_key.this", "private_key"),
					resource.TestCheckResourceAttrPair("opwan_wireguard_key.this", "id", "opwan_wireguard_key.this", "public_key"),
				),
			},
			{
				Config:           wireguardKeyConfig("1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config: wireguardKeyConfig("2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("opwan_wireguard_key.this", plancheck.ResourceActionReplace)},
				},
			},
		},
	})
}
