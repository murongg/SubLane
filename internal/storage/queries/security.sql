-- name: GetSecurityUser :one
SELECT id, username, role, password_hash, totp_secret, totp_step, security_version, passkey_handle
FROM users WHERE id=sqlc.arg(id) AND enabled=1;

-- name: EnableTOTP :exec
UPDATE users SET totp_secret=sqlc.arg(secret), totp_step=sqlc.arg(step) WHERE id=sqlc.arg(user_id);

-- name: AdvanceTOTPStep :exec
UPDATE users SET totp_step=sqlc.arg(step) WHERE id=sqlc.arg(user_id);

-- name: DisableTOTP :exec
UPDATE users SET totp_secret=NULL, totp_step=-1 WHERE id=sqlc.arg(user_id);

-- name: BumpSecurityVersion :exec
UPDATE users SET security_version=security_version+1 WHERE id=sqlc.arg(user_id);

-- name: SetPasskeyHandle :exec
UPDATE users SET passkey_handle=sqlc.arg(handle) WHERE id=sqlc.arg(user_id) AND passkey_handle IS NULL;

-- name: FindPasskeyUser :one
SELECT id FROM users WHERE passkey_handle=sqlc.arg(handle) AND enabled=1;

-- name: CountTOTPSecrets :one
SELECT count(*) FROM users WHERE totp_secret IS NOT NULL;

-- name: ListTOTPSecrets :many
SELECT id, totp_secret FROM users WHERE totp_secret IS NOT NULL AND id>sqlc.arg(after_id) ORDER BY id LIMIT 100;

-- name: CountRecoveryCodes :one
SELECT count(*) FROM recovery_codes WHERE user_id=sqlc.arg(user_id);

-- name: CreateRecoveryCode :exec
INSERT INTO recovery_codes(user_id, code_hash) VALUES(sqlc.arg(user_id), sqlc.arg(code_hash));

-- name: ConsumeRecoveryCode :execrows
DELETE FROM recovery_codes WHERE user_id=sqlc.arg(user_id) AND code_hash=sqlc.arg(code_hash);

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes WHERE user_id=sqlc.arg(user_id);

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE user_id=sqlc.arg(user_id) AND token_hash!=sqlc.arg(current_hash);

-- name: ListPasskeys :many
SELECT id, name, credential_id, credential, created_at, last_used_at FROM passkeys WHERE user_id=sqlc.arg(user_id) ORDER BY id;

-- name: CreatePasskey :execrows
INSERT INTO passkeys(user_id, credential_id, credential, name, created_at)
SELECT sqlc.arg(user_id), sqlc.arg(credential_id), sqlc.arg(credential), sqlc.arg(name), sqlc.arg(created_at)
WHERE (SELECT count(*) FROM passkeys WHERE user_id=sqlc.arg(user_id))<10
ON CONFLICT(credential_id) DO NOTHING;

-- name: UpdatePasskey :execrows
UPDATE passkeys SET credential=sqlc.arg(credential), last_used_at=sqlc.arg(now)
WHERE credential_id=sqlc.arg(credential_id) AND user_id=sqlc.arg(user_id) AND credential=sqlc.arg(previous);

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id=sqlc.arg(id) AND user_id=sqlc.arg(user_id);

-- name: DeleteUserPasskeys :exec
DELETE FROM passkeys WHERE user_id=sqlc.arg(user_id);

-- name: DeleteExpiredAuthChallenges :exec
DELETE FROM auth_challenges WHERE expires_at<=sqlc.arg(now);

-- name: DeleteSessionChallenges :exec
DELETE FROM auth_challenges WHERE session_hash=sqlc.arg(session_hash) AND kind=sqlc.arg(kind);

-- name: CreateAuthChallenge :execrows
INSERT INTO auth_challenges(token_hash, kind, user_id, session_hash, security_version, data, expires_at)
SELECT sqlc.arg(token_hash), sqlc.arg(kind), sqlc.narg(user_id), sqlc.arg(session_hash), sqlc.arg(security_version), sqlc.arg(data), sqlc.arg(expires_at)
WHERE (SELECT count(*) FROM auth_challenges)<1000;

-- name: GetAuthChallenge :one
SELECT * FROM auth_challenges WHERE token_hash=sqlc.arg(token_hash) AND kind=sqlc.arg(kind) AND expires_at>sqlc.arg(now);

-- name: DeleteAuthChallenge :exec
DELETE FROM auth_challenges WHERE token_hash=sqlc.arg(token_hash);

-- name: DeleteUserAuthChallenges :exec
DELETE FROM auth_challenges WHERE user_id=sqlc.arg(user_id);
