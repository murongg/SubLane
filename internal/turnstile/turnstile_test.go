package turnstile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestSettingsPersistEncryptedAndPreserveSecret(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.Actor{ID: 0, TenantID: 1, Username: "synthetic-local", Role: "admin", Source: "local"})
	dir := t.TempDir()
	conn, err := storage.Open(ctx, filepath.Join(dir, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(ctx, "synthetic-owner", "synthetic-password", "Synthetic"); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ctx, conn, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, "", "example.test"); err != nil {
		t.Fatal("disabled verification", err)
	}
	if _, err := s.Save(ctx, Input{Enabled: true, SiteKey: "synthetic-site"}); !errors.Is(err, ErrInput) {
		t.Fatal("enabled without secret", err)
	}
	secret := "synthetic-secret"
	state, err := s.Save(ctx, Input{Enabled: true, SiteKey: "synthetic-site", SecretKey: &secret})
	if err != nil || !state.Enabled || !state.SecretConfigured {
		t.Fatal(state, err)
	}
	var persisted string
	if err := conn.QueryRow("SELECT value FROM settings WHERE key='instance.turnstile'").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, secret) {
		t.Fatal("stored plaintext secret")
	}
	reopened, err := New(ctx, conn, v, nil)
	if err != nil || reopened.State() != state {
		t.Fatal("restart lost setting", err)
	}
	state, err = reopened.Save(ctx, Input{Enabled: true, SiteKey: "synthetic-replacement"})
	if err != nil || !state.SecretConfigured {
		t.Fatal("omitted secret was not preserved", err)
	}
	empty := ""
	if _, err := reopened.Save(ctx, Input{Enabled: true, SiteKey: state.SiteKey, SecretKey: &empty}); !errors.Is(err, ErrInput) {
		t.Fatal("cleared enabled secret", err)
	}
	if _, err := reopened.Save(ctx, Input{Enabled: false, SecretKey: &empty}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow("SELECT count(*) FROM audit_events WHERE action='settings.update' AND resource_id='turnstile'").Scan(&count); err != nil || count != 3 {
		t.Fatal("audit", count, err)
	}
	wrong, _ := vault.Open(filepath.Join(t.TempDir(), "wrong-key"), true)
	if _, err := New(ctx, conn, wrong, nil); err == nil {
		t.Fatal("accepted wrong encryption key")
	}
}

func TestVerificationFailsClosed(t *testing.T) {
	ctx := context.Background()
	conn, err := storage.Open(ctx, filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	v, _ := vault.Open(filepath.Join(t.TempDir(), "key"), true)
	body, status, calls := `{"success":true,"hostname":"example.test","action":"login"}`, 200, 0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/turnstile/v0/siteverify" {
			t.Error("wrong request", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("secret") != "synthetic-secret" || r.Form.Get("response") != "synthetic-token" {
			t.Error("missing verification fields")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer fake.Close()
	client := fake.Client()
	client.Transport = rewriteTransport{base: client.Transport, target: fake.URL}
	s, err := New(ctx, conn, v, client)
	if err != nil {
		t.Fatal(err)
	}
	secret := "synthetic-secret"
	if _, err := s.Save(ctx, Input{Enabled: true, SiteKey: "synthetic-site", SecretKey: &secret}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", strings.Repeat("x", 2049)} {
		if err := s.Verify(ctx, token, "example.test"); !errors.Is(err, ErrRejected) {
			t.Fatal("accepted invalid token", err)
		}
	}
	if calls != 0 {
		t.Fatal("sent invalid tokens upstream")
	}
	if err := s.Verify(ctx, "synthetic-token", "example.test"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"success":false}`, `{"success":true,"hostname":"foreign.test","action":"login"}`, `{"success":true,"hostname":"example.test","action":"register"}`} {
		body = invalid
		if err := s.Verify(ctx, "synthetic-token", "example.test"); !errors.Is(err, ErrRejected) {
			t.Fatal("accepted failed verification", err)
		}
	}
	for _, invalid := range []string{"not-json", strings.Repeat("x", 17000), `{"success":true} {}`} {
		body = invalid
		if err := s.Verify(ctx, "synthetic-token", "example.test"); !errors.Is(err, ErrUnavailable) {
			t.Fatal("accepted malformed response", err)
		}
	}
	status = 503
	if err := s.Verify(ctx, "synthetic-token", "example.test"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("accepted HTTP error", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Verify(cancelled, "synthetic-token", "example.test"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("ignored cancellation", err)
	}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.URL.Scheme = "http"
	copy.URL.Host = strings.TrimPrefix(t.target, "http://")
	return t.base.RoundTrip(copy)
}
