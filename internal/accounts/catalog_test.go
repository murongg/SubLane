package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestCatalogPersistsAndRejectsObsoleteAuthorization(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.db")
	connection, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	s := New(connection, cipher)
	credential := Credential{AccountID: "synthetic-catalog", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	a, err := s.Authorize(ctx, "Synthetic", credential, "")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.Catalog(ctx, a.ID)
	if err != nil || initial.UpdatedAt != 0 || initial.Models == nil {
		t.Fatal("new account must be unknown", initial, err)
	}
	at := time.Now().Unix()
	if err := s.SaveCatalog(ctx, a.ID, initial.Revision, []string{"synthetic-z", "synthetic-a", "synthetic-z"}, at, "synthetic:v1", "synthetic-z"); err != nil {
		t.Fatal(err)
	}
	connection.Close()
	connection, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	s = New(connection, cipher)
	saved, err := s.Catalog(ctx, a.ID)
	if err != nil || saved.UpdatedAt != at || saved.Source != "synthetic:v1" || !reflect.DeepEqual(saved.Models, []string{"synthetic-a", "synthetic-z"}) {
		t.Fatal("snapshot did not survive restart", saved, err)
	}
	if !reflect.DeepEqual(saved.HiddenModels, []string{"synthetic-z"}) || !reflect.DeepEqual(saved.VisibleModels(), []string{"synthetic-a"}) || len(saved.Models) != 2 {
		t.Fatal("visibility replaced callable capabilities", saved)
	}
	if _, err := s.Authorize(ctx, "Synthetic", credential, a.ID); err != nil {
		t.Fatal(err)
	}
	current, err := s.Catalog(ctx, a.ID)
	if err != nil || current.UpdatedAt != 0 || current.Revision == saved.Revision {
		t.Fatal("reauthorization retained old capabilities", current, err)
	}
	if err := s.SaveCatalog(ctx, a.ID, saved.Revision, saved.Models, at+1, "synthetic:v1"); !errors.Is(err, ErrCatalogChanged) {
		t.Fatal("obsolete fetch published", err)
	}
	if err := s.SaveCatalog(ctx, a.ID, current.Revision, []string{}, at, "synthetic:v1"); err != nil {
		t.Fatal(err)
	}
	empty, err := s.Catalog(ctx, a.ID)
	if err != nil || empty.UpdatedAt == 0 || len(empty.Models) != 0 {
		t.Fatal("known empty catalog became unknown", empty, err)
	}
	if _, err := s.SetEnabled(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetEnabled(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCatalog(ctx, a.ID, current.Revision, saved.Models, at, "synthetic:v1"); !errors.Is(err, ErrCatalogChanged) {
		t.Fatal("disable/re-enable allowed an old fetch", err)
	}
	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Catalog(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted snapshot remained", err)
	}
}

func TestCatalogRejectsInvalidModelsWithoutReplacingSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(ctx, filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	seedInitialTenant(t, connection)
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	s := New(connection, cipher)
	a, err := s.Authorize(ctx, "Synthetic", Credential{AccountID: "synthetic", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "")
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := s.Catalog(ctx, a.ID)
	for _, models := range [][]string{{"bad\nmodel"}, {""}, make([]string, 513)} {
		if err := s.SaveCatalog(ctx, a.ID, catalog.Revision, models, time.Now().Unix(), "synthetic:v1"); !errors.Is(err, ErrInput) {
			t.Fatal("invalid catalog accepted", err)
		}
	}
	for _, hidden := range [][]string{{"synthetic-unknown"}, {"bad\nmodel"}, make([]string, 513)} {
		if err := s.SaveCatalog(ctx, a.ID, catalog.Revision, []string{"synthetic-model"}, time.Now().Unix(), "synthetic:v1", hidden...); !errors.Is(err, ErrInput) {
			t.Fatal("invalid hidden capability accepted", err)
		}
	}
	catalog, err = s.Catalog(ctx, a.ID)
	if err != nil || catalog.UpdatedAt != 0 {
		t.Fatal("invalid catalog published", err)
	}
}

func TestDecodeCatalogValidatesHiddenCapabilities(t *testing.T) {
	for _, raw := range []string{
		`{"models":["synthetic-model"],"hidden_models":["synthetic-unknown"],"updated_at":1}`,
		`{"models":["synthetic-model"],"hidden_models":["bad\nmodel"],"updated_at":1}`,
	} {
		if _, err := DecodeCatalog([]byte(raw), 0); !errors.Is(err, ErrInput) {
			t.Fatal("invalid hidden capabilities decoded", err)
		}
	}
	value, err := DecodeCatalog([]byte(`{"models":["synthetic-model"],"updated_at":1}`), 0)
	if err != nil || !reflect.DeepEqual(value.VisibleModels(), []string{"synthetic-model"}) {
		t.Fatal("legacy catalog visibility changed", value, err)
	}
}

func TestCatalogDecodesMaximumHiddenCapabilities(t *testing.T) {
	models := make([]string, 512)
	for i := range models {
		prefix := fmt.Sprintf("synthetic-%03d-", i)
		models[i] = prefix + strings.Repeat("x", 128-len(prefix))
	}
	raw, err := json.Marshal(Catalog{Models: models, HiddenModels: models, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeCatalog(raw, 0)
	if err != nil || len(value.Models) != len(models) || len(value.VisibleModels()) != 0 {
		t.Fatal("valid maximum-size hidden catalog cannot be read", err)
	}
	if _, err := DecodeCatalog(append(raw, []byte(strings.Repeat(" ", 512<<10))...), 0); !errors.Is(err, ErrInput) {
		t.Fatal("oversized snapshot accepted", err)
	}
}

func TestCatalogRoundTripsCapabilitiesAndReadsLegacySnapshots(t *testing.T) {
	raw := []byte(`{"models":["synthetic-model"],"updated_at":1,"capabilities":{"synthetic-model":{"context_window":200000,"max_output_tokens":16000,"supported_reasoning_levels":["low","high"],"supports_images":false,"supports_tools":true}}}`)
	value, err := DecodeCatalog(raw, 7)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	var got map[string]any
	json.Unmarshal(encoded, &got)
	if got["capabilities"] == nil {
		t.Fatalf("capabilities discarded: %s", encoded)
	}
	for _, invalid := range []string{
		`{"models":["synthetic-model"],"updated_at":1,"capabilities":{"synthetic-other":{"supports_tools":true}}}`,
		`{"models":["synthetic-model"],"updated_at":1,"capabilities":{"synthetic-model":{"context_window":-1}}}`,
		`{"models":["synthetic-model"],"updated_at":1,"capabilities":{"synthetic-model":{"max_output_tokens":1000000001}}}`,
		`{"models":["synthetic-model"],"updated_at":1,"capabilities":{"synthetic-model":{"supported_reasoning_levels":["bad\nlevel"]}}}`,
	} {
		if _, err := DecodeCatalog([]byte(invalid), 0); !errors.Is(err, ErrInput) {
			t.Fatalf("invalid capabilities accepted: %s: %v", invalid, err)
		}
	}
	legacy, err := DecodeCatalog([]byte(`{"models":["synthetic-model"],"updated_at":1}`), 0)
	if err != nil || len(legacy.Models) != 1 {
		t.Fatal(legacy, err)
	}
}

func TestCatalogMaximumCapabilitiesFitSnapshotBound(t *testing.T) {
	value := Catalog{Models: []string{}, Capabilities: map[string]ModelCapabilities{}, UpdatedAt: 1, Source: strings.Repeat("x", 64)}
	limit := int64(MaxModelTokens)
	supported := true
	levels := []string{"synthetic-lvl-a", "synthetic-lvl-b", "synthetic-lvl-c", "synthetic-lvl-d", "synthetic-lvl-e", "synthetic-lvl-f", "synthetic-lvl-g", "synthetic-lvl-h"}
	for i := range 512 {
		prefix := fmt.Sprintf("synthetic-%03d-", i)
		id := prefix + strings.Repeat("x", 128-len(prefix))
		value.Models = append(value.Models, id)
		value.HiddenModels = append(value.HiddenModels, id)
		value.Capabilities[id] = ModelCapabilities{ContextWindow: &limit, MaxOutputTokens: &limit, SupportedReasoningLevels: levels, SupportsImages: &supported, SupportsTools: &supported}
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 512<<10 {
		t.Fatal("maximum bounded metadata exceeds snapshot limit", len(raw), err)
	}
	decoded, err := DecodeCatalog(raw, 0)
	if err != nil || len(decoded.Capabilities) != 512 {
		t.Fatal("complete catalog cannot be decoded", err)
	}
}
