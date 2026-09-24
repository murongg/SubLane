package upstream

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
)

func TestAccountModelAndRefreshUseBoundProxy(t *testing.T) {
	var modelRequests, refreshRequests int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "Basic c3ludGhldGljLXVzZXI6c3ludGhldGljLXBhc3M=" {
			t.Error("proxy authentication missing")
		}
		switch r.URL.Path {
		case "/backend-api/codex/models":
			modelRequests++
			fmt.Fprint(w, `{"models":[{"slug":"synthetic-model","visibility":"list"}]}`)
		case "/oauth/token":
			refreshRequests++
			fmt.Fprint(w, `{"access_token":"synthetic-new-access","refresh_token":"synthetic-new-refresh","expires_in":3600}`)
		default:
			t.Errorf("unexpected proxied path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer proxy.Close()
	client := New()
	defer client.Close()
	client.baseURL = "http://synthetic-upstream.example.test/backend-api/codex"
	client.tokenURL = "http://synthetic-upstream.example.test/oauth/token"
	credential := accounts.Credential{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-account", ExpiresAt: time.Now().Add(time.Hour).Unix(), ProxyURL: strings.Replace(proxy.URL, "http://", "http://synthetic-user:synthetic-pass@", 1)}
	models, err := client.Models(context.Background(), credential)
	if err != nil || len(models) != 1 || models[0].ID != "synthetic-model" {
		t.Fatalf("models: %+v %v", models, err)
	}
	updated, err := client.Refresh(context.Background(), credential)
	if err != nil || updated.AccessToken != "synthetic-new-access" {
		t.Fatalf("refresh: %+v %v", updated, err)
	}
	if modelRequests != 1 || refreshRequests != 1 {
		t.Fatalf("proxy calls: models=%d refresh=%d", modelRequests, refreshRequests)
	}
}

func TestHTTPSModelDiscoveryUsesProxyConnectTunnel(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"slug":"synthetic-model","visibility":"list"}]}`)
	}))
	defer upstream.Close()
	var connected atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != strings.TrimPrefix(upstream.URL, "https://") {
			t.Errorf("wrong proxy tunnel: %s %s", r.Method, r.Host)
			return
		}
		connected.Store(true)
		destination, err := net.Dial("tcp", r.Host)
		if err != nil {
			t.Error(err)
			return
		}
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			destination.Close()
			t.Error(err)
			return
		}
		fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		finished := make(chan struct{})
		go func() { io.Copy(destination, client); destination.Close(); close(finished) }()
		io.Copy(client, destination)
		client.Close()
		<-finished
	}))
	defer proxy.Close()
	base := upstream.Client().Transport.(*http.Transport).Clone()
	base.Proxy = nil
	client := NewWithTransport(base)
	defer client.Close()
	client.baseURL = upstream.URL + "/backend-api/codex"
	credential := accounts.Credential{AccessToken: "synthetic-access", AccountID: "synthetic-account", ProxyURL: proxy.URL}
	models, err := client.Models(context.Background(), credential)
	if err != nil || len(models) != 1 || !connected.Load() {
		t.Fatalf("HTTPS proxy tunnel: %+v %v connected=%t", models, err, connected.Load())
	}
}

func TestBoundProxySurvivesDetachedSDKRequestContext(t *testing.T) {
	called := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	transport := &proxyTransport{base: &http.Transport{Proxy: nil}}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://synthetic-upstream.example.test/model", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&boundTransport{base: transport, address: proxy.URL}).RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !called {
		t.Fatal("detached SDK request bypassed bound proxy")
	}
}

func TestProxyConnectionPoolsRemainBoundedAfterAddressChanges(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer proxy.Close()
	transport := &proxyTransport{base: &http.Transport{Proxy: nil}}
	defer transport.CloseIdleConnections()
	for index := range 70 {
		address := strings.Replace(proxy.URL, "http://", fmt.Sprintf("http://synthetic-%d:synthetic-pass@", index), 1)
		request, err := http.NewRequestWithContext(WithProxyURL(context.Background(), address), http.MethodGet, "http://synthetic-upstream.example.test/model", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if len(transport.routes) > 64 {
		t.Fatalf("retained %d proxy connection pools", len(transport.routes))
	}
}
