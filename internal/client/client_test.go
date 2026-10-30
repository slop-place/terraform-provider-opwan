package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRetriesRateLimitedRequests(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(`{"version":"v1","api_version":"0.9.0","terms_version":"x","terms_url":"https://opwan.io/terms/"}`),
		)
	}))
	defer server.Close()

	c, err := New(server.URL, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := c.GetMeta(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ApiVersion != "0.9.0" || calls.Load() != 2 {
		t.Fatalf("got api_version %q after %d calls", meta.ApiVersion, calls.Load())
	}
}

func TestProblemErrors(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"type":"x","title":"x","status":422,"code":"validation_failed","detail":"Check the rules.",
			"errors":[{"field":"rules[2].ports","message":"port 25 is blocked"}],"request_id":"req1"}`))
	}))
	defer server.Close()

	c, _ := New(server.URL, "opwan_x", "test")
	_, err := c.GetFirewall(t.Context(), "72.244.89.10")
	for _, want := range []string{"Check the rules.", "validation_failed", "rules[2].ports: port 25 is blocked", "req1"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v does not mention %q", err, want)
		}
	}
	if !HasCode(err, "validation_failed") {
		t.Error("HasCode missed the problem code")
	}
}
