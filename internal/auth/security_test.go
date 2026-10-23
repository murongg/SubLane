package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/vault"
	"github.com/pquerna/otp/totp"
)

func securityFixture(t *testing.T) (*Service, Session) {
	t.Helper()
	s, conn := fixture(t)
	cipher, err := vault.Open(filepath.Join(t.TempDir(), "credentials.key"), true)
	if err != nil {
		t.Fatal(err)
	}
	s, err = NewWithVault(conn, cipher)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1800000000, 0) }
	session, err := s.Setup(context.Background(), "security-test", syntheticPassword, "Synthetic workspace")
	if err != nil {
		t.Fatal(err)
	}
	return s, session
}

func enrollTOTP(t *testing.T, s *Service, session Session) (TOTPSetup, []string) {
	t.Helper()
	ctx := context.Background()
	setup, err := s.BeginTOTP(ctx, session.Token, Proof{Password: syntheticPassword})
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, s.now())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := s.EnableTOTP(ctx, session.Token, setup.Challenge, code)
	if err != nil {
		t.Fatal(err)
	}
	return setup, codes
}

func TestTOTPLoginRecoveryAndRevocation(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	old, err := s.Login(ctx, owner.User.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	setup, codes := enrollTOTP(t, s, owner)
	if len(codes) != 10 || !strings.HasPrefix(setup.QR, "data:image/png;base64,") {
		t.Fatal("missing setup QR or recovery codes")
	}
	state, err := s.State(ctx, old.Token)
	if err != nil || state.User != nil {
		t.Fatal("enrollment must revoke other sessions")
	}
	var encrypted []byte
	if err := s.db.QueryRow("SELECT totp_secret FROM users WHERE id=1").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), setup.Secret) {
		t.Fatal("TOTP secret persisted in plaintext")
	}
	if _, err := s.Login(ctx, owner.User.Username, syntheticPassword); !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("password bypass: %v", err)
	}
	code, _ := totp.GenerateCode(setup.Secret, s.now())
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, code); !errors.Is(err, ErrFactor) {
		t.Fatalf("enrollment code replay: %v", err)
	}
	s.now = func() time.Time { return time.Unix(1800000030, 0) }
	code, _ = totp.GenerateCode(setup.Secret, s.now())
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, code); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, code); !errors.Is(err, ErrFactor) {
		t.Fatalf("login code replay: %v", err)
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[0]); !errors.Is(err, ErrFactor) {
		t.Fatalf("recovery replay: %v", err)
	}
	security, err := s.SecurityState(ctx, owner.Token)
	if err != nil || !security.TOTPEnabled || security.RecoveryCodes != 9 {
		t.Fatalf("security state: %+v %v", security, err)
	}
	if err := s.DisableTOTP(ctx, owner.Token, Proof{Password: syntheticPassword, Code: codes[1]}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, owner.User.Username, syntheticPassword); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPEnrollmentBindingExpiryAndRecovery(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	if _, err := s.BeginTOTP(ctx, owner.Token, Proof{Password: "wrong-password"}); !errors.Is(err, ErrCurrentPassword) {
		t.Fatalf("missing reauthentication: %v", err)
	}
	pending, err := s.BeginTOTP(ctx, owner.Token, Proof{Password: syntheticPassword})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Login(ctx, owner.User.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(pending.Secret, s.now())
	if _, err := s.EnableTOTP(ctx, other.Token, pending.Challenge, code); !errors.Is(err, ErrChallenge) {
		t.Fatalf("cross-session enrollment: %v", err)
	}
	s.now = func() time.Time { return time.Unix(1800000400, 0) }
	if _, err := s.EnableTOTP(ctx, owner.Token, pending.Challenge, code); !errors.Is(err, ErrChallenge) {
		t.Fatalf("expired enrollment: %v", err)
	}
	_, codes := enrollTOTP(t, s, owner)
	if _, err := s.BeginTOTP(ctx, owner.Token, Proof{Password: syntheticPassword}); !errors.Is(err, ErrInput) {
		t.Fatalf("replaced enabled factor: %v", err)
	}
	replacement, err := s.RegenerateRecoveryCodes(ctx, owner.Token, Proof{Password: syntheticPassword, Code: codes[0]})
	if err != nil || len(replacement) != 10 {
		t.Fatalf("regenerate: %v", err)
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[1]); !errors.Is(err, ErrFactor) {
		t.Fatalf("old recovery code survived: %v", err)
	}
	if err := s.RecoverAdministrator(ctx, "new password 42"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, owner.User.Username, "new password 42"); err != nil {
		t.Fatalf("local recovery locked out: %v", err)
	}
}

func TestSecurityChangesRecheckSessionAndRollbackAuditFailures(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	setup, err := s.BeginTOTP(ctx, owner.Token, Proof{Password: syntheticPassword})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(ctx, owner.User.ID, syntheticPassword, "new password 42"); err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(setup.Secret, s.now())
	if _, err := s.EnableTOTP(ctx, owner.Token, setup.Challenge, code); !errors.Is(err, ErrChallenge) {
		t.Fatalf("revoked session enrolled factor: %v", err)
	}
	owner, err = s.Login(ctx, owner.User.Username, "new password 42")
	if err != nil {
		t.Fatal(err)
	}
	actorCtx := audit.WithActor(ctx, audit.Actor{TenantID: 1, ID: owner.User.ID, Username: owner.User.Username, Role: "admin", Source: "user"})
	setup, err = s.BeginTOTP(actorCtx, owner.Token, Proof{Password: "new password 42"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_factor_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'synthetic audit failure'); END"); err != nil {
		t.Fatal(err)
	}
	code, _ = totp.GenerateCode(setup.Secret, s.now())
	if _, err := s.EnableTOTP(actorCtx, owner.Token, setup.Challenge, code); err == nil {
		t.Fatal("enrollment survived audit failure")
	}
	if _, err := s.Login(ctx, owner.User.Username, "new password 42"); err != nil {
		t.Fatal("rolled-back factor blocked login", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_factor_audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnableTOTP(actorCtx, owner.Token, setup.Challenge, code); err != nil {
		t.Fatal(err)
	}
	var action string
	if err := s.db.QueryRow("SELECT action FROM audit_events ORDER BY id DESC LIMIT 1").Scan(&action); err != nil || action != "user.totp_enable" {
		t.Fatalf("missing audit: %q %v", action, err)
	}
}

func TestFactorFailuresAreLimitedPerAccount(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	_, codes := enrollTOTP(t, s, owner)
	for range 5 {
		if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, "invalid-code"); !errors.Is(err, ErrFactor) {
			t.Fatal(err)
		}
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[0]); err == nil || err.Error() != "rate_limited" {
		t.Fatalf("account factor attempts were not limited: %v", err)
	}
	s.now = func() time.Time { return time.Unix(1800000061, 0) }
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[0]); err != nil {
		t.Fatalf("factor budget did not expire: %v", err)
	}
	// A successful verification clears failed attempts; it must not throttle normal security management.
	for range 4 {
		if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, "invalid-code"); !errors.Is(err, ErrFactor) {
			t.Fatal(err)
		}
	}
	if _, err := s.LoginWithCode(ctx, owner.User.Username, syntheticPassword, codes[1]); err != nil {
		t.Fatal(err)
	}
	if err := s.DisableTOTP(ctx, owner.Token, Proof{Password: syntheticPassword, Code: codes[2]}); err != nil {
		t.Fatal(err)
	}
}
