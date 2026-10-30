package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// WIREGUARD_KEY_BYTES is the length of a WireGuard key before base64.
const WIREGUARD_KEY_BYTES = 32

var (
	errNotIPv4         = errors.New("not an IPv4 address")
	errBadWireguardKey = errors.New("a WireGuard public key is base64 of exactly 32 bytes")
)

// stringCheck adapts a plain check function to a schema string validator.
type stringCheck struct {
	description string
	check       func(string) error
}

func (v stringCheck) Description(_ context.Context) string         { return v.description }
func (v stringCheck) MarkdownDescription(_ context.Context) string { return v.description }

func (v stringCheck) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := v.check(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", err.Error())
	}
}

func ipv4Address() validator.String {
	return stringCheck{
		description: "must be an IPv4 address",
		check: func(value string) error {
			addr, err := netip.ParseAddr(value)
			if err != nil || !addr.Is4() {
				return fmt.Errorf("%q is %w", value, errNotIPv4)
			}
			return nil
		},
	}
}

func portList() validator.String {
	return stringCheck{
		description: fmt.Sprintf("must be ports and ranges separated by commas, %d to %d", MIN_PORT, MAX_PORT),
		check: func(value string) error {
			_, err := parsePorts(value)
			return err
		},
	}
}

func ipv4Source() validator.String {
	return stringCheck{
		description: "must be an IPv4 address or CIDR",
		check: func(value string) error {
			_, err := parseSource(value)
			return err
		},
	}
}

func wireguardPublicKey() validator.String {
	return stringCheck{description: "must be a base64 WireGuard public key", check: checkWireguardKey}
}

func checkWireguardKey(value string) error {
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != WIREGUARD_KEY_BYTES {
		return errBadWireguardKey
	}
	return nil
}
