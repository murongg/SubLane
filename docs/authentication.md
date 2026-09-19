# Administrator authentication

SubLane supports one local administrator and administrator-created members. Both roles share local login and persisted sessions. See [member access](members.md) for permissions and account lifecycle. Personal gateway keys are available through the separate [API key workflow](api-keys.md). Invitation links, upstream subscription authorization, MFA, and password recovery are separate milestones.

## First-time setup

Start SubLane normally and open the UI. If the database has no administrator, `/setup` shows a welcome page explaining the setup flow. Select **Start setup** to open `/setup/admin`, then enter a username, password, and password confirmation. You can return to the welcome page before submitting. Creating the administrator signs you in and opens the workspace automatically. No separate initialization key is required.

The administrator form has its own URL so refreshing it keeps the current step. Both setup routes are unavailable after initialization: signed-out visitors go to login, and signed-in users go to their role-appropriate workspace. No onboarding state or account credentials are stored in browser storage.

Complete setup locally before opening the instance to other users. Until initialization completes, anyone who can access the setup page can create the first administrator. The default listener and Compose host port remain bound to loopback.

Usernames use 3–32 ASCII letters, numbers, underscores, or hyphens, starting with a letter or number. They are case-insensitive and stored in lowercase. New passwords use 8–20 Unicode characters and are not trimmed. Login continues to accept existing passwords up to the previous 128-character limit.

The administrator and first session are committed in one transaction. A guarded insert ensures concurrent setup requests cannot replace the first administrator. Initialization closes after the transaction succeeds, and restarting the service does not reopen it.

Existing administrator accounts and sessions are preserved. A leftover `setup.key` from an earlier development build is unused and may be deleted.

Deleting or replacing the database is not a password-recovery procedure. No password reset endpoint or recovery command is provided in this milestone.

## Sessions

- Login uses an opaque, cryptographically random 256-bit token in an HttpOnly, SameSite=Lax, path-root cookie.
- SQLite stores only the token's SHA-256 digest.
- Sessions expire after 12 hours, without sliding renewal.
- At most five sessions are retained per user. A new login replaces only that user's oldest excess session.
- Logout revokes the current session in SQLite before reporting success and clearing the cookie.
- Sessions survive a normal process restart.
- The browser stores theme and language preferences locally, but never session tokens.

The UI waits for the server's session state before showing protected pages. It rechecks state on focus and periodically, and returns to login when a protected API request reports an expired session. Logout cancels pending queries and clears private cached data. Background session checks also clear private data when the user ID or role changes.

## Reverse proxies and HTTPS

For a network deployment, terminate HTTPS at your reverse proxy and set the externally visible origin:

```sh
SUBLANE_PUBLIC_URL=https://sublane.example.com ./bin/sublane
```

The origin must contain only the scheme, host, and optional port. Paths, credentials, query parameters, and fragments are rejected. An HTTPS public origin enables Secure cookies even when the internal proxy connection uses HTTP.

Without this setting, same-origin checks use the request's Host and actual TLS state. Forwarded host, protocol, and peer headers are not implicitly trusted. Vite explicitly preserves the original Host on its API proxy for local development.

Keep the backend bound to loopback or a private network behind the proxy. Use HTTPS for network access; the local HTTP default is for development.

## Request protection and resource limits

All management routes under `/api/` require a valid enabled administrator session by default, except the four explicit authentication endpoints. Personal `/api/keys` and `/api/me/requests` endpoints are explicitly session-protected for both roles and enforce ownership. Members can read their own session state and sign out, but cannot access administrator management APIs. Static SPA assets remain public; rendering the application bundle does not grant access to management data.

Mutations require an exact same-origin Origin header, reject cross-site Fetch Metadata, and accept JSON only. Auth bodies are limited to 4 KiB and have a five-second read deadline.

Setup and login share a five-attempt-per-minute limit per connection peer and a sixty-attempt-per-minute global limit. Responses include Retry-After when throttled. Limiter state is bounded to 1,024 peers and is process-local, so restarting resets the counters. Behind a reverse proxy, clients share the proxy's peer budget until explicit trusted-proxy support is introduced.

Password derivation is limited to two concurrent operations. Argon2id uses 19 MiB, two iterations, and one lane, following the [OWASP password storage baseline](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). These limits are not a total-process memory guarantee.

## API

| Endpoint | Access | Result |
| --- | --- | --- |
| `GET /api/auth/state` | Public | `initialized` and the current user, or `null` |
| `POST /api/auth/setup` | Uninitialized instances only | Create the administrator and a session |
| `POST /api/auth/login` | Username/password | Create a new session |
| `POST /api/auth/logout` | Same-origin request | Revoke the current token and clear its cookie |
| `GET /api/system` | Administrator session | Instance status |
| `GET/POST /api/members` | Administrator session | Paginated list or member creation |
| `PATCH /api/members/{id}` | Administrator session | Enable or disable a member |
| `GET /healthz`, `GET /readyz` | Public | Liveness/readiness |

Setup and login accept `username` and `password`; the setup form checks password confirmation before submitting. Logout accepts an empty JSON object. Authenticated state responses include the persisted user ID, username, and `admin` or `member` role, and never contain tokens or password hashes. Error responses contain stable codes for localized frontend messages.

For command-line HTTP clients, supply the correct Origin header and JSON content type explicitly. Do not put credentials in shell history or share them in bug reports.
