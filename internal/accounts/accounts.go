package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
)

var (
	ErrInput       = errors.New("invalid_account_input")
	ErrDuplicate   = errors.New("account_exists")
	ErrIdentity    = errors.New("account_identity_mismatch")
	ErrNotFound    = errors.New("account_not_found")
	ErrDisabled    = errors.New("account_disabled")
	ErrReauthorize = errors.New("account_reauthorization_required")
	ErrRefresh     = errors.New("account_refresh_failed")
	ErrLimit       = errors.New("account_limit")
)

type Account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Plan      string `json:"plan"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Service struct {
	db      *sql.DB
	queries *db.Queries
	vault   *vault.Vault
	now     func() time.Time
	// Refresh and administrator mutations share one owner; no model stream holds this lock.
	mu sync.Mutex
}

func New(connection *sql.DB, cipher *vault.Vault) *Service {
	return &Service{db: connection, queries: db.New(connection), vault: cipher, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]Account, error) {
	rows, err := s.queries.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Account, 0, len(rows))
	for _, row := range rows {
		result = append(result, Account(row))
	}
	return result, nil
}

func (s *Service) Import(ctx context.Context, name string, raw []byte, replaceID string) (Account, error) {
	credential, err := ParseCredential(raw)
	if err != nil {
		return Account{}, err
	}
	return s.save(ctx, name, credential, replaceID, "unverified")
}

func (s *Service) Authorize(ctx context.Context, name string, credential Credential, replaceID string) (Account, error) {
	return s.save(ctx, name, credential, replaceID, "ready")
}

func (s *Service) save(ctx context.Context, name string, credential Credential, replaceID, status string) (Account, error) {
	if err := credential.validate(); err != nil {
		return Account{}, err
	}
	var err error
	name, err = NormalizeName(name)
	if err != nil {
		return Account{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	if replaceID != "" {
		row, err := s.get(ctx, replaceID)
		if err != nil {
			return Account{}, err
		}
		// Existing continuations must never be rebound to a different upstream identity.
		if row.AccountID != credential.AccountID {
			return Account{}, ErrIdentity
		}
		if err := s.persist(ctx, replaceID, credential, status); err != nil {
			return Account{}, err
		}
		row.Email, row.Plan, row.Status, row.ExpiresAt, row.UpdatedAt = credential.Email, credential.Plan, status, credential.ExpiresAt, now
		return metadata(row), nil
	}
	random := make([]byte, 16)
	_, _ = rand.Read(random)
	id := hex.EncodeToString(random)
	plaintext, err := json.Marshal(credential)
	if err != nil {
		return Account{}, err
	}
	encrypted, err := s.vault.Seal(id, plaintext)
	if err != nil {
		return Account{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	count, err := queries.CountAccounts(ctx)
	if err != nil {
		return Account{}, err
	}
	if count >= 100 {
		return Account{}, ErrLimit
	}
	n, err := queries.CreateAccount(ctx, db.CreateAccountParams{ID: id, Name: name, AccountID: credential.AccountID, Email: credential.Email, Plan: credential.Plan, Status: status, Credential: encrypted, ExpiresAt: credential.ExpiresAt, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return Account{}, err
	}
	if n == 0 {
		return Account{}, ErrDuplicate
	}
	if err := tx.Commit(); err != nil {
		return Account{}, err
	}
	return Account{ID: id, Name: name, Email: credential.Email, Plan: credential.Plan, Enabled: true, Status: status, ExpiresAt: credential.ExpiresAt, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.get(ctx, id)
	if err != nil {
		return Account{}, err
	}
	row.Enabled = enabled
	row.UpdatedAt = s.now().Unix()
	if err := s.queries.SetAccountEnabled(ctx, db.SetAccountEnabledParams{ID: id, Enabled: enabled, UpdatedAt: row.UpdatedAt}); err != nil {
		return Account{}, err
	}
	return metadata(row), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.queries.DeleteAccount(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Prepare(ctx context.Context, id string, refresh func(context.Context, Credential) (Credential, error)) (Credential, error) {
	return s.prepare(ctx, id, "", refresh)
}

func (s *Service) RefreshAfterRejection(ctx context.Context, id, rejectedToken string, refresh func(context.Context, Credential) (Credential, error)) (Credential, error) {
	if rejectedToken == "" {
		return Credential{}, ErrInput
	}
	return s.prepare(ctx, id, rejectedToken, refresh)
}

func (s *Service) prepare(ctx context.Context, id, rejectedToken string, refresh func(context.Context, Credential) (Credential, error)) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.get(ctx, id)
	if err != nil {
		return Credential{}, err
	}
	if !row.Enabled {
		return Credential{}, ErrDisabled
	}
	if row.Status == "reauth_required" {
		return Credential{}, ErrReauthorize
	}
	credential, err := s.decrypt(row)
	if err != nil {
		return Credential{}, err
	}
	// Another request may already have rotated the rejected token while this one was waiting.
	if credential.ExpiresAt > s.now().Add(2*time.Minute).Unix() && (rejectedToken == "" || credential.AccessToken != rejectedToken) {
		return credential, nil
	}
	if refresh == nil {
		return Credential{}, ErrRefresh
	}
	// Network IO runs outside database transactions. Persist rotated tokens before publishing them to callers.
	updated, err := refresh(ctx, credential)
	if err != nil {
		if errors.Is(err, ErrReauthorize) {
			if saveErr := s.queries.SetAccountStatus(ctx, db.SetAccountStatusParams{ID: id, Status: "reauth_required", UpdatedAt: s.now().Unix()}); saveErr != nil {
				return Credential{}, saveErr
			}
			return Credential{}, ErrReauthorize
		}
		if ctx.Err() != nil {
			return Credential{}, ctx.Err()
		}
		return Credential{}, ErrRefresh
	}
	if updated.AccountID != row.AccountID {
		return Credential{}, ErrIdentity
	}
	if err := updated.validate(); err != nil {
		return Credential{}, err
	}
	if updated.ExpiresAt <= s.now().Add(2*time.Minute).Unix() {
		return Credential{}, ErrRefresh
	}
	if err := s.persist(ctx, id, updated, "ready"); err != nil {
		return Credential{}, err
	}
	return updated, nil
}

func (s *Service) persist(ctx context.Context, id string, c Credential, status string) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	encrypted, err := s.vault.Seal(id, raw)
	if err != nil {
		return err
	}
	return s.queries.UpdateAccountCredential(ctx, db.UpdateAccountCredentialParams{ID: id, Credential: encrypted, Email: c.Email, Plan: c.Plan, Status: status, ExpiresAt: c.ExpiresAt, UpdatedAt: s.now().Unix()})
}

func (s *Service) get(ctx context.Context, id string) (db.Account, error) {
	row, err := s.queries.GetAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func metadata(row db.Account) Account {
	return Account{ID: row.ID, Name: row.Name, Email: row.Email, Plan: row.Plan, Enabled: row.Enabled, Status: row.Status, ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", ErrInput
	}
	return name, nil
}

func (s *Service) Get(ctx context.Context, id string) (Account, error) {
	row, err := s.get(ctx, id)
	return metadata(row), err
}

func (s *Service) decrypt(row db.Account) (Credential, error) {
	plaintext, err := s.vault.Open(row.ID, row.Credential)
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	if json.Unmarshal(plaintext, &credential) != nil || credential.AccountID != row.AccountID {
		return Credential{}, vault.ErrDecrypt
	}
	return credential, nil
}

func (s *Service) RecordUse(ctx context.Context, id, usedToken string, accepted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	credential, err := s.decrypt(row)
	if err != nil {
		return err
	}
	// Results from older in-flight requests must not overwrite a later refresh or reauthorization.
	if credential.AccessToken != usedToken {
		return nil
	}
	status := "reauth_required"
	if accepted {
		status = "ready"
	}
	if row.Status == status {
		return nil
	}
	return s.queries.SetAccountStatus(ctx, db.SetAccountStatusParams{ID: id, Status: status, UpdatedAt: s.now().Unix()})
}

// Verify fails startup before serving traffic if the encryption key no longer matches persisted credentials.
func (s *Service) Verify(ctx context.Context) error {
	rows, err := s.queries.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, metadata := range rows {
		row, err := s.get(ctx, metadata.ID)
		if err != nil {
			return err
		}
		if _, err := s.decrypt(row); err != nil {
			return err
		}
	}
	return nil
}
