package provider

import "testing"

func TestPortsEquivalent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want bool
	}{
		{"443", "443", true},
		{"443,80", "80, 443", true},
		{"80, 80, 443", "80, 443", true},
		{"8000-8100, 8050-8200", "8000-8200", true},
		{"1-24, 26-100", "1-24, 26-100", true},
		{"10-20, 21-30", "10-30", true},
		{"80", "443", false},
		{"1-24", "1-25", false},
	}
	for _, tt := range tests {
		if got := portsEquivalent(tt.a, tt.b); got != tt.want {
			t.Errorf("portsEquivalent(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParsePortsRejects(t *testing.T) {
	t.Parallel()
	for _, ports := range []string{"", "0", "65536", "abc", "100-90", "1-2-3"} {
		if _, err := parsePorts(ports); err == nil {
			t.Errorf("parsePorts(%q) accepted an invalid list", ports)
		}
	}
}

func TestSourcesEquivalent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want bool
	}{
		{"203.0.113.5", "203.0.113.5/32", true},
		{"203.0.113.7/24", "203.0.113.0/24", true},
		{"203.0.113.0/24", "203.0.113.0/25", false},
		{"2001:db8::1", "2001:db8::1", true},
	}
	for _, tt := range tests {
		if got := sourcesEquivalent(tt.a, tt.b); got != tt.want {
			t.Errorf("sourcesEquivalent(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
