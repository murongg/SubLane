package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"image/png"
	"math"
	"strings"
	"time"

	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/storage/db"
	"github.com/murongg/SubLane/internal/vault"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var (
	ErrTwoFactorRequired  = errors.New("two_factor_required")
	ErrFactor             = errors.New("invalid_factor")
	ErrChallenge          = errors.New("challenge_expired")
	ErrPasskey            = errors.New("passkey_failed")
	ErrPasskeyUnavailable = errors.New("passkey_unavailable")
)

const challengeTTL = 5 * time.Minute

// FactorRateLimitError carries the account-wide retry interval across HTTP and workspace boundaries.
type FactorRateLimitError struct{ RetryAfter int }

func (*FactorRateLimitError) Error() string { return "rate_limited" }

type factorWindow struct {
	count int
	reset time.Time
}

func (s *Service) factorAttempt(userID int64) error {
	s.factorMu.Lock()
	defer s.factorMu.Unlock()
	now := s.now()
	for id, window := range s.factorFailures {
		if !now.Before(window.reset) {
			delete(s.factorFailures, id)
		}
	}
	window, exists := s.factorFailures[userID]
	if !exists {
		if len(s.factorFailures) >= 1024 {
			return ErrBusy
		}
		window.reset = now.Add(time.Minute)
	}
	if window.count >= 5 {
		return &FactorRateLimitError{RetryAfter: max(1, int(math.Ceil(window.reset.Sub(now).Seconds())))}
	}
	window.count++
	if s.factorFailures == nil {
		s.factorFailures = make(map[int64]factorWindow)
	}
	s.factorFailures[userID] = window
	return nil
}
func (s *Service) factorVerified(userID int64) {
	s.factorMu.Lock()
	defer s.factorMu.Unlock()
	delete(s.factorFailures, userID)
}

type Proof struct {
	Password string `json:"current_password"`
	Code     string `json:"code"`
}
type TOTPSetup struct {
	Secret    string `json:"secret"`
	URI       string `json:"uri"`
	QR        string `json:"qr"`
	Challenge string `json:"-"`
}
type Passkey struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at"`
}
type SecurityState struct {
	TOTPEnabled   bool      `json:"totp_enabled"`
	RecoveryCodes int64     `json:"recovery_codes"`
	Passkeys      []Passkey `json:"passkeys"`
}

func NewWithVault(connection *sql.DB, cipher *vault.Vault) (*Service, error) {
	s, err := New(connection)
	if err == nil {
		s.cipher = cipher
	}
	return s, err
}

func digest(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func (s *Service) securityUser(ctx context.Context, q *db.Queries, token string) (db.GetSecurityUserRow, error) {
	if !validToken(token) {
		return db.GetSecurityUserRow{}, ErrCredentials
	}
	user, err := q.GetSessionUser(ctx, db.GetSessionUserParams{TokenHash: digest(token), Now: s.now().Unix()})
	if errors.Is(err, sql.ErrNoRows) {
		return db.GetSecurityUserRow{}, ErrCredentials
	}
	if err != nil {
		return db.GetSecurityUserRow{}, err
	}
	return q.GetSecurityUser(ctx, user.ID)
}

func (s *Service) SecurityState(ctx context.Context, token string) (SecurityState, error) {
	user, err := s.securityUser(ctx, s.queries, token)
	state := SecurityState{Passkeys: []Passkey{}}
	if err != nil {
		return state, err
	}
	state.TOTPEnabled = len(user.TotpSecret) > 0
	state.RecoveryCodes, err = s.queries.CountRecoveryCodes(ctx, user.ID)
	if err != nil {
		return state, err
	}
	rows, err := s.queries.ListPasskeys(ctx, user.ID)
	for _, row := range rows {
		state.Passkeys = append(state.Passkeys, Passkey{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt})
	}
	return state, err
}

func (s *Service) withProof(ctx context.Context, token string, proof Proof, apply func(*db.Queries, db.GetSecurityUserRow) error) error {
	if !validPassword(proof.Password, 128) || len(proof.Code) > 64 {
		return ErrCurrentPassword
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.hashSlots }()
	verified, err := s.securityUser(ctx, s.queries, token)
	if err != nil {
		return err
	}
	if !verifyPassword(verified.PasswordHash, proof.Password) {
		return ErrCurrentPassword
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	user, err := s.securityUser(ctx, q, token)
	if err != nil {
		return err
	}
	if user.PasswordHash != verified.PasswordHash || user.SecurityVersion != verified.SecurityVersion {
		return ErrPasswordChanged
	}
	if err := s.verifyFactor(ctx, q, user, proof.Code); err != nil {
		return err
	}
	if err := apply(q, user); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) verifyFactor(ctx context.Context, q *db.Queries, user db.GetSecurityUserRow, code string) error {
	if len(user.TotpSecret) == 0 {
		return nil
	}
	if code == "" {
		return ErrTwoFactorRequired
	}
	if len(code) > 64 {
		return ErrFactor
	}
	if s.cipher == nil {
		return vault.ErrDecrypt
	}
	// This budget is account-wide; the HTTP peer limiter alone can be bypassed by rotating IPs.
	if err := s.factorAttempt(user.ID); err != nil {
		return err
	}
	secret, err := s.cipher.OpenTOTP(user.ID, user.TotpSecret)
	if err != nil {
		return err
	}
	if step, ok := totpStep(string(secret), code, s.now(), user.TotpStep); ok {
		if err := q.AdvanceTOTPStep(ctx, db.AdvanceTOTPStepParams{UserID: user.ID, Step: step}); err != nil {
			return err
		}
		s.factorVerified(user.ID)
		return nil
	}
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(normalized) != 16 {
		return ErrFactor
	}
	n, err := q.ConsumeRecoveryCode(ctx, db.ConsumeRecoveryCodeParams{UserID: user.ID, CodeHash: digest(normalized)})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrFactor
	}
	s.factorVerified(user.ID)
	return nil
}

func totpStep(secret, code string, now time.Time, previous int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for offset := int64(-1); offset <= 1; offset++ {
		step := now.Unix()/30 + offset
		if step <= previous {
			continue
		}
		expected, err := totp.GenerateCode(secret, time.Unix(step*30, 0))
		if err == nil && subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func (s *Service) newChallenge(ctx context.Context, q *db.Queries, kind string, user db.GetSecurityUserRow, sessionToken, data string) (string, error) {
	if err := q.DeleteExpiredAuthChallenges(ctx, s.now().Unix()); err != nil {
		return "", err
	}
	sessionHash := []byte{}
	var userID *int64
	if user.ID > 0 {
		userID = &user.ID
		sessionHash = digest(sessionToken)
		if err := q.DeleteSessionChallenges(ctx, db.DeleteSessionChallengesParams{SessionHash: sessionHash, Kind: kind}); err != nil {
			return "", err
		}
	}
	token := newToken()
	n, err := q.CreateAuthChallenge(ctx, db.CreateAuthChallengeParams{TokenHash: digest(token), Kind: kind, UserID: userID, SessionHash: sessionHash, SecurityVersion: user.SecurityVersion, Data: data, ExpiresAt: s.now().Add(challengeTTL).Unix()})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrBusy
	}
	return token, nil
}

func (s *Service) challenge(ctx context.Context, q *db.Queries, token, kind, sessionToken string) (db.AuthChallenge, error) {
	if !validToken(token) {
		return db.AuthChallenge{}, ErrChallenge
	}
	c, err := q.GetAuthChallenge(ctx, db.GetAuthChallengeParams{TokenHash: digest(token), Kind: kind, Now: s.now().Unix()})
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrChallenge
	}
	if err != nil {
		return c, err
	}
	if c.UserID != nil {
		user, err := s.securityUser(ctx, q, sessionToken)
		if err != nil {
			return c, ErrChallenge
		}
		if user.ID != *c.UserID || user.SecurityVersion != c.SecurityVersion || !bytes.Equal(c.SessionHash, digest(sessionToken)) {
			return c, ErrChallenge
		}
	}
	return c, nil
}

func (s *Service) securityChanged(ctx context.Context, q *db.Queries, userID int64, sessionToken, action string) error {
	if err := q.BumpSecurityVersion(ctx, userID); err != nil {
		return err
	}
	if err := q.DeleteOtherSessions(ctx, db.DeleteOtherSessionsParams{UserID: userID, CurrentHash: digest(sessionToken)}); err != nil {
		return err
	}
	if err := q.DeleteUserAuthChallenges(ctx, &userID); err != nil {
		return err
	}
	return audit.Record(ctx, q, action, "user", audit.ID(userID))
}

func (s *Service) BeginTOTP(ctx context.Context, sessionToken string, proof Proof) (TOTPSetup, error) {
	var setup TOTPSetup
	if s.cipher == nil {
		return setup, vault.ErrDecrypt
	}
	user, err := s.securityUser(ctx, s.queries, sessionToken)
	if err != nil {
		return setup, err
	}
	if len(user.TotpSecret) > 0 {
		return setup, ErrInput
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "SubLane", AccountName: user.Username})
	if err != nil {
		return setup, err
	}
	picture, err := key.Image(240, 240)
	if err != nil {
		return setup, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, picture); err != nil {
		return setup, err
	}
	encrypted, err := s.cipher.SealTOTP(user.ID, []byte(key.Secret()))
	if err != nil {
		return setup, err
	}
	setup = TOTPSetup{Secret: key.Secret(), URI: key.URL(), QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}
	err = s.withProof(ctx, sessionToken, proof, func(q *db.Queries, current db.GetSecurityUserRow) error {
		if len(current.TotpSecret) > 0 {
			return ErrInput
		}
		setup.Challenge, err = s.newChallenge(ctx, q, "totp", current, sessionToken, base64.StdEncoding.EncodeToString(encrypted))
		return err
	})
	return setup, err
}

func recoveryCodes() []string {
	codes := make([]string, 10)
	for i := range codes {
		raw := make([]byte, 10)
		_, _ = rand.Read(raw)
		code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		codes[i] = code[:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:]
	}
	return codes
}
func saveRecoveryCodes(ctx context.Context, q *db.Queries, userID int64, codes []string) error {
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return err
	}
	for _, code := range codes {
		if err := q.CreateRecoveryCode(ctx, db.CreateRecoveryCodeParams{UserID: userID, CodeHash: digest(strings.ReplaceAll(code, "-", ""))}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) EnableTOTP(ctx context.Context, sessionToken, challengeToken, code string) ([]string, error) {
	if s.cipher == nil {
		return nil, vault.ErrDecrypt
	}
	codes := recoveryCodes()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	c, err := s.challenge(ctx, q, challengeToken, "totp", sessionToken)
	if err != nil {
		return nil, err
	}
	encrypted, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return nil, err
	}
	secret, err := s.cipher.OpenTOTP(*c.UserID, encrypted)
	if err != nil {
		return nil, err
	}
	if err := s.factorAttempt(*c.UserID); err != nil {
		return nil, err
	}
	step, ok := totpStep(string(secret), code, s.now(), -1)
	if !ok {
		return nil, ErrFactor
	}
	s.factorVerified(*c.UserID)
	if err := q.EnableTOTP(ctx, db.EnableTOTPParams{UserID: *c.UserID, Secret: encrypted, Step: step}); err != nil {
		return nil, err
	}
	if err := saveRecoveryCodes(ctx, q, *c.UserID, codes); err != nil {
		return nil, err
	}
	if err := s.securityChanged(ctx, q, *c.UserID, sessionToken, "user.totp_enable"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}
func (s *Service) DisableTOTP(ctx context.Context, sessionToken string, proof Proof) error {
	return s.withProof(ctx, sessionToken, proof, func(q *db.Queries, user db.GetSecurityUserRow) error {
		if len(user.TotpSecret) == 0 {
			return ErrInput
		}
		if err := q.DisableTOTP(ctx, user.ID); err != nil {
			return err
		}
		if err := q.DeleteRecoveryCodes(ctx, user.ID); err != nil {
			return err
		}
		return s.securityChanged(ctx, q, user.ID, sessionToken, "user.totp_disable")
	})
}
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, sessionToken string, proof Proof) ([]string, error) {
	codes := recoveryCodes()
	err := s.withProof(ctx, sessionToken, proof, func(q *db.Queries, user db.GetSecurityUserRow) error {
		if len(user.TotpSecret) == 0 {
			return ErrInput
		}
		if err := saveRecoveryCodes(ctx, q, user.ID, codes); err != nil {
			return err
		}
		return s.securityChanged(ctx, q, user.ID, sessionToken, "user.recovery_codes")
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func VerifyFactors(ctx context.Context, connection *sql.DB, cipher *vault.Vault) error {
	q := db.New(connection)
	var after int64
	for {
		rows, err := q.ListTOTPSecrets(ctx, after)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			plain, err := cipher.OpenTOTP(row.ID, row.TotpSecret)
			if err != nil {
				return err
			}
			key, err := otp.NewKeyFromURL("otpauth://totp/SubLane:test?secret=" + string(plain))
			if err != nil || len(key.Secret()) != 32 {
				return vault.ErrDecrypt
			}
			if _, err := totp.GenerateCode(string(plain), time.Unix(0, 0)); err != nil {
				return vault.ErrDecrypt
			}
			after = row.ID
		}
	}
}
