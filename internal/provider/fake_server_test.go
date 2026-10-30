package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

const (
	TEST_TOKEN      = "opwan_test"
	TEST_LOCATION   = "dfw"
	TEST_ADDRESS    = "72.244.89.10"
	TEST_SERVER_KEY = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
)

// fakeOpwan serves the slice of the opwan API the provider uses, normalising firewall ports like the real one.
type fakeOpwan struct {
	mu        sync.Mutex
	tunnel    *opwanapi.Tunnel
	firewalls map[string]opwanapi.Firewall
}

func newFakeOpwan(t *testing.T) *httptest.Server {
	fake := &fakeOpwan{firewalls: map[string]opwanapi.Firewall{TEST_ADDRESS: defaultFirewall()}}
	server := httptest.NewServer(fake.routes())
	t.Cleanup(server.Close)
	return server
}

func defaultFirewall() opwanapi.Firewall {
	return opwanapi.Firewall{Default: opwanapi.FirewallDefaultAllow, Rules: []opwanapi.FirewallRule{}}
}

func (f *fakeOpwan) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/me", f.authed(f.getMe))
	mux.HandleFunc("PUT /v1/tunnel", f.authed(f.putTunnel))
	mux.HandleFunc("GET /v1/addresses/{ip}/firewall", f.authed(f.getFirewall))
	mux.HandleFunc("PUT /v1/addresses/{ip}/firewall", f.authed(f.putFirewall))
	mux.HandleFunc("GET /v1/usage", f.authed(f.getUsage))
	mux.HandleFunc("GET /v1/locations", f.getLocations)
	mux.HandleFunc("GET /v1/plans", f.getPlans)
	mux.HandleFunc("GET /v1/meta", f.getMeta)
	return mux
}

func (f *fakeOpwan) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+TEST_TOKEN {
			writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in again.")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		next(w, r)
	}
}

func (f *fakeOpwan) getMe(w http.ResponseWriter, _ *http.Request) {
	location := TEST_LOCATION
	writeJSON(w, opwanapi.Me{
		Account:   opwanapi.Account{Email: "you@example.com", Status: "active", CreatedAt: time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC), Location: &location},
		Addresses: []opwanapi.Address{{Ip: TEST_ADDRESS, Location: TEST_LOCATION}},
		Tunnel:    f.tunnel,
		Usage:     opwanapi.MonthUsage{Month: "2026-10", Throttled: "none", AllowanceBytes: 4_000_000_000_000},
	})
}

func (f *fakeOpwan) putTunnel(w http.ResponseWriter, r *http.Request) {
	var req opwanapi.TunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if f.tunnel != nil && f.tunnel.Location != req.Location {
		writeProblem(w, http.StatusConflict, "location_locked", "The tunnel's location can't change.")
		return
	}
	if f.tunnel == nil {
		serverKey := TEST_SERVER_KEY
		f.tunnel = &opwanapi.Tunnel{
			Location:  req.Location,
			State:     "active",
			CreatedAt: time.Date(2026, 10, 29, 18, 5, 0, 0, time.UTC),
			Config: opwanapi.TunnelConfig{
				Location:            req.Location,
				Endpoint:            req.Location + ".wg.opwan.io:51820",
				ServerPubkey:        &serverKey,
				Addresses:           []string{TEST_ADDRESS + "/32"},
				AllowedIps:          []string{"0.0.0.0/0"},
				Mtu:                 1420,
				PersistentKeepalive: 25,
			},
			Status: opwanapi.TunnelStatus{Throttled: "none"},
		}
	}
	f.tunnel.CustomerPubkey = req.Pubkey
	writeJSON(w, f.tunnel)
}

func (f *fakeOpwan) getFirewall(w http.ResponseWriter, r *http.Request) {
	firewall, ok := f.firewalls[r.PathValue("ip")]
	if !ok {
		writeProblem(w, http.StatusNotFound, "not_found", "That address isn't yours.")
		return
	}
	writeJSON(w, firewall)
}

func (f *fakeOpwan) putFirewall(w http.ResponseWriter, r *http.Request) {
	ip := r.PathValue("ip")
	if _, ok := f.firewalls[ip]; !ok {
		writeProblem(w, http.StatusNotFound, "not_found", "That address isn't yours.")
		return
	}
	var firewall opwanapi.Firewall
	if err := json.NewDecoder(r.Body).Decode(&firewall); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	for i, rule := range firewall.Rules {
		if rule.Ports != nil {
			normal := normalisePorts(*rule.Ports)
			firewall.Rules[i].Ports = &normal
		}
	}
	f.firewalls[ip] = firewall
	writeJSON(w, firewall)
}

func normalisePorts(ports string) string {
	ranges, _ := parsePorts(ports)
	items := make([]string, 0, len(ranges))
	for _, r := range mergePorts(ranges) {
		if r.from == r.to {
			items = append(items, fmt.Sprint(r.from))
			continue
		}
		items = append(items, fmt.Sprintf("%d-%d", r.from, r.to))
	}
	return strings.Join(items, ", ")
}

func (f *fakeOpwan) getUsage(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		month = "2026-10"
	}
	writeJSON(w, map[string]any{
		"month": month, "rx_bytes": 52, "tx_bytes": 3, "used_bytes": 55, "allowance_bytes": 4_000_000_000_000,
		"days": []map[string]any{{"date": month + "-01", "rx_bytes": 52, "tx_bytes": 3}},
		"live": nil,
	})
}

func (f *fakeOpwan) getLocations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"locations": []map[string]any{{
		"id": TEST_LOCATION, "name": "Dallas", "city": "Dallas", "country": "US", "network_gbps": 50,
		"test_ip": "72.244.89.1", "ping_host": "ping.dfw.opwan.io", "available": true, "maintenance": nil,
	}}})
}

func (f *fakeOpwan) getPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"currency": "usd",
		"plans": []map[string]any{
			{"id": "base", "name": "Base", "price_cents": 500, "ceil_mbps": 500, "rate_mbps": 10, "slow_mbps": 10, "allowance_bytes": 1_000_000_000_000},
		},
		"add_ons": map[string]any{
			"extra_ip_price_cents": 300, "max_extra_ips": 2, "monthly_tb_price_cents": 200, "max_monthly_tb": 50,
			"extra_tb_price_cents": 200, "max_extra_tb": 20, "peak_plan": "plus",
			"peak_tiers": []map[string]any{{"mbps": 1000, "price_cents": 0}},
		},
	})
}

func (f *fakeOpwan) getMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"version": "v0.1.0", "api_version": "0.9.0", "terms_version": "2026-10-30", "terms_url": "https://opwan.io/terms/"})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://opwan.io/problems/" + code, "title": http.StatusText(status), "status": status,
		"code": code, "detail": detail, "request_id": "test",
	})
}
