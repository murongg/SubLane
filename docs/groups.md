# Account groups

Groups define which subscription accounts a personal gateway key may use. Roles continue to control management access. Groups can contain multiple providers; the requested model prefix selects the provider inside that pool.

## Default group and migration

Migration `008_groups.sql` creates the default group (ID 1), assigns existing accounts and members, and binds existing keys and conversation records to it without changing key hashes, encrypted credentials, or session hashes. Existing client configurations continue to work.

New subscription accounts and local users join the default group within their creation transaction. The default group cannot be renamed or disabled. Administrators may edit its account pool and revoke a member's access to it. Default membership is a compatibility choice, not a permanent authorization bypass.

## Administrator workflow

1. Open **Account groups** and create a named group. Select its subscription accounts; empty groups may be configured before accounts are connected.
2. Open **Members → Group access** and choose the groups a member may use. Members may have no group access. Administrators can use every enabled group.
3. Members select an authorized, enabled group when creating a key. Each key has one immutable group binding; revoke and recreate it to change groups.

Accounts may belong to several groups. Adding an account to a new group does not remove it from the default or any other pool. For exclusive access, remove the account from every pool that should no longer expose it. Account credentials and upstream subscription quota remain shared when an account is intentionally shared.

The first implementation supports up to 32 groups and 100 account associations per group. Groups can be renamed, edited, and disabled. Disabling preserves keys and history; there is no deletion action in this iteration. Per-group billing, quotas, model allowlists, and routing priorities are not implemented.

## Request and conversation behavior

- Key creation checks member enablement, group enablement, and current group access in the same transaction as the key insert.
- Every gateway request and every new WebSocket turn rechecks the key's owner and group access. Disabling a group or removing a member grant blocks its keys from subsequent requests. Restoring the grant/group restores keys that have not been revoked.
- Model discovery only uses eligible accounts in the key's group. Discovery does not create conversation bindings.
- Conversation affinity is persisted by member, group, session, and provider. Existing default-group sessions retain their previous upstream cache/session digest.
- Removing a bound account from a pool blocks that conversation instead of silently choosing another account. Start a new conversation to intentionally use a different account. Disabling or deleting an upstream account also blocks future use.
- A request already admitted may complete; permission changes apply to later requests/turns. Keys and model requests cannot override their group through a supplied owner/group field.
- Personal connection readiness considers only the member's authorized pools. Key metadata shows the bound group and whether group access is currently blocked.

## HTTP endpoints

All management endpoints require an enabled administrator browser session. Mutations require the existing same-origin and JSON protections.

| Route | Purpose |
| --- | --- |
| `GET /api/groups` | List groups and account/member counts |
| `POST /api/groups` | Create a group with `name`, `enabled`, and `account_ids` |
| `GET /api/groups/{id}` | Read a group and its account IDs |
| `PATCH /api/groups/{id}` | Atomically replace its name, enablement, and account selection |
| `GET /api/groups/members/{id}` | Read a member's explicit grants |
| `PUT /api/groups/members/{id}` | Atomically replace grants with `group_ids`; an empty array revokes all |
| `GET /api/keys/groups` | Personal endpoint: only enabled authorized group IDs/names |

`POST /api/keys` accepts `name` and `group_id`. Omitting `group_id` selects group 1 for compatibility, but still checks permission. An explicit invalid ID is rejected. All accounts and groups must exist; unknown or duplicate IDs reject an entire pool/grant update, leaving the previous state intact.

Back up SQLite and `credentials.key` together before upgrades. Restoring a pre-groups executable requires the matching database backup. Automated verification uses synthetic identities and temporary databases, including migration preservation, defaults, key/grant enforcement, model isolation, atomic edits, and WebSocket permission changes.
