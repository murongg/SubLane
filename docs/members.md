# Member accounts and access

SubLane supports one administrator and local member accounts. The administrator opens **Members** to create accounts and enable or disable them. New members use the same login page and the same username/password rules as the administrator. Creation assigns the `member` role on the server; clients cannot choose or change a role.

The member list is ordered newest first and paginated in batches of 50. Passwords and hashes are never returned. Share the initial credentials with the intended member privately. Public registration, invitation links, role promotion and account deletion are not implemented. Password changes, administrator member resets and local administrator recovery are described in [team controls](team-controls.md).

## Permissions

| Surface | Administrator | Member |
| --- | --- | --- |
| Homepage | Instance health and gateway readiness | Own account and client availability |
| Subscription-account management | Allowed | Denied |
| Member list and account creation/status | Allowed | Denied |
| System status API | Allowed | Denied |
| Personal API keys | Own keys only | Own keys only |
| Requests | Own history | Own history |
| All requests | All users’ history | Denied |
| Usage summaries | Own and team summaries | Own summaries |
| Request-limit policy | Configure members | Read own limits |
| Password changes | Own password; reset members | Own password |
| Appearance and language | Own preferences | Own preferences |
| Session status and sign-out | Own session | Own session |

Menus and direct-route access use the same frontend policy. The backend independently requires an enabled administrator session for management endpoints under `/api/`, with explicit ownership-protected exceptions for personal `/api/keys` and `/api/me/requests`. An authenticated member receives `403 forbidden`; missing, expired, or disabled sessions receive `401 unauthorized`. Changing browser state or calling a management URL directly does not grant permission.

Members do not request administrator data. Private cached query data is cleared on logout, sign-in, and when a background session check discovers an identity or role change in another tab. Unknown or missing roles are rejected at the frontend response boundary.

## Disabling accounts

Disabling a member updates account status and deletes that member’s sessions transactionally. Every subsequent authenticated request checks the persisted enabled status. The UI rechecks session state on focus and at regular intervals; it returns to login when it observes revocation. Re-enabling permits a fresh login and never revives old cookies. The administrator cannot be disabled through member-management APIs or the database status field.

Session limits apply per user. Member logins cannot evict administrator sessions. Session creation rechecks the account inside the transaction so concurrent disable operations cannot leave a usable new session behind.

## API

All endpoints below require an administrator session. Mutations use the existing same-origin JSON protections and 4 KiB body limit.

| Endpoint | Request | Result |
| --- | --- | --- |
| `GET /api/members?cursor=0` | Cursor from the previous page, or `0` | `members` and `next_cursor`; `0` means no next page |
| `POST /api/members` | `username`, `password` | New member with ID, username, role, enabled state, and creation timestamp |
| `PATCH /api/members/{id}` | `enabled` boolean | Updated member |

Usernames are case-insensitive. Duplicates return `409 username_taken`. Unknown member IDs and the administrator ID return `404 member_not_found` from status updates. Pagination is bounded; additional body fields such as `role` are rejected.

## Upgrade behavior

Migration `003_members.sql` copies the original administrator and every persisted session into `users` and `sessions`, preserving IDs, hashes, token digests, and expiration times. The entire migration is transactional. Existing sign-ins remain valid after upgrading, and existing passwords keep the previous login-length compatibility.

The migration replaces the old administrator-only tables. Older binaries cannot use the new schema; downgrading requires restoring a database backup from before migration.

Personal [API keys](api-keys.md) are available to both roles. Disabling a member suspends its non-revoked keys; re-enabling restores their use. Explicitly revoked keys never become valid again. Model forwarding and personal request history are available. Daily usage summaries and member request limits are available. Per-member token budgets are not implemented.
