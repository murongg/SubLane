package apikey

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

// Reveal is owner-only, even for administrators. Metadata and gateway authentication never decrypt secrets.
func (s *Service) Reveal(ctx context.Context, userID, keyID int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	row, err := q.GetKeySecret(ctx, db.GetKeySecretParams{ID: keyID, UserID: userID, TenantID: s.tenantID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if row.RevokedAt != nil {
		return "", ErrRevoked
	}
	if row.Secret == nil {
		return "", ErrNotCopyable
	}
	plain, err := s.decryptSecret(row.UserID, row.ID, row.Secret, row.TokenHash)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	if err := audit.Record(ctx, q, "key.reveal", "key", audit.ID(keyID)); err != nil {
		return "", err
	}
	// Disclosure is successful only after its metadata-only audit event is durable.
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return string(plain), nil
}
func (s *Service) decryptSecret(userID, keyID int64, encrypted, digest []byte) ([]byte, error) {
	plain, err := s.vault.OpenAPIKey(userID, keyID, encrypted)
	if err != nil {
		return nil, err
	}
	actual := sha256.Sum256(plain)
	if len(plain) != 46 || subtle.ConstantTimeCompare(actual[:], digest) != 1 {
		clear(plain)
		return nil, vault.ErrDecrypt
	}
	return plain, nil
}

// Verify uses bounded pages because the key count can grow with the number of members.
func (s *Service) Verify(ctx context.Context) error {
	var cursor int64
	for {
		rows, err := s.queries.ListKeySecrets(ctx, cursor)
		if err != nil {
			return err
		}
		for _, row := range rows {
			plain, err := s.decryptSecret(row.UserID, row.ID, row.Secret, row.TokenHash)
			if err != nil {
				return err
			}
			clear(plain)
			cursor = row.ID
		}
		if len(rows) < 100 {
			return nil
		}
	}
}
