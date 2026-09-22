# Account pools and access

For a first connection, follow [First request](quickstart.md). Use this page when adding accounts or separating access for different teams.

## Administrator workflow

1. Open **Account groups** and create a named group. Select its subscription accounts; empty groups may be configured before accounts are connected.
2. Open **Personnel teams**, add members and grant the shared groups they may use. A member sees the union of their enabled teams’ groups. Selected shared groups have no team allowance by default. For optional limits, expand **Advanced settings: usage limits** and add a [team resource](allocations.md). Administrators can use every enabled group; managed resources still require their own authorization.
3. Members select an authorized, enabled group when creating a key. Each key has one immutable group binding; revoke and recreate it to change groups.

Accounts in unmanaged groups may belong to several groups. A [team resource](allocations.md) requires an exclusive pool, and reserved account membership cannot be changed even while the resource is paused. Adding an account to a new group does not remove it from the default or any other pool. For exclusive access, remove the account from every pool that should no longer expose it. Account credentials and upstream subscription quota remain shared when an account is intentionally shared.

The first implementation supports up to 32 groups and 100 account associations per group. Groups can be renamed, edited, and disabled. Disabling preserves keys and history; there is no deletion action in this iteration. Dedicated team resources support share, amount or token allowances. Billing and routing-priority configuration remain outside this feature.

## Unassigned accounts and upgrades

Migration `008_groups.sql` creates the default group (ID 1), assigns existing accounts and members, and binds existing keys and conversation records to it without changing key hashes, encrypted credentials, or session hashes. Existing client configurations continue to work.

Migration `024_pools.sql` converts that former default into an ordinary pool while preserving its ID, accounts, model policy, key bindings and conversation hashes. It can be renamed, disabled or used for a team resource under the same rules as any other pool. A fresh installation has no pre-created pool.

New subscription accounts remain **Unassigned** until an administrator adds them to an account group. Importing does not grant member access. Open **Account groups**, create or edit a pool and select the accounts to assign. Members receive access through enabled personnel teams. Legacy individual grants are honored only before the instance creates any personnel team; once teams exist, those grants no longer authorize access.

## Request and conversation behavior

- Key creation checks member enablement, group enablement, and current group access in the same transaction as the key insert.
- Every gateway request and every new WebSocket turn rechecks the key's owner and group access. Disabling a group or removing team access blocks its keys from subsequent requests. Restoring the grant/group restores keys that have not been revoked.
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
| `GET /api/groups/members/{id}` | Read legacy individual grants, not effective team access |
| `PUT /api/groups/members/{id}` | Replace legacy grants; does not change team access once personnel teams exist |
| `GET /api/keys/groups` | Personal endpoint: only enabled authorized group IDs/names |

`POST /api/keys` accepts `name` and `group_id`; reserved resource keys also require `scheme_id`. Use `/api/teams` for current team access management and `/api/keys/groups` for effective personal choices. Creating an ordinary key requires an explicit `group_id`; omission is rejected. A resource key can instead select its pool through `scheme_id`. Existing keys retain their bindings. An explicit invalid ID is rejected. All accounts and groups must exist; unknown or duplicate IDs reject an entire pool/grant update, leaving the previous state intact.

Back up SQLite and `credentials.key` together before upgrades. Restoring a pre-groups executable requires the matching database backup. Automated verification uses synthetic identities and temporary databases, including migration preservation, defaults, key/grant enforcement, model isolation, atomic edits, and WebSocket permission changes.

## Model access

An administrator can enable **Limit allowed models** in the group editor. Enter up to 100 exact IDs, one per line, using the IDs returned by `/v1/models`. Native IDs allow that exact model across supporting providers in the group. Existing provider-qualified rules retain their original provider scope; updating SubLane does not rewrite or broaden them. Legacy qualified rules can still be submitted explicitly. Wildcards are not supported, and model variants must be listed explicitly.

Groups remain unrestricted by default, including newly discovered models. Enabling the allowlist with no entries denies all models. The API accepts an optional `model_policy: {"restricted": true, "models": ["synthetic-model"]}` on create/update. Omission preserves an existing policy; it does not reset access. Group responses include `restricted_models`; detail responses also include `allowed_models`.

Model discovery filters out models outside the allowlist and does not contact providers excluded entirely by the policy. Responses, Chat Completions, compaction, and every WebSocket turn (including local prewarm) check current policy before account selection or upstream work. A denied model returns `403 model_not_allowed`; it consumes no member rate/concurrency allowance and creates no affinity. Already admitted requests may finish. Migration `015_model_policy.sql` preserves existing unrestricted groups.

## Automatically discovered models

Open **Models** on a group to inspect the union of its eligible accounts’ saved model catalogs. The list follows account membership and applies the existing allowlist; it is not a second editable copy of that policy. New upstream models appear automatically only in unrestricted groups. Explicit restrictions, including an enabled empty deny-all list, remain unchanged by synchronization.

Inference uses account-specific catalog support before applying normal load/cooldown rules. Accounts with different subscription capabilities may share a group: a request is eligible only for accounts that reported its model. See [model catalogs](models.md) for freshness, failures and conversation affinity.
