# Personal API keys

Administrators and members open **API keys** in the General navigation group to manage their own gateway credentials. Each key has a name and a visible prefix. The complete secret is displayed only immediately after creation; copy it before closing the dialog. Metadata lists never return the secret or its hash.

Keys use a `sl_` prefix followed by 32 cryptographically random bytes encoded as unpadded base64url. SQLite stores only the SHA-256 digest, owner ID, name, display prefix, creation time, last authentication time, and revocation time. Creation and list responses are not cacheable. The UI drops one-time secrets when the dialog closes or the active user changes; it does not save them to browser storage.

## Ownership and lifecycle

- A user can have at most 20 non-revoked keys. The limit is checked transactionally.
- Lists are paginated in batches of 50, newest first. Each user sees and revokes only their own keys, including administrators using the personal key endpoint.
- Revocation is permanent. The same key is rejected by subsequent gateway requests.
- Disabling a member suspends all its keys through an owner-status check on every gateway request. Re-enabling restores non-revoked keys; revoked keys stay invalid.
- Last-used time records successful key authentication, including requests that subsequently return an unavailable-provider response. Writes are throttled to once per minute per key. It is not a model-usage or billing counter.

## Browser API

These routes require a valid enabled-user session cookie. Mutations retain the exact-origin, JSON-only, and body-size protections used by the rest of the browser API. The owner is taken from the session, never from a submitted user ID.

| Endpoint | Request | Response |
| --- | --- | --- |
| `GET /api/keys?cursor=0` | Optional cursor | `keys` metadata and `next_cursor` |
| `POST /api/keys` | `name`, 1–64 characters | `key` metadata and one-time `secret` |
| `POST /api/keys/{id}/revoke` | Empty JSON object | Revoked key metadata |

Unknown or other-user key IDs return `404 api_key_not_found`. The active-key limit returns `409 api_key_limit`. Key names cannot contain control characters.

## Gateway authentication

Clients supply `Authorization: Bearer <api-key>` to `/v1/*`. Browser session cookies alone do not authenticate gateway requests. Conversely, API keys cannot access `/api/system`, `/api/members`, or `/api/keys`.

Missing, invalid, revoked, and suspended-owner keys return `401 invalid_api_key`. Valid keys can discover models and forward Responses HTTP/SSE/WebSocket, compaction, and Chat Completions requests through configured Codex subscriptions. Every WebSocket turn rechecks the key and member enablement. Unknown gateway paths return JSON 404 responses after authentication. See [Codex gateway](codex.md) for configuration, limits, and remaining live-client validation.

Use the backend origin in development, or Vite's `/v1` proxy. The production binary serves the UI and gateway on the same origin. Use HTTPS for network deployments and avoid putting real keys into source control, shared logs, or shell history.

Migration `004_api_keys.sql` adds the key table and indexes. It does not change user IDs or browser sessions.
