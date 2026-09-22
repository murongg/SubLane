package pricing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestParseLiteLLMPriceAndModelAliases(t *testing.T) {
	prices, err := parsePrices([]byte(`{"gpt-5.1-codex":{"input_cost_per_token":0.00000125,"cache_read_input_token_cost":0.000000125,"output_cost_per_token":0.00001}}`), "remote")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{prices: prices}
	got, ok := s.Lookup("codex/gpt-5.1-codex(high)")
	if !ok || got.Input != 1250000 || got.Cached != 125000 || got.Output != 10000000 {
		t.Fatalf("unexpected price: %+v, %v", got, ok)
	}
}

func TestRefreshUsesHashAndKeepsCacheOnFailure(t *testing.T) {
	var jsonCalls atomic.Int32
	var hash = "old"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hash" {
			_, _ = fmt.Fprintln(w, hash)
			return
		}
		jsonCalls.Add(1)
		if hash == "broken" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprintln(w, `{"gpt-5.1-codex":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002}}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	s, err := New(Config{CachePath: filepath.Join(dir, "prices.json"), HashPath: filepath.Join(dir, "prices.sha256"), URL: server.URL, HashURL: server.URL + "/hash"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jsonCalls.Load() != 1 {
		t.Fatalf("first refresh downloaded %d times", jsonCalls.Load())
	}
	if err = s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jsonCalls.Load() != 1 {
		t.Fatalf("unchanged hash downloaded %d times", jsonCalls.Load())
	}
	hash = "broken"
	if err = s.Refresh(context.Background()); err == nil {
		t.Fatal("expected failed download")
	}
	if got, ok := s.Lookup("gpt-5.1-codex"); !ok || got.Input != 1000000 {
		t.Fatalf("cache was discarded: %+v, %v", got, ok)
	}
}

func TestFallbackAndOverride(t *testing.T) {
	dir := t.TempDir()
	fallback := filepath.Join(dir, "fallback.json")
	override := filepath.Join(dir, "override.json")
	if err := os.WriteFile(fallback, []byte(`{"claude-3-7-sonnet-20250219":{"input_cost_per_token":0.000003,"output_cost_per_token":0.000004}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte(`{"claude-3-7-sonnet-20250219":{"output_cost_per_token":0.000005}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{FallbackPath: fallback, OverridePath: override})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Lookup("claude-3-7-sonnet-20250219")
	if !ok || got.Input != 3000000 || got.Output != 5000000 {
		t.Fatalf("fallback/override merge failed: %+v, %v", got, ok)
	}
}

func TestHashForDocument(t *testing.T) {
	if got := hashDocument([]byte("synthetic")); got != "b3cc0475bb78a5026098858e9889acf666d31062d513d303314eca31d36e72f2" {
		t.Fatalf("unexpected hash: %s", got)
	}
}
