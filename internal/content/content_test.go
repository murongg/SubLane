package content

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func fixture(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	conn, err := storage.Open(ctx, filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	identity, err := auth.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Setup(ctx, "synthetic-owner", "synthetic-pass42", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(t.TempDir(), "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	return New(conn, cipher, 1)
}

func TestConfigurationEncryptedRedactedAndAtomic(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	secret := "SYNTHETIC_SECRET_123456"
	initial, err := s.State(ctx)
	if err != nil || initial.Mode != "off" || initial.Revision != 0 || len(initial.Rules) != 0 {
		t.Fatalf("initial: %+v %v", initial, err)
	}
	saved, err := s.Update(ctx, Input{Mode: "block", Rules: []RuleInput{{Name: "Synthetic rule", Kind: "text", Pattern: &secret, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), secret) || len(saved.Rules) != 1 || saved.Revision != 1 {
		t.Fatalf("unsafe state: %s", raw)
	}
	var encrypted []byte
	if err = s.connection.QueryRowContext(ctx, "SELECT content_config FROM tenants WHERE id=1").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), secret) {
		t.Fatal("plaintext persisted")
	}
	replacement := New(s.connection, s.vault, 1)
	result, err := replacement.Check(ctx, []byte(`{"input":"SYNTHETIC_SECRET_123456"}`), 1<<20)
	if !errors.Is(err, ErrBlocked) || len(result.RuleIDs) != 1 || result.Revision != 1 {
		t.Fatalf("restart: %+v %v", result, err)
	}
	invalid := "["
	if _, err = s.Update(ctx, Input{Revision: 1, Mode: "block", Rules: []RuleInput{{Name: "Invalid", Kind: "regex", Pattern: &invalid, Enabled: true}}}); !errors.Is(err, ErrInput) {
		t.Fatal("invalid regex accepted", err)
	}
	if _, err = s.Update(ctx, Input{Revision: 0, Mode: "off"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale edit accepted", err)
	}
	state, _ := s.State(ctx)
	if state.Revision != 1 || state.Mode != "block" {
		t.Fatal("invalid edit changed policy", state)
	}
	state, err = s.Update(ctx, Input{Revision: 1, Mode: "observe", Rules: []RuleInput{{ID: saved.Rules[0].ID, Name: "Renamed", Kind: "text", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.Check(ctx, []byte(`{"input":"SYNTHETIC_SECRET_123456"}`), 1<<20)
	if err != nil || result.Mode != "observe" || len(result.RuleIDs) != 1 {
		t.Fatalf("preserved pattern: %+v %v", result, err)
	}
	other := New(s.connection, s.vault, 2)
	if _, err = other.State(ctx); err == nil {
		t.Fatal("missing workspace accepted")
	}
	if err = s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.connection.ExecContext(ctx, "UPDATE tenants SET content_config=x'01' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Check(ctx, []byte(`{"input":"safe"}`), 1<<20); !errors.Is(err, ErrUnavailable) {
		t.Fatal("corrupt policy passed", err)
	}
	if err = s.Verify(ctx); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
}

func TestDecodedStringsAndBoundedChecks(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	pattern := `DEMO_TOKEN_[A-Z0-9]{8}`
	saved, err := s.Update(ctx, Input{Mode: "block", Rules: []RuleInput{{Name: "Synthetic regex", Kind: "regex", Pattern: &pattern, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"instructions":"DEMO_TOKEN_ABCDEF12"}`, `{"input":[{"type":"function_call_output","output":"DEMO_\u0054OKEN_ABCDEF12"}]}`, `{"tools":[{"description":"DEMO_TOKEN_ABCDEF12"}]}`} {
		result, err := s.Check(ctx, []byte(raw), 1<<20)
		if !errors.Is(err, ErrBlocked) || len(result.RuleIDs) != 1 {
			t.Fatalf("missed decoded text: %+v %v", result, err)
		}
	}
	result, err := s.Check(ctx, []byte(`{"input":"DEMO_TOKEN_abcdef12"}`), 1<<20)
	if err != nil || len(result.RuleIDs) != 0 {
		t.Fatal("case insensitive match", result, err)
	}
	for _, raw := range []string{`{"input":"DEMO_TOKEN_ABCDEF12"}`, `{"input":`} {
		result, err = s.Check(ctx, []byte(raw), 8)
		if !errors.Is(err, ErrUnavailable) || result.CheckFailed != true {
			t.Fatalf("incomplete check passed: %+v %v", result, err)
		}
	}
	observed, err := s.Update(ctx, Input{Revision: saved.Revision, Mode: "observe", Rules: []RuleInput{{ID: saved.Rules[0].ID, Name: "Synthetic regex", Kind: "regex", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.Check(ctx, []byte(`{"input":"safe"}`), 8)
	if err != nil || !result.CheckFailed || result.Revision != observed.Revision {
		t.Fatalf("observation failure: %+v %v", result, err)
	}
	matched, err := s.Test(ctx, RuleInput{ID: saved.Rules[0].ID, Name: "Synthetic regex", Kind: "regex"}, "DEMO_TOKEN_ABCDEF12")
	if err != nil || !matched {
		t.Fatal("saved rule test", matched, err)
	}
}

func TestInvalidRules(t *testing.T) {
	for _, pattern := range []string{"", "[", "a*", `(?=synthetic)`} {
		s := fixture(t)
		if _, err := s.Update(context.Background(), Input{Mode: "block", Rules: []RuleInput{{Name: "Invalid", Kind: "regex", Pattern: &pattern, Enabled: true}}}); !errors.Is(err, ErrInput) {
			t.Fatalf("accepted %q: %v", pattern, err)
		}
	}
}
