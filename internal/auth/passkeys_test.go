package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
)

type syntheticAuthenticator struct {
	key    *ecdsa.PrivateKey
	id     []byte
	handle []byte
	count  uint32
}

func authenticator(t *testing.T) *syntheticAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &syntheticAuthenticator{key: key, id: []byte("synthetic-credential-id")}
}
func (a *syntheticAuthenticator) response(t *testing.T, challenge, origin string, registration bool) []byte {
	t.Helper()
	kind := "webauthn.get"
	if registration {
		kind = "webauthn.create"
	}
	client, _ := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": origin, "crossOrigin": false})
	rp := sha256.Sum256([]byte("example.test"))
	data := append([]byte{}, rp[:]...)
	flags := byte(5)
	if registration {
		flags |= 64
	}
	data = append(data, flags)
	a.count++
	data = binary.BigEndian.AppendUint32(data, a.count)
	encode := base64.RawURLEncoding.EncodeToString
	response := map[string]any{"clientDataJSON": encode(client)}
	if registration {
		public, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: a.key.X.FillBytes(make([]byte, 32)), -3: a.key.Y.FillBytes(make([]byte, 32))})
		data = append(data, make([]byte, 16)...)
		data = binary.BigEndian.AppendUint16(data, uint16(len(a.id)))
		data = append(data, a.id...)
		data = append(data, public...)
		attestation, _ := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": data})
		response["attestationObject"] = encode(attestation)
		response["transports"] = []string{"internal"}
	} else {
		hash := sha256.Sum256(client)
		signed := sha256.Sum256(append(data, hash[:]...))
		signature, err := ecdsa.SignASN1(rand.Reader, a.key, signed[:])
		if err != nil {
			t.Fatal(err)
		}
		response["authenticatorData"] = encode(data)
		response["signature"] = encode(signature)
		response["userHandle"] = encode(a.handle)
	}
	out, _ := json.Marshal(map[string]any{"id": encode(a.id), "rawId": encode(a.id), "type": "public-key", "response": response, "clientExtensionResults": map[string]any{}})
	return out
}
func registerSyntheticPasskey(t *testing.T, s *Service, owner Session, a *syntheticAuthenticator, proof Proof) {
	t.Helper()
	options, challenge, err := s.BeginPasskeyRegistration(context.Background(), owner.Token, "https://example.test", proof)
	if err != nil {
		t.Fatal(err)
	}
	a.handle = []byte(options.Response.User.ID.(protocol.URLEncodedBase64))
	data := a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", true)
	if err := s.FinishPasskeyRegistration(context.Background(), owner.Token, challenge, "https://example.test", "Synthetic device", data); err != nil {
		t.Fatal(err)
	}
}
func TestPasskeyRegistrationLoginAndRemoval(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	a := authenticator(t)
	registerSyntheticPasskey(t, s, owner, a, Proof{Password: syntheticPassword})
	security, err := s.SecurityState(ctx, owner.Token)
	if err != nil || len(security.Passkeys) != 1 {
		t.Fatalf("list: %+v %v", security, err)
	}
	options, challenge, err := s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	response := a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)
	session, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", response)
	if err != nil || session.User.ID != owner.User.ID {
		t.Fatalf("passwordless login: %v", err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", response); !errors.Is(err, ErrChallenge) {
		t.Fatalf("challenge replay: %v", err)
	}
	if err := s.DeletePasskey(ctx, owner.Token, security.Passkeys[0].ID, Proof{Password: "incorrect-pass"}); !errors.Is(err, ErrCurrentPassword) {
		t.Fatalf("delete without proof: %v", err)
	}
	if err := s.DeletePasskey(ctx, owner.Token, security.Passkeys[0].ID, Proof{Password: syntheticPassword}); err != nil {
		t.Fatal(err)
	}
	options, challenge, err = s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)); !errors.Is(err, ErrPasskey) {
		t.Fatalf("deleted passkey accepted: %v", err)
	}
}
func TestPasskeyOriginVerificationAndTOTPProof(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	_, codes := enrollTOTP(t, s, owner)
	if _, _, err := s.BeginPasskeyRegistration(ctx, owner.Token, "https://example.test", Proof{Password: syntheticPassword}); !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("TOTP bypass during registration: %v", err)
	}
	a := authenticator(t)
	registerSyntheticPasskey(t, s, owner, a, Proof{Password: syntheticPassword, Code: codes[0]})
	options, challenge, err := s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://evil.test", false)); !errors.Is(err, ErrPasskey) {
		t.Fatalf("wrong origin accepted: %v", err)
	}
	if _, _, err := s.BeginPasskeyLogin(ctx, "http://example.test"); !errors.Is(err, ErrPasskeyUnavailable) {
		t.Fatalf("insecure origin accepted: %v", err)
	}
	options, challenge, err = s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)); err != nil {
		t.Fatalf("verified passkey must satisfy 2FA: %v", err)
	}
	if err := s.RecoverAdministrator(ctx, "new password 42"); err != nil {
		t.Fatal(err)
	}
	options, challenge, err = s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)); !errors.Is(err, ErrPasskey) {
		t.Fatalf("recovered passkey accepted: %v", err)
	}
}

func TestPasskeyRegistrationIsSessionBoundAndRequiresDeviceVerification(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	other, err := s.Login(ctx, owner.User.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	options, challenge, err := s.BeginPasskeyRegistration(ctx, owner.Token, "https://example.test", Proof{Password: syntheticPassword})
	if err != nil {
		t.Fatal(err)
	}
	a := authenticator(t)
	a.handle = []byte(options.Response.User.ID.(protocol.URLEncodedBase64))
	response := a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", true)
	if err := s.FinishPasskeyRegistration(ctx, other.Token, challenge, "https://example.test", "Synthetic", response); !errors.Is(err, ErrChallenge) {
		t.Fatalf("cross-session registration accepted: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(response, &parsed); err != nil {
		t.Fatal(err)
	}
	authResponse := parsed["response"].(map[string]any)
	attestation, _ := base64.RawURLEncoding.DecodeString(authResponse["attestationObject"].(string))
	var object map[string]any
	if err := cbor.Unmarshal(attestation, &object); err != nil {
		t.Fatal(err)
	}
	data := object["authData"].([]byte)
	data[32] &= ^byte(4)
	object["authData"] = data
	attestation, _ = cbor.Marshal(object)
	authResponse["attestationObject"] = base64.RawURLEncoding.EncodeToString(attestation)
	response, _ = json.Marshal(parsed)
	if err := s.FinishPasskeyRegistration(ctx, owner.Token, challenge, "https://example.test", "Synthetic", response); !errors.Is(err, ErrPasskey) {
		t.Fatalf("registration without user verification: %v", err)
	}
}

func TestPasskeyLoginRejectsTamperingExpiredChallengesAndUnlinkedHandles(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	a := authenticator(t)
	registerSyntheticPasskey(t, s, owner, a, Proof{Password: syntheticPassword})
	for _, kind := range []string{"signature", "expired", "unlinked"} {
		t.Run(kind, func(t *testing.T) {
			options, challenge, err := s.BeginPasskeyLogin(ctx, "https://example.test")
			if err != nil {
				t.Fatal(err)
			}
			response := a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)
			expected := ErrPasskey
			switch kind {
			case "signature":
				var value map[string]any
				json.Unmarshal(response, &value)
				value["response"].(map[string]any)["signature"] = base64.RawURLEncoding.EncodeToString([]byte("synthetic-invalid-signature"))
				response, _ = json.Marshal(value)
			case "expired":
				s.now = func() time.Time { return time.Unix(1800000400, 0) }
				expected = ErrChallenge
			case "unlinked":
				if _, err := s.db.Exec("UPDATE users SET passkey_handle=NULL WHERE id=1"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", response); !errors.Is(err, expected) {
				t.Fatalf("accepted %s: %v", kind, err)
			}
		})
	}
}

func TestMemberFactorsSurviveRemoteResetAndDisabledUsersCannotLogin(t *testing.T) {
	s, owner := securityFixture(t)
	ctx := context.Background()
	member, err := s.CreateMember(ctx, "synthetic-member", syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Login(ctx, member.Username, syntheticPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, codes := enrollTOTP(t, s, session)
	a := authenticator(t)
	registerSyntheticPasskey(t, s, session, a, Proof{Password: syntheticPassword, Code: codes[0]})
	status, err := s.SecurityState(ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePasskey(ctx, owner.Token, status.Passkeys[0].ID, Proof{Password: syntheticPassword}); !errors.Is(err, ErrInput) {
		t.Fatalf("another user deleted passkey: %v", err)
	}
	if err := s.ResetMemberPassword(ctx, member.ID, "synthetic-new-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(ctx, member.Username, "synthetic-new-pass"); !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("remote reset bypassed factor: %v", err)
	}
	options, challenge, err := s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	login, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false))
	if err != nil || login.User.ID != member.ID {
		t.Fatalf("reset removed passkey: %v", err)
	}
	options, challenge, err = s.BeginPasskeyLogin(ctx, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMemberEnabled(ctx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishPasskeyLogin(ctx, challenge, "https://example.test", a.response(t, base64.RawURLEncoding.EncodeToString(options.Response.Challenge), "https://example.test", false)); !errors.Is(err, ErrPasskey) {
		t.Fatalf("disabled user logged in: %v", err)
	}
}
