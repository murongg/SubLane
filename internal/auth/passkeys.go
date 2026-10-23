package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/murongg/SubLane/internal/storage/db"
)

type passkeyUser struct {
	db.GetSecurityUserRow
	credentials []webauthn.Credential
	rows        []db.ListPasskeysRow
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.PasskeyHandle }
func (u passkeyUser) WebAuthnName() string                       { return u.Username }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.Username }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func passkeyRP(origin string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, ErrPasskeyUnavailable
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, ErrPasskeyUnavailable
	}
	rp, err := webauthn.New(&webauthn.Config{RPDisplayName: "SubLane", RPID: u.Hostname(), RPOrigins: []string{origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}})
	if err != nil {
		return nil, ErrPasskeyUnavailable
	}
	return rp, nil
}
func (s *Service) passkeyUser(ctx context.Context, userID int64) (passkeyUser, error) {
	user, err := s.queries.GetSecurityUser(ctx, userID)
	if err != nil {
		return passkeyUser{}, err
	}
	rows, err := s.queries.ListPasskeys(ctx, userID)
	if err != nil {
		return passkeyUser{}, err
	}
	u := passkeyUser{GetSecurityUserRow: user, rows: rows}
	for _, row := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(row.Credential), &c); err != nil {
			return u, err
		}
		u.credentials = append(u.credentials, c)
	}
	return u, nil
}
func (s *Service) saveChallenge(ctx context.Context, kind string, user db.GetSecurityUserRow, sessionToken string, data *webauthn.SessionData) (string, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if user.ID > 0 {
		current, err := s.securityUser(ctx, q, sessionToken)
		if err != nil {
			return "", err
		}
		if current.ID != user.ID || current.SecurityVersion != user.SecurityVersion {
			return "", ErrChallenge
		}
	}
	token, err := s.newChallenge(ctx, q, kind, user, sessionToken, string(encoded))
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}
func (s *Service) takeChallenge(ctx context.Context, token, kind, sessionToken string) (db.AuthChallenge, webauthn.SessionData, error) {
	var data webauthn.SessionData
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.AuthChallenge{}, data, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	c, err := s.challenge(ctx, q, token, kind, sessionToken)
	if err != nil {
		return c, data, err
	}
	// A failed ceremony also consumes its challenge; signatures must never be replayable.
	if err := q.DeleteAuthChallenge(ctx, c.TokenHash); err != nil {
		return c, data, err
	}
	if err := tx.Commit(); err != nil {
		return c, data, err
	}
	err = json.Unmarshal([]byte(c.Data), &data)
	return c, data, err
}
func (s *Service) BeginPasskeyRegistration(ctx context.Context, sessionToken, origin string, proof Proof) (*protocol.CredentialCreation, string, error) {
	rp, err := passkeyRP(origin)
	if err != nil {
		return nil, "", err
	}
	handle := make([]byte, 32)
	_, _ = rand.Read(handle)
	var verified db.GetSecurityUserRow
	err = s.withProof(ctx, sessionToken, proof, func(q *db.Queries, user db.GetSecurityUserRow) error {
		verified = user
		return q.SetPasskeyHandle(ctx, db.SetPasskeyHandleParams{UserID: user.ID, Handle: handle})
	})
	if err != nil {
		return nil, "", err
	}
	user, err := s.passkeyUser(ctx, verified.ID)
	if err != nil {
		return nil, "", err
	}
	if len(user.credentials) >= 10 {
		return nil, "", ErrInput
	}
	if user.SecurityVersion != verified.SecurityVersion {
		return nil, "", ErrChallenge
	}
	options, data, err := rp.BeginRegistration(user, webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()), webauthn.WithRegistrationOrigin(origin))
	if err != nil {
		return nil, "", ErrPasskey
	}
	challenge, err := s.saveChallenge(ctx, "register", user.GetSecurityUserRow, sessionToken, data)
	return options, challenge, err
}
func (s *Service) FinishPasskeyRegistration(ctx context.Context, sessionToken, challengeToken, origin, name string, response []byte) error {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 || len(response) > 65536 {
		return ErrInput
	}
	rp, err := passkeyRP(origin)
	if err != nil {
		return err
	}
	c, data, err := s.takeChallenge(ctx, challengeToken, "register", sessionToken)
	if err != nil {
		return err
	}
	user, err := s.passkeyUser(ctx, *c.UserID)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return ErrPasskey
	}
	credential, err := rp.CreateCredential(user, data, parsed)
	if err != nil {
		return ErrPasskey
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if len(encoded) > 32768 || len(credential.ID) > 1024 {
		return ErrInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	current, err := s.securityUser(ctx, q, sessionToken)
	if err != nil {
		return err
	}
	if current.ID != user.ID || current.SecurityVersion != c.SecurityVersion {
		return ErrChallenge
	}
	n, err := q.CreatePasskey(ctx, db.CreatePasskeyParams{UserID: user.ID, CredentialID: credential.ID, Credential: string(encoded), Name: name, CreatedAt: s.now().Unix()})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInput
	}
	if err := s.securityChanged(ctx, q, user.ID, sessionToken, "user.passkey_add"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) BeginPasskeyLogin(ctx context.Context, origin string) (*protocol.CredentialAssertion, string, error) {
	rp, err := passkeyRP(origin)
	if err != nil {
		return nil, "", err
	}
	options, data, err := rp.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired), webauthn.WithLoginOrigin(origin))
	if err != nil {
		return nil, "", ErrPasskey
	}
	token, err := s.saveChallenge(ctx, "login", db.GetSecurityUserRow{}, "", data)
	return options, token, err
}
func (s *Service) FinishPasskeyLogin(ctx context.Context, challengeToken, origin string, response []byte) (Session, error) {
	if len(response) > 65536 {
		return Session{}, ErrInput
	}
	rp, err := passkeyRP(origin)
	if err != nil {
		return Session{}, err
	}
	_, data, err := s.takeChallenge(ctx, challengeToken, "login", "")
	if err != nil {
		return Session{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return Session{}, ErrPasskey
	}
	var user passkeyUser
	_, credential, err := rp.ValidatePasskeyLogin(func(id, handle []byte) (webauthn.User, error) {
		userID, err := s.queries.FindPasskeyUser(ctx, handle)
		if err != nil {
			return nil, ErrPasskey
		}
		user, err = s.passkeyUser(ctx, userID)
		return user, err
	}, data, parsed)
	if err != nil || credential.Authenticator.CloneWarning {
		return Session{}, ErrPasskey
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return Session{}, err
	}
	previous := ""
	for _, row := range user.rows {
		if bytes.Equal(row.CredentialID, credential.ID) {
			previous = row.Credential
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	current, err := q.GetSecurityUser(ctx, user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrPasskey
	}
	if err != nil {
		return Session{}, err
	}
	if current.SecurityVersion != user.SecurityVersion {
		return Session{}, ErrPasskey
	}
	// Commit the updated authenticator counter and session together, rechecking credential ownership.
	now := s.now().Unix()
	n, err := q.UpdatePasskey(ctx, db.UpdatePasskeyParams{UserID: user.ID, CredentialID: credential.ID, Previous: previous, Credential: string(encoded), Now: &now})
	if err != nil {
		return Session{}, err
	}
	if n != 1 {
		return Session{}, ErrPasskey
	}
	session, err := s.createSession(ctx, tx, User{ID: user.ID}, user.PasswordHash)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}
func (s *Service) DeletePasskey(ctx context.Context, sessionToken string, id int64, proof Proof) error {
	if id <= 0 {
		return ErrInput
	}
	return s.withProof(ctx, sessionToken, proof, func(q *db.Queries, user db.GetSecurityUserRow) error {
		n, err := q.DeletePasskey(ctx, db.DeletePasskeyParams{ID: id, UserID: user.ID})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInput
		}
		return s.securityChanged(ctx, q, user.ID, sessionToken, "user.passkey_delete")
	})
}
