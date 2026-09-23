package audit_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/vault"
)

func TestManualManagementAuditAndRollback(t *testing.T) {
	base := context.Background()
	dir := t.TempDir()
	connection, err := storage.Open(base, filepath.Join(dir, "management.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Setup(base, "synthetic-admin", "synthetic-pass", "Synthetic workspace"); err != nil {
		t.Fatal(err)
	}
	member, err := identity.CreateMember(base, "synthetic-member", "synthetic-pass")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := vault.Open(filepath.Join(dir, "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := accounts.New(connection, cipher)
	credential := accounts.Credential{AccountID: "synthetic-subject", AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	account, err := subscriptions.Authorize(base, "Synthetic", credential, "")
	if err != nil {
		t.Fatal(err)
	}
	forwarding := gateway.New(base, connection, subscriptions, nil)
	defer forwarding.Close()
	ctx := audit.WithActor(base, audit.Actor{TenantID: 1, ID: 1, Username: "synthetic-admin", Role: "admin", Source: "user"})
	operations := []struct {
		action string
		run    func() error
	}{
		{"member.create", func() error { _, err := identity.CreateMember(ctx, "another-member", "synthetic-pass"); return err }},
		{"member.update", func() error { _, err := identity.SetMemberEnabled(ctx, member.ID, false); return err }},
		{"member.password", func() error { return identity.ResetMemberPassword(ctx, member.ID, "replacement-pass") }},
		{"user.password", func() error { return identity.ChangePassword(ctx, 1, "synthetic-pass", "replacement-pass") }},
		{"member.limits", func() error { return forwarding.SetMemberLimits(ctx, member.ID, 30, 2) }},
		{"member.groups", func() error { return groups.New(connection).SetMemberGroups(ctx, member.ID, []int64{}) }},
		{"account.authorize", func() error { _, err := subscriptions.Authorize(ctx, "Synthetic", credential, account.ID); return err }},
		{"account.update", func() error { _, err := subscriptions.SetEnabled(ctx, account.ID, false); return err }},
		{"account.concurrency", func() error { return forwarding.SetConcurrency(ctx, account.ID, 3) }},
		{"account.resume", func() error { return forwarding.Resume(ctx, account.ID) }},
		{"account.delete", func() error { return subscriptions.Delete(ctx, account.ID) }},
	}
	for _, op := range operations {
		t.Run(op.action, func(t *testing.T) {
			if _, err := connection.Exec("CREATE TRIGGER fail_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END"); err != nil {
				t.Fatal(err)
			}
			if err := op.run(); err == nil {
				t.Fatal("mutation succeeded despite audit failure")
			}
			if _, err := connection.Exec("DROP TRIGGER fail_audit"); err != nil {
				t.Fatal(err)
			}
			if err := op.run(); err != nil {
				t.Fatal("mutation not rolled back", err)
			}
			page, err := audit.New(connection).List(ctx, audit.Filter{})
			if err != nil || len(page.Events) == 0 || page.Events[0].Action != op.action {
				t.Fatal("missing successful audit", err)
			}
		})
	}
}
