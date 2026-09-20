package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/vault"
)

func providerGateway(t *testing.T, transport http.RoundTripper) (*Service, map[string]string) {
	return providerFixture(t, transport, true)
}

// Codex protocol fixtures must not accidentally execute their synthetic SSE through another provider.
func codexGateway(t *testing.T, transport http.RoundTripper) (*Service, map[string]string) {
	t.Helper()
	service, ids := providerGateway(t, transport)
	if _, err := groups.New(service.db).Save(context.Background(), 1, groups.Input{Name: "Default", Enabled: true, AccountIDs: []string{ids["codex"]}}); err != nil {
		t.Fatal(err)
	}
	return service, ids
}
func discoveryGateway(t *testing.T, transport http.RoundTripper) (*Service, map[string]string) {
	return providerFixture(t, transport, false)
}
func providerFixture(t *testing.T, transport http.RoundTripper, known bool, version ...func() string) (*Service, map[string]string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Setup(ctx, "synthetic-admin", "synthetic-pass"); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	service := accounts.New(connection, cipher)
	ids := map[string]string{}
	for _, provider := range []string{"codex", "claude", "antigravity"} {
		row, err := service.Authorize(ctx, provider, accounts.Credential{Provider: provider, AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix(), Metadata: map[string]json.RawMessage{"project_id": json.RawMessage(`"synthetic-project"`)}}, "")
		if err != nil {
			t.Fatal(err)
		}
		ids[provider] = row.ID
		if known {
			if err := service.SaveCatalog(ctx, row.ID, 0, []string{"synthetic-model"}, time.Now().Unix(), upstream.CatalogSource(provider)); err != nil {
				t.Fatal(err)
			}
		}
	}
	client := upstream.NewWithTransport(transport, version...)
	t.Cleanup(client.Close)
	gateway := New(ctx, connection, service, client)
	t.Cleanup(gateway.Close)
	return gateway, ids
}

func TestAffinityIsPartitionedByProviderAndPersists(t *testing.T) {
	gateway, ids := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected network")
		return nil, errors.New("synthetic")
	}))
	ctx := context.Background()
	for range 2 {
		for provider, id := range ids {
			actual, _, err := gateway.selectAccount(ctx, 1, 1, "same-session", provider, "", Responses)
			if err != nil || actual != id {
				t.Fatal("provider binding crossed", provider, actual, err)
			}
		}
	}
	if _, err := gateway.accounts.SetEnabled(ctx, ids["claude"], false); err != nil {
		t.Fatal(err)
	}
	reopened := New(ctx, gateway.db, gateway.accounts, gateway.provider)
	defer reopened.Close()
	_, err := reopened.Open(ctx, 1, 1, []byte(`{"model":"claude/synthetic-model","input":"synthetic"}`), http.Header{"Session_id": {"same-session"}}, Responses)
	if !errors.Is(err, accounts.ErrDisabled) {
		t.Fatal("disabled provider binding moved", err)
	}
	id, _, err := reopened.selectAccount(ctx, 1, 1, "same-session", "codex", "", Responses)
	if err != nil || id != ids["codex"] {
		t.Fatal("another provider affected", err)
	}
}

func TestModelCatalogKeepsHealthyProvidersWithNativeIDs(t *testing.T) {
	gateway, _ := discoveryGateway(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"models":[{"slug":"synthetic-codex"}]}`
		status := 200
		switch r.URL.Host {
		case "api.anthropic.com":
			status = 503
			body = `{}`
		case "daily-cloudcode-pa.googleapis.com":
			body = `{"models":{"synthetic-gemini":{}}}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	models, err := gateway.Models(context.Background(), 1, 1)
	if err != nil {
		t.Fatal("one provider hid every model", err)
	}
	got := []string{}
	for _, model := range models {
		got = append(got, model.ID)
	}
	if strings.Join(got, ",") != "synthetic-codex,synthetic-gemini" {
		t.Fatal("unexpected native model catalog", got)
	}
}

func TestUnsupportedQuotaDoesNotVerifyOrPublishSnapshot(t *testing.T) {
	gateway, ids := providerGateway(t, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unsupported quota made a network call")
		return nil, errors.New("synthetic")
	}))
	ctx := context.Background()
	for _, kind := range []string{"claude", "antigravity"} {
		// Import-equivalent state: no successful provider request has verified this account.
		if _, err := gateway.db.Exec("UPDATE accounts SET status='unverified' WHERE id=?", ids[kind]); err != nil {
			t.Fatal(err)
		}
		snapshot, err := gateway.Usage(ctx, ids[kind])
		if err == nil || snapshot.UpdatedAt != 0 {
			t.Fatal("unsupported quota published a successful observation", kind, err)
		}
		row, err := gateway.accounts.Get(ctx, ids[kind])
		if err != nil || row.Status != "unverified" {
			t.Fatal("unsupported quota verified account", err)
		}
	}
}
