package upstream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyProbeReportsExitLocationAndFailure(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "geo.example.test" {
			t.Errorf("wrong probe target: %s", r.URL.Host)
		}
		fmt.Fprint(w, `{"ip":"203.0.113.8","city":"Test city","region":"Test region","country":"US"}`)
	}))
	defer proxy.Close()
	result, err := checkProxy(context.Background(), proxy.URL, "http://geo.example.test/json")
	if err != nil || !result.Reachable || result.ExitIP != "203.0.113.8" || result.City != "Test city" || result.Country != "US" {
		t.Fatalf("probe: %+v %v", result, err)
	}
	failure, err := checkProxy(context.Background(), "http://127.0.0.1:1", "http://geo.example.test/json")
	if err != nil || failure.Reachable || failure.ErrorCode == "" {
		t.Fatalf("failure: %+v %v", failure, err)
	}
}

func TestProxyProbeSeparatesRateLimitAndRejectsInvalidCountry(t *testing.T) {
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }))
	defer limited.Close()
	result, err := checkProxy(context.Background(), limited.URL, "http://geo.example.test/json")
	if err != nil || result.Reachable || result.ErrorCode != "lookup_rate_limited" {
		t.Fatalf("rate limit: %+v %v", result, err)
	}
	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"ip":"203.0.113.8","country":"??"}`) }))
	defer invalid.Close()
	result, err = checkProxy(context.Background(), invalid.URL, "http://geo.example.test/json")
	if err != nil || !result.Reachable || result.Country != "" {
		t.Fatalf("invalid location: %+v %v", result, err)
	}
}
