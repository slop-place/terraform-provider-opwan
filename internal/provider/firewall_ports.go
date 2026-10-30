package provider

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// Limits opwan puts on firewall rules.
const (
	MIN_PORT                = 1
	MAX_PORT                = 65535
	MAX_PORT_ITEMS_PER_RULE = 32
	MAX_FIREWALL_RULES      = 64
	BLOCKED_SMTP_PORT       = 25
)

var (
	errTooManyPorts  = errors.New("too many ports or ranges in one rule")
	errReversedRange = errors.New("range starts after it ends")
	errNotAPort      = errors.New("not a port from 1 to 65535")
	errNotIPv4Source = errors.New("not an IPv4 address or CIDR")
)

type portRange struct {
	from, to int
}

// parsePorts reads opwan's port list syntax ("80, 443, 8000-8100").
func parsePorts(ports string) ([]portRange, error) {
	items := strings.Split(ports, ",")
	if len(items) > MAX_PORT_ITEMS_PER_RULE {
		return nil, fmt.Errorf("%w: at most %d, got %d", errTooManyPorts, MAX_PORT_ITEMS_PER_RULE, len(items))
	}

	ranges := make([]portRange, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		fromText, toText, isRange := strings.Cut(item, "-")
		if !isRange {
			toText = fromText
		}
		from, err := parsePort(fromText)
		if err != nil {
			return nil, err
		}
		to, err := parsePort(toText)
		if err != nil {
			return nil, err
		}
		if from > to {
			return nil, fmt.Errorf("%w: %q", errReversedRange, item)
		}
		ranges = append(ranges, portRange{from: from, to: to})
	}
	return ranges, nil
}

func parsePort(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || port < MIN_PORT || port > MAX_PORT {
		return 0, fmt.Errorf("%q is %w", text, errNotAPort)
	}
	return port, nil
}

func (r portRange) contains(port int) bool {
	return r.from <= port && port <= r.to
}

// mergePorts sorts ranges and joins the ones that overlap or touch, so equal port sets compare equal.
func mergePorts(ranges []portRange) []portRange {
	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, func(a, b portRange) int { return a.from - b.from })

	merged := make([]portRange, 0, len(sorted))
	for _, r := range sorted {
		last := len(merged) - 1
		if last >= 0 && r.from <= merged[last].to+1 {
			merged[last].to = max(merged[last].to, r.to)
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

func portsEquivalent(a, b string) bool {
	rangesA, errA := parsePorts(a)
	rangesB, errB := parsePorts(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return slices.Equal(mergePorts(rangesA), mergePorts(rangesB))
}

func parseSource(source string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(source); err == nil && prefix.Addr().Is4() {
		return prefix.Masked(), nil
	}
	if addr, err := netip.ParseAddr(source); err == nil && addr.Is4() {
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	return netip.Prefix{}, fmt.Errorf("%q is %w", source, errNotIPv4Source)
}

func sourcesEquivalent(a, b string) bool {
	prefixA, errA := parseSource(a)
	prefixB, errB := parseSource(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return prefixA == prefixB
}
