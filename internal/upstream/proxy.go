package upstream

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/murongg/SubLane/internal/accounts"
)

type proxyContextKey struct{}

// WithProxyURL selects an already validated, workspace-owned proxy for an OAuth exchange.
func WithProxyURL(ctx context.Context, address string) context.Context {
	if address == "" {
		return ctx
	}
	return context.WithValue(ctx, proxyContextKey{}, address)
}

func credentialContext(ctx context.Context, credential accounts.Credential) context.Context {
	return WithProxyURL(ctx, credential.ProxyURL)
}

// Some SDK paths create a fresh request context. The per-call transport keeps
// the account's selected exit even when the SDK does not preserve context values.
type boundTransport struct {
	base    http.RoundTripper
	address string
}

func (t *boundTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.address == "" {
		return t.base.RoundTrip(request)
	}
	return t.base.RoundTrip(request.Clone(WithProxyURL(request.Context(), t.address)))
}

// proxyTransport keeps a separate connection pool per proxy. A request takes one
// snapshot of the binding; an in-flight stream is never moved or replayed.
type proxyTransport struct {
	base   http.RoundTripper
	mu     sync.Mutex
	routes map[string]*http.Transport
	order  []string
}

func (t *proxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	address, _ := request.Context().Value(proxyContextKey{}).(string)
	if address == "" {
		return t.base.RoundTrip(request)
	}
	u, err := url.Parse(address)
	if err != nil || u == nil || u.Host == "" {
		return nil, ErrUpstream
	}
	t.mu.Lock()
	if t.routes == nil {
		t.routes = make(map[string]*http.Transport)
	}
	transport := t.routes[address]
	if transport == nil {
		// Proxy edits can introduce new addresses indefinitely; retire the oldest
		// idle pool without disturbing requests already using its transport.
		if len(t.order) == 64 {
			oldest := t.order[0]
			t.order = t.order[1:]
			t.routes[oldest].CloseIdleConnections()
			delete(t.routes, oldest)
		}
		if base, ok := t.base.(*http.Transport); ok {
			transport = base.Clone()
		} else {
			transport = http.DefaultTransport.(*http.Transport).Clone()
		}
		transport.Proxy = http.ProxyURL(u)
		t.routes[address] = transport
		t.order = append(t.order, address)
	}
	t.mu.Unlock()
	return transport.RoundTrip(request)
}

func (t *proxyTransport) CloseIdleConnections() {
	if base, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		base.CloseIdleConnections()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, transport := range t.routes {
		transport.CloseIdleConnections()
	}
}
