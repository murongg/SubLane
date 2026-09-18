package accounts

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

// Credential is internal secret material. HTTP responses must use Account instead.
type Credential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	AccountID    string `json:"account_id"`
	Email        string `json:"email,omitempty"`
	Plan         string `json:"plan,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
}

func ParseCredential(raw []byte) (Credential, error) {
	if len(raw) == 0 || len(raw) > 65536 {
		return Credential{}, ErrInput
	}
	var input struct {
		Credential
		Tokens  *Credential `json:"tokens"`
		Type    string      `json:"type"`
		Mode    string      `json:"auth_mode"`
		APIKey  string      `json:"OPENAI_API_KEY"`
		Expired string      `json:"expired"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return Credential{}, ErrInput
	}
	if input.APIKey != "" || input.Type != "" && input.Type != "codex" || input.Mode != "" && input.Mode != "chatgpt" {
		return Credential{}, ErrInput
	}
	credential := input.Credential
	if input.Tokens != nil {
		credential = *input.Tokens
	}
	// Token claims are used only as display/routing metadata, never as SubLane authentication or roles.
	claims := tokenClaims(credential.IDToken)
	if claims.AccountID != "" {
		if credential.AccountID != "" && credential.AccountID != claims.AccountID {
			return Credential{}, ErrIdentity
		}
		credential.AccountID = claims.AccountID
	}
	if claims.Email != "" {
		credential.Email = claims.Email
	}
	if claims.Plan != "" {
		credential.Plan = claims.Plan
	}
	if credential.ExpiresAt == 0 {
		credential.ExpiresAt = tokenClaims(credential.AccessToken).Expiry
	}
	if credential.ExpiresAt == 0 && input.Expired != "" {
		expiry, err := time.Parse(time.RFC3339, input.Expired)
		if err != nil {
			return Credential{}, ErrInput
		}
		credential.ExpiresAt = expiry.Unix()
	}
	if err := credential.validate(); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (c Credential) validate() error {
	if !validToken(c.AccessToken) || !validToken(c.RefreshToken) || len(c.IDToken) > 16384 || c.AccountID == "" || len(c.AccountID) > 256 || len(c.Email) > 320 || len(c.Plan) > 64 || c.ExpiresAt < 0 {
		return ErrInput
	}
	for _, value := range []string{c.AccountID, c.Email, c.Plan} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return ErrInput
		}
	}
	return nil
}

func validToken(value string) bool {
	return value != "" && len(value) <= 16384 && strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

type claims struct {
	AccountID, Email, Plan string
	Expiry                 int64
}

func tokenClaims(token string) claims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(raw) > 16384 {
		return claims{}
	}
	var payload struct {
		Email  string `json:"email"`
		Expiry int64  `json:"exp"`
		Auth   struct {
			AccountID string `json:"chatgpt_account_id"`
			Plan      string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return claims{}
	}
	return claims{AccountID: payload.Auth.AccountID, Email: payload.Email, Plan: payload.Auth.Plan, Expiry: payload.Expiry}
}
