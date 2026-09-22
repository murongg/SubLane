# Personal API keys

Start here if you already have a login and an authorized account pool. You do not need subscription credentials or knowledge of quota accounting.

## Create and use a key

1. Sign in and open **API keys**, then create a key.
2. Give it a recognizable name and select an authorized pool. Set an expiry only if needed.
3. Copy the key and configure your client with it, the instance address and a model ID available in that pool. Codex users can follow [client configuration](codex.md#client-configuration).
4. Send a message and confirm the result in **Requests**.

**Finished:** the client receives a response. Keep using this key; creating an allocation scheme is not required.

No pool to select? Ask the administrator to check [team access](members.md). Administrators setting up their first account should [create a pool first](quickstart.md#3-put-the-account-in-a-pool).

Next, see [Everyday use](usage.md). The sections below are for copying, changing, disabling or integrating keys when needed.

## Reference

Administrators and members open **API keys** in the General navigation group to manage their own gateway credentials. Each key has a name and a visible prefix. New keys can be copied immediately after creation or later using the copy icon beside their prefix. Metadata lists never return the secret, ciphertext or hash.

Keys use a `sl_` prefix followed by 32 cryptographically random bytes encoded as unpadded base64url. SQLite stores a SHA-256 digest for authentication alongside owner ID, name, display prefix, lifecycle times, enablement and optional expiry. New keys also have an AES-256-GCM encrypted full value in `api_keys.encrypted_secret`, using the existing instance-local `credentials.key`. Ciphertext is bound to the API-key domain, owner and record ID, and decrypted values must match their authentication hash. Gateway requests continue using hashes; they do not decrypt secrets.

## Ownership and lifecycle

- A user can have at most 20 non-revoked keys. The limit is checked transactionally.
- Lists are paginated in batches of 50, newest first. Each user sees, edits and revokes only their own keys, including administrators using the personal key endpoint.
- Keys default to enabled with no expiry. Set an expiry at creation, or edit the name, enablement and expiry later. The UI offers no expiry or 7/30/90 days, and preserves existing deadlines unless changed. Expiry is an absolute Unix timestamp (seconds); the key is invalid at that timestamp. The list displays status using the server clock.
- Pausing is reversible and preserves the secret and immutable group binding. Paused and expired keys still count toward the 20 non-revoked-key limit. An expired key needs an extended/removed deadline before it can work again.
- Revocation is permanent. The same key is rejected by subsequent gateway requests.
- Disabling a member suspends all its keys through an owner-status check on every gateway request. Re-enabling restores enabled, unexpired, non-revoked keys; revoked keys stay invalid.
- Last-used time records successful key authentication, including requests that subsequently return an unavailable-provider response. Writes are throttled to once per minute per key. It is not a model-usage or billing counter.

## Browser API

These routes require a valid enabled-user session cookie. Mutations retain the exact-origin, JSON-only, and body-size protections used by the rest of the browser API. The owner is taken from the session, never from a submitted user ID.

| Endpoint | Request | Response |
| --- | --- | --- |
| `GET /api/keys?cursor=0` | Optional cursor | `keys` metadata, `next_cursor` and `server_time` |
| `POST /api/keys` | `name` (1–64 characters), explicit `group_id` (or resource `scheme_id`) and nullable `expires_at` | `key` metadata and `secret` |
| `GET /api/keys/{id}/models` | No body | Authorized group model catalog for an active, owned key |
| `POST /api/keys/{id}/secret` | Empty JSON object | Full `secret`, for the owning user only |
| `PATCH /api/keys/{id}` | All three fields: `name`, `enabled`, nullable `expires_at` | Updated metadata; never a secret |
| `POST /api/keys/{id}/revoke` | Empty JSON object | Revoked key metadata |

Unknown or other-user key IDs return `404 api_key_not_found`. Editing a revoked key returns `409 api_key_revoked`. New or changed expiry values must be in the future; an unchanged past deadline can be retained while renaming an expired key. Omitting expiry on PATCH is rejected rather than silently removing a deadline. The non-revoked-key limit returns `409 api_key_limit`. Key names cannot contain control characters.

## Gateway authentication

Clients supply `Authorization: Bearer <api-key>` to `/v1/*`. Native `/v1/messages` additionally accepts `x-api-key`; Gemini `/v1beta` accepts `x-goog-api-key`, Bearer authentication, or a `key` query parameter. Conflicting credentials are rejected. See [native client protocols](protocols.md). Browser session cookies alone do not authenticate gateway requests. Conversely, API keys cannot access `/api/system`, `/api/members`, or `/api/keys`.

Missing, invalid, paused, expired, revoked, and suspended-owner keys return `401 invalid_api_key`. Valid keys can discover models and forward Responses HTTP/SSE/WebSocket, compaction, and Chat Completions requests through configured Codex subscriptions. Every WebSocket turn rechecks the key, expiry, member enablement, current group grants and model policy. In-flight admitted calls may finish after a change; new requests and turns must pass the current policy. Unknown gateway paths return JSON 404 responses after authentication. See [Codex gateway](codex.md) for configuration, limits, and remaining live-client validation.

Use the backend origin in development, or Vite's `/v1` proxy. The production binary serves the UI and gateway on the same origin. Use HTTPS for network deployments and avoid putting real keys into source control, shared logs, or shell history.

Migration `004_api_keys.sql` adds the key table and indexes. Migration `014_key_lifecycle.sql` makes existing keys enabled with no expiry, preserving hashes and group bindings. Manual changes are included in the administrator [audit log](audit.md). It does not change user IDs or browser sessions.

## Copying existing keys

The browser fetches a full value only after an explicit copy or import preparation action. The secret endpoint requires an enabled browser session, exact-origin validation and current ownership; an administrator cannot disclose another user's key. It uses POST with `Cache-Control: no-store`. Successful retrieval commits a metadata-only `key.reveal` audit record before returning the secret. Failed authenticated attempts use the existing sanitized audit path. This event records retrieval, not whether the operating system accepted the clipboard write or an external application imported the configuration.

The UI does not cache retrieved secrets in TanStack Query or mutation results, local/session storage, or web URLs. If clipboard access fails, a focused dialog allows manual selection/copy; closing the dialog or switching identities drops that temporary value. Pending requests are aborted on unmount, and late successes or failures from a previous identity are ignored. Creation responses retain the existing short-lived dialog behavior. The optional CC Switch handoff below includes the selected secret in a local application URI.

Paused and expired keys can still be copied by their owner; copying does not enable them or extend their expiry. Revocation atomically removes the encrypted value and disables subsequent retrieval. All keys expose a `copyable` metadata flag.

Migration `016_key_secrets.sql` introduced encrypted values; `017_consolidate.sql` moves them into `api_keys.encrypted_secret` without changing key IDs, hashes or ciphertext. Existing hash-only keys remain untouched. Those keys remain valid but report `copyable: false`; retrieval returns `409 api_key_not_copyable`. There is no automatic reconstruction or capture from gateway requests. Users who no longer have their old key can create a new one, update clients and revoke the old key.

Back up `credentials.key` alongside SQLite even when there are no upstream accounts. Startup must not replace a missing master key when encrypted gateway keys exist. Startup validates recoverable keys in bounded batches and fails if the encryption key or ciphertext is wrong. Existing upstream credential envelopes remain compatible.

## Import into CC Switch

Install [CC Switch](https://github.com/farion1231/cc-switch) on the computer running your browser. In **API keys**, choose the external-link icon on an active, recoverable key:

1. Review the prefilled configuration name, key's group and instance URL. Search and select a discovered model available to that group. The picker uses the owner-only model endpoint and does not retrieve the key secret.
2. Select **Prepare import** to retrieve your key through the same owner-only, audited endpoint used for copying.
3. Select **Open CC Switch**, then review and confirm the configuration in that application. Enable it there when ready.

This creates a **Codex** configuration using the current origin plus `/v1` and the Responses protocol. The [CC Switch V1 deep-link contract](https://github.com/farion1231/cc-switch/blob/main/docs/user-manual/zh/5-faq/5.3-deeplink.md) passes the configuration name, endpoint, model and full key through `ccswitch://v1/import`. No remote relay is used, and `enabled=false` avoids requesting an automatic provider switch. CC Switch manages the imported key in its own configuration/authentication storage; this differs from the environment-variable setup in [the manual client guide](codex.md#client-configuration).

Preparation and opening use separate clicks so the external application launch retains a browser user gesture. The prepared URI exists only in the mounted dialog state, never in a rendered link or persisted browser cache. Closing or editing the dialog discards it. Legacy hash-only, paused, expired, revoked and group-inaccessible keys cannot use this action. The picker uses the [derived group catalog](models.md); the gateway checks policy and account support again on each inference request. Import does not perform model generation.

SubLane cannot detect whether CC Switch is installed or whether its confirmation completed. Tests validate URI encoding, configuration fields, cancellation and identity isolation with synthetic keys; they do not establish an actual installed-app import or live model response. CC Switch import currently prepares Codex configurations only. For native Claude Messages or Gemini requests, use **Setup guide**, which selects the matching protocol, base URL and request example without retrieving a key secret.

## Allocation schemes

For a managed pool, select its team resource when creating a Key. The resource binding is immutable. Usage is shared per member and resource across keys and transports; joining several personnel teams gives separate resource balances. Team membership and resource availability are rechecked on every request and WebSocket turn. Existing unbound keys cannot access managed pools. See [team resources](allocations.md).
