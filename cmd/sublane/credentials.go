package main

import (
	"context"
	"database/sql"

	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

func openVault(ctx context.Context, connection *sql.DB, directory, configured string) (*vault.Vault, error) {
	path, err := vault.KeyPath(directory, configured)
	if err != nil {
		return nil, err
	}
	q := db.New(connection)
	accounts, err := q.CountAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := q.CountEncryptedKeys(ctx)
	if err != nil {
		return nil, err
	}
	webhooks, err := q.CountWebhookSecrets(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := q.CountContentSecrets(ctx)
	if err != nil {
		return nil, err
	}
	factors, err := q.CountTOTPSecrets(ctx)
	if err != nil {
		return nil, err
	}
	turnstileSettings, err := q.CountTurnstileSettings(ctx)
	if err != nil {
		return nil, err
	}
	// Settings and content rules can hold secrets before any subscription account exists.
	return vault.Open(path, accounts == 0 && keys == 0 && webhooks == 0 && rules == 0 && factors == 0 && turnstileSettings == 0)
}
