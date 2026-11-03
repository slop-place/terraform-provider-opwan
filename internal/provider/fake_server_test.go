package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

const (
	TEST_TOKEN          = "opwan_test"
	TEST_LOCATION       = "dfw"
	TEST_ADDRESS        = "72.244.89.10"
	TEST_ADDRESS_2      = "72.244.89.11"
	TEST_SERVER_KEY     = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
	TEST_FIRST_TUNNEL   = "tun_first"
	TEST_SECOND_TUNNEL  = "tun_second"
	TEST_L2TP_TUNNEL    = "tun_l2tp"
	TEST_DNS_PROFILE    = "a1b2c3"
	TEST_BGP_KEY        = "bgp-secret"
	TEST_ASN            = int64(4237915604)
	TEST_OUR_ASN        = 403104
	TEST_RESOLVER       = "23.166.164.53"
	TEST_BGP_LOCAL      = "169.254.64.2"
	TEST_BGP_PEER       = "169.254.64.3"
	TEST_BGP_HOLD_TIME  = 9
	TEST_MTU            = 1420
	TEST_KEEPALIVE      = 25
	TEST_ALLOWANCE      = 4_000_000_000_000
	TEST_FIRST_NUMBER   = 1
	TEST_SECOND_NUMBER  = 2
	TEST_L2TP_NUMBER    = 3
	TEST_PEAK_MBPS      = 1000
	TEST_MAX_TUNNELS    = 8
	TEST_ENTITLEMENT    = 3
	TEST_L2TP_SERVER    = "dfw.l2tp.opwan.io"
	TEST_DNS_PROFILE_NM = "Family"
)

// fakeOpwan serves the slice of the opwan API the provider uses, normalising firewall ports like the real one.
type fakeOpwan struct {
	mu        sync.Mutex
	tunnels   []*opwanapi.TunnelItem
	bgp       map[string]*opwanapi.TunnelBGP
	addresses []opwanapi.Address
	firewalls map[string]opwanapi.Firewall
	dns       opwanapi.DNSSettings
}

func newFakeOpwan(t *testing.T) *httptest.Server {
	t.Helper()
	l2tpIke := opwanapi.TunnelItemIke("ikev2")
	fake := &fakeOpwan{
		tunnels: []*opwanapi.TunnelItem{
			newFakeTunnel(TEST_SECOND_TUNNEL, TEST_SECOND_NUMBER, opwanapi.Wireguard, TEST_ADDRESS_2),
			newFakeTunnel(TEST_L2TP_TUNNEL, TEST_L2TP_NUMBER, opwanapi.L2tpIpsec, ""),
		},
		bgp: map[string]*opwanapi.TunnelBGP{},
		addresses: []opwanapi.Address{
			fakeAddress(TEST_ADDRESS, opwanapi.AssignmentKindUnassigned, nil),
			fakeAddress(TEST_ADDRESS_2, opwanapi.AssignmentKindTunnel, new(TEST_SECOND_TUNNEL)),
		},
		firewalls: map[string]opwanapi.Firewall{TEST_ADDRESS: defaultFirewall()},
		dns: opwanapi.DNSSettings{
			Available:    true,
			OpdnsAccount: new(true),
			Profiles:     []opwanapi.DNSProfile{{Id: TEST_DNS_PROFILE, Name: TEST_DNS_PROFILE_NM}},
			Resolver:     TEST_RESOLVER,
			State:        opwanapi.Off,
			LinkedIps:    []string{},
		},
	}
	fake.tunnels[1].Ike = &l2tpIke
	server := httptest.NewServer(fake.routes())
	t.Cleanup(server.Close)
	return server
}

func newFakeTunnel(id string, number int, tunnelType opwanapi.TunnelType, address string) *opwanapi.TunnelItem {
	addresses := []string{}
	if address != "" {
		addresses = append(addresses, address+"/32")
	}
	tunnel := &opwanapi.TunnelItem{
		Id:             id,
		Number:         number,
		Name:           fmt.Sprintf("Tunnel %d", number),
		Type:           tunnelType,
		Location:       TEST_LOCATION,
		State:          opwanapi.TunnelItemStateActive,
		Plan:           "plus",
		PeakMbps:       TEST_PEAK_MBPS,
		AllowanceBytes: TEST_ALLOWANCE,
		Addresses:      addresses,
		CreatedAt:      time.Date(2026, 10, 29, 18, 5, 0, 0, time.UTC),
		Setup:          opwanapi.TunnelSetup{Server: TEST_LOCATION + ".wg.opwan.io", Mtu: TEST_MTU},
		Status:         opwanapi.TunnelStatus{Throttled: "none"},
	}
	if tunnelType == opwanapi.Wireguard {
		tunnel.Setup.Wireguard = &opwanapi.TunnelConfig{
			Location:            TEST_LOCATION,
			Endpoint:            TEST_LOCATION + ".wg.opwan.io:51820",
			ServerPubkey:        new(TEST_SERVER_KEY),
			Addresses:           addresses,
			AllowedIps:          []string{"0.0.0.0/0"},
			Mtu:                 TEST_MTU,
			PersistentKeepalive: TEST_KEEPALIVE,
		}
		return tunnel
	}
	tunnel.Setup.Server = TEST_L2TP_SERVER
	tunnel.L2tpUsername = new("opwan-user")
	return tunnel
}

func fakeAddress(ip string, kind opwanapi.AssignmentKind, tunnel *string) opwanapi.Address {
	return opwanapi.Address{
		Ip:         ip,
		Location:   TEST_LOCATION,
		Assignment: opwanapi.Assignment{Kind: kind, Tunnel: tunnel},
	}
}

func defaultFirewall() opwanapi.Firewall {
	return opwanapi.Firewall{Default: opwanapi.FirewallDefaultAllow, Rules: []opwanapi.FirewallRule{}}
}

func (f *fakeOpwan) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/me", f.authed(f.getMe))
	mux.HandleFunc("PUT /v1/tunnel", f.authed(f.putTunnel))
	mux.HandleFunc("GET /v1/tunnels", f.authed(f.listTunnels))
	mux.HandleFunc("GET /v1/tunnels/{id}", f.authed(f.withTunnel(f.getTunnel)))
	mux.HandleFunc("PATCH /v1/tunnels/{id}", f.authed(f.withTunnel(f.renameTunnel)))
	mux.HandleFunc("PUT /v1/tunnels/{id}/key", f.authed(f.withTunnel(f.putTunnelKey)))
	mux.HandleFunc("GET /v1/tunnels/{id}/bgp", f.authed(f.withTunnel(f.getBGP)))
	mux.HandleFunc("PUT /v1/tunnels/{id}/bgp", f.authed(f.withTunnel(f.putBGP)))
	mux.HandleFunc("GET /v1/tunnels/{id}/bgp/config", f.authed(f.withTunnel(f.getBGPConfig)))
	mux.HandleFunc("GET /v1/addresses", f.authed(f.listAddresses))
	mux.HandleFunc("PUT /v1/addresses/{ip}/assignment", f.authed(f.assignAddress))
	mux.HandleFunc("GET /v1/addresses/{ip}/firewall", f.authed(f.getFirewall))
	mux.HandleFunc("PUT /v1/addresses/{ip}/firewall", f.authed(f.putFirewall))
	mux.HandleFunc("GET /v1/dns", f.authed(f.getDNS))
	mux.HandleFunc("PUT /v1/dns", f.authed(f.putDNS))
	mux.HandleFunc("DELETE /v1/dns", f.authed(f.deleteDNS))
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

func (f *fakeOpwan) withTunnel(
	next func(http.ResponseWriter, *http.Request, *opwanapi.TunnelItem),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tunnel := f.tunnel(r.PathValue("id"))
		if tunnel == nil {
			writeProblem(w, http.StatusNotFound, "not_found", "No such tunnel.")
			return
		}
		next(w, r, tunnel)
	}
}

func (f *fakeOpwan) tunnel(id string) *opwanapi.TunnelItem {
	for _, tunnel := range f.tunnels {
		if tunnel.Id == id {
			return tunnel
		}
	}
	return nil
}

func (f *fakeOpwan) first() *opwanapi.TunnelItem {
	for _, tunnel := range f.tunnels {
		if tunnel.Number == TEST_FIRST_NUMBER {
			return tunnel
		}
	}
	return nil
}

func (f *fakeOpwan) getMe(w http.ResponseWriter, _ *http.Request) {
	tunnels := make([]opwanapi.TunnelItem, len(f.tunnels))
	for i, tunnel := range f.tunnels {
		tunnels[i] = *tunnel
	}
	writeJSON(w, opwanapi.Me{
		Account: opwanapi.Account{
			Email:     "you@example.com",
			Status:    "active",
			CreatedAt: time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC),
			Location:  new(TEST_LOCATION),
		},
		Addresses: f.addresses,
		Tunnel:    legacyTunnel(f.first()),
		Tunnels:   tunnels,
		Usage:     opwanapi.MonthUsage{Month: "2026-10", Throttled: "none", AllowanceBytes: TEST_ALLOWANCE},
	})
}

func legacyTunnel(tunnel *opwanapi.TunnelItem) *opwanapi.Tunnel {
	if tunnel == nil {
		return nil
	}
	return &opwanapi.Tunnel{
		Location:       tunnel.Location,
		State:          opwanapi.TunnelState(tunnel.State),
		CustomerPubkey: *tunnel.CustomerPubkey,
		CreatedAt:      tunnel.CreatedAt,
		Config:         *tunnel.Setup.Wireguard,
		Status:         tunnel.Status,
	}
}

func (f *fakeOpwan) putTunnel(w http.ResponseWriter, r *http.Request) {
	var req opwanapi.TunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	tunnel := f.first()
	if tunnel != nil && tunnel.Location != req.Location {
		writeProblem(w, http.StatusConflict, "location_locked", "The tunnel's location can't change.")
		return
	}
	if tunnel == nil {
		tunnel = newFakeTunnel(TEST_FIRST_TUNNEL, TEST_FIRST_NUMBER, opwanapi.Wireguard, TEST_ADDRESS)
		tunnel.Location = req.Location
		f.tunnels = append([]*opwanapi.TunnelItem{tunnel}, f.tunnels...)
	}
	tunnel.CustomerPubkey = &req.Pubkey
	writeJSON(w, legacyTunnel(tunnel))
}

func (f *fakeOpwan) listTunnels(w http.ResponseWriter, _ *http.Request) {
	tunnels := make([]opwanapi.TunnelItem, len(f.tunnels))
	for i, tunnel := range f.tunnels {
		tunnels[i] = *tunnel
	}
	writeJSON(w, map[string]any{"tunnels": tunnels, "max_tunnels": TEST_MAX_TUNNELS})
}

func (f *fakeOpwan) getTunnel(w http.ResponseWriter, _ *http.Request, tunnel *opwanapi.TunnelItem) {
	writeJSON(w, tunnel)
}

func (f *fakeOpwan) renameTunnel(w http.ResponseWriter, r *http.Request, tunnel *opwanapi.TunnelItem) {
	var req opwanapi.RenameTunnelJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	tunnel.Name = req.Name
	writeJSON(w, tunnel)
}

func (f *fakeOpwan) putTunnelKey(w http.ResponseWriter, r *http.Request, tunnel *opwanapi.TunnelItem) {
	if tunnel.Type != opwanapi.Wireguard {
		writeProblem(w, http.StatusConflict, "not_wireguard", "Not a WireGuard tunnel.")
		return
	}
	var req opwanapi.PutTunnelKeyJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	tunnel.CustomerPubkey = &req.Pubkey
	writeJSON(w, tunnel)
}

func (f *fakeOpwan) bgpOf(tunnel *opwanapi.TunnelItem) *opwanapi.TunnelBGP {
	bgp, ok := f.bgp[tunnel.Id]
	if !ok {
		bgp = &opwanapi.TunnelBGP{
			Auth:     opwanapi.TunnelBGPAuthNone,
			OurAsn:   TEST_OUR_ASN,
			HoldTime: TEST_BGP_HOLD_TIME,
			Accepted: []string{},
		}
		f.bgp[tunnel.Id] = bgp
	}
	return bgp
}

func (f *fakeOpwan) getBGP(w http.ResponseWriter, _ *http.Request, tunnel *opwanapi.TunnelItem) {
	writeJSON(w, f.bgpOf(tunnel))
}

func (f *fakeOpwan) putBGP(w http.ResponseWriter, r *http.Request, tunnel *opwanapi.TunnelItem) {
	var req opwanapi.TunnelBGPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	bgp := f.bgpOf(tunnel)
	bgp.Enabled = req.Enabled
	tunnel.Bgp.Enabled = req.Enabled
	if !req.Enabled {
		bgp.Export = nil
		writeJSON(w, bgp)
		return
	}
	bgp.Asn = new(TEST_ASN)
	bgp.Local = new(TEST_BGP_LOCAL)
	bgp.Peer = new(TEST_BGP_PEER)
	bgp.Export = new(opwanapi.TunnelBGPExport(*req.Export))
	bgp.Auth = opwanapi.TunnelBGPAuth(*req.Auth)
	bgp.TtlSecurity = *req.TtlSecurity
	writeJSON(w, bgp)
}

func (f *fakeOpwan) getBGPConfig(w http.ResponseWriter, _ *http.Request, tunnel *opwanapi.TunnelItem) {
	bgp := f.bgpOf(tunnel)
	config := opwanapi.TunnelBGPConfig{Auth: opwanapi.TunnelBGPConfigAuth(bgp.Auth), Announce: []string{}}
	if bgp.Auth != opwanapi.TunnelBGPAuthNone {
		config.Key = new(TEST_BGP_KEY)
	}
	writeJSON(w, config)
}

func (f *fakeOpwan) listAddresses(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"addresses": f.addresses, "entitlement": TEST_ENTITLEMENT})
}

func (f *fakeOpwan) assignAddress(w http.ResponseWriter, r *http.Request) {
	var req opwanapi.AssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	for i := range f.addresses {
		address := &f.addresses[i]
		if address.Ip != r.PathValue("ip") {
			continue
		}
		address.Assignment = opwanapi.Assignment{Kind: opwanapi.AssignmentKindTunnel, Tunnel: req.Tunnel}
		if req.Kind != nil {
			address.Assignment = opwanapi.Assignment{Kind: opwanapi.AssignmentKind(*req.Kind)}
		}
		writeJSON(w, address)
		return
	}
	writeProblem(w, http.StatusNotFound, "not_found", "That address isn't yours.")
}

func (f *fakeOpwan) getDNS(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, f.dns)
}

func (f *fakeOpwan) putDNS(w http.ResponseWriter, r *http.Request) {
	var req opwanapi.PutDNSJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	for _, profile := range f.dns.Profiles {
		if profile.Id == req.ProfileId {
			f.dns.Profile = &profile
			f.dns.State = opwanapi.Linked
			f.dns.LinkedIps = []string{TEST_ADDRESS}
			writeJSON(w, f.dns)
			return
		}
	}
	writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Not one of your profiles.")
}

func (f *fakeOpwan) deleteDNS(w http.ResponseWriter, _ *http.Request) {
	f.dns.Profile = nil
	f.dns.State = opwanapi.Off
	f.dns.LinkedIps = []string{}
	writeJSON(w, f.dns)
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
			items = append(items, strconv.Itoa(r.from))
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
		"month": month, "rx_bytes": 52, "tx_bytes": 3, "used_bytes": 55, "allowance_bytes": TEST_ALLOWANCE,
		"days": []map[string]any{{"date": month + "-01", "rx_bytes": 52, "tx_bytes": 3}},
		"live": nil,
	})
}

func (f *fakeOpwan) getLocations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"locations": []map[string]any{{
		"id": TEST_LOCATION, "name": "Dallas", "city": "Dallas", "country": "US", "network_gbps": 50,
		"test_ip": "72.244.89.1", "ping_host": "ping.dfw.opwan.io", "available": true, "maintenance": nil,
		"tunnel_types": []string{"wireguard", "l2tp_ipsec"},
	}}})
}

func (f *fakeOpwan) getPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"currency": "usd",
		"plans": []map[string]any{
			{
				"id":              "base",
				"name":            "Base",
				"price_cents":     500,
				"ceil_mbps":       500,
				"rate_mbps":       10,
				"slow_mbps":       10,
				"allowance_bytes": 1_000_000_000_000,
			},
		},
		"add_ons": map[string]any{
			"extra_ip_price_cents": 300, "max_extra_ips": 7, "monthly_tb_price_cents": 200, "max_monthly_tb": 50,
			"extra_tb_price_cents": 200, "max_extra_tb": 20, "peak_plan": "plus",
			"peak_tiers":  []map[string]any{{"mbps": 1000, "price_cents": 0}},
			"max_tunnels": TEST_MAX_TUNNELS, "block_sizes": []int{1, 2, 4, 8},
		},
	})
}

func (f *fakeOpwan) getMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(
		w,
		map[string]any{
			"version":       "v0.1.0",
			"api_version":   "0.16.0",
			"terms_version": "2026-10-30",
			"terms_url":     "https://opwan.io/terms/",
		},
	)
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	encode(w, body)
}

func encode(w http.ResponseWriter, body any) {
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	encode(w, map[string]any{
		"type": "https://opwan.io/problems/" + code, "title": http.StatusText(status), "status": status,
		"code": code, "detail": detail, "request_id": "test",
	})
}
