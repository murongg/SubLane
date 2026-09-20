# Model catalogs

SubLane discovers model IDs per subscription account and derives each group's catalog from its enabled accounts. Group allowlists remain administrator-owned policy; discovery never replaces them. A reported model describes upstream support, not remaining quota or a successful generation test.

## Synchronization and storage

Account import, OAuth completion and reauthorization start discovery after credentials have been saved. Reauthorization clears the old catalog even if the submitted token is unchanged. Existing accounts discover lazily when their catalog or group is read, or before their first model request. Re-enabling an account checks whether its saved catalog needs refreshing. **Verify connection** and **Refresh models** share the same discovery operation.

Migration `018_model_catalog.sql` adds a bounded JSON snapshot and lifecycle revision to `accounts`. A snapshot contains sorted, deduplicated native model IDs, the last successful observation time and a discovery-source marker. The marker includes the Codex client version and an adapter revision; upgrading that contract refreshes older or unmarked snapshots regardless of their age. Each account is limited to 512 model IDs of at most 128 ASCII characters; no upstream instructions, manifests or credentials are stored. A null snapshot means unknown support. A successfully retrieved empty list is a known empty catalog.

Snapshots are fresh for 15 minutes. A stale read returns the saved value and starts a shared background refresh. There is no periodic upstream polling while idle. Cold requests also join a refresh when the discovery-source marker changes. A failed upgrade refresh may retain previously known capabilities, but an obsolete catalog cannot prove an unlisted model unsupported. Manual refresh has a five-second cooldown; failed refreshes retain the last successful snapshot and back off for 30 seconds. Failed observations never change its timestamp. After 24 hours, the old snapshot remains visible to administrators but is excluded from advertised and routable models until refreshed. Refresh state and backoff are process-local; successful snapshots survive restarts.

At most two model discoveries run concurrently, within the existing eight-operation upstream admission limit. Concurrent readers of one account/revision join the same operation. A browser disconnect cancels its wait, while shared discovery uses the process context and a 45-second deadline. Shutdown cancels and joins workers before SQLite closes. Cold gateway discovery waits within an overall 45-second request budget; fresh catalogs require no upstream lookup.

Conditional writes reject results from an obsolete authorization, a disabled/deleted account or an earlier lifecycle revision. Discovery captures the runtime Codex version once; results from a version superseded during the network request are discarded, and snapshots retain the version actually sent upstream. Version policy and automatic stable-release synchronization are managed in [system settings](settings.md). Group grants, membership, policy and persisted snapshots are read again in one SQLite transaction before publishing a group result. Models removed from the group cannot leak through a refresh started before the edit.

## Group catalogs and routing

The effective group catalog is the deduplicated union of usable account snapshots, filtered by the group's exact model allowlist. Disabled and reauthorization-required accounts are excluded. Cooldown, concurrency and remaining subscription quota remain separate runtime concerns; a listed model does not promise an immediately available request slot.

Account, group and key catalogs and `GET /v1/models` expose native model IDs without adding provider prefixes. Identical IDs from multiple providers appear once; `owned_by` is `sublane` when more than one provider can serve the ID. Explicitly permitted thinking variants use the discovered base model for capability selection while retaining the complete requested ID for permission checks. Groups are capped at 4,096 distinct model IDs; oversized aggregate catalogs fail explicitly. Legacy explicitly qualified request IDs are still accepted, but are not advertised.

Before inference, discovery warms missing/stale catalogs for eligible providers in the group; an explicit legacy prefix narrows discovery to that provider. Compaction warms and selects Codex accounts only. The scheduler rechecks group policy and account snapshots in its selection transaction, filters out accounts that do not support the requested model, then applies existing concurrency, cooldown and rotation rules. A known unsupported model returns `404 model_not_available`; unknown or over-age capabilities return `503 model_catalog_unavailable` with a retry hint. Other eligible, known accounts may still serve a model when a peer's discovery fails.

Native requests select among permitted, available accounts reporting the model, including accounts from different providers. Account selection and reservation share the existing transaction and process lock. Existing native conversations keep a single account binding across providers; model changes, saturation, cooldown, removal or disablement never cause an automatic switch. An account that no longer supports the requested model returns `conversation_account_unavailable`. The existing `account_affinity` table stores native routing under an `auto` scope, with no new table or migration. A single legacy provider binding is adopted without changing the account or upstream session hash; multiple legacy bindings under the same session are ambiguous and require starting a new conversation. Explicit legacy-prefix requests retain their provider-specific bindings. No permanent API-key/account binding is added.

## UI and browser APIs

- **Accounts → Models** shows an account's last reported model IDs, observation time and synchronization state, with manual refresh.
- **Account groups → Models** shows the automatically derived, policy-filtered group catalog. The group editor continues to manage the allowlist separately.
- **API keys → Models** shows the current key's authorized group catalog. **Import into CC Switch** provides a searchable model picker using that same catalog.

| Endpoint | Access | Purpose |
| --- | --- | --- |
| `GET /api/accounts/{id}/models` | Administrator | Read account snapshot and trigger stale discovery |
| `POST /api/accounts/{id}/models/refresh` | Administrator, exact origin, empty JSON body | Refresh or join account discovery |
| `GET /api/groups/{id}/models` | Administrator | Read a derived group catalog |
| `GET /api/keys/{id}/models` | Enabled key owner | Read an active key's group catalog without decrypting its secret |
| `GET /v1/models` | Gateway bearer key | Return the standard model list for the key's group |

Personal catalog access rechecks key enablement, expiry, revocation and group access. Administrators cannot use the personal endpoint to inspect another owner's key. Responses contain model metadata and aggregate freshness counts, not subscription account IDs, names or credentials. A partial group response exposes that some account catalogs remain unknown instead of treating them as supporting every model.

## Verification boundary

Automated tests use synthetic accounts and local fake upstreams. They cover restart persistence, authoritative empty catalogs, failed-refresh retention/backoff, authorization and group-revocation races, cancellation/shutdown, mixed-account routing, group allowlists, thinking variants, conversation affinity, endpoint ownership, and the migration from schema 017. Frontend tests cover catalog states, keyboard model selection and stale responses after identity changes. Discovery does not probe model generation, and upstream catalogs may change between synchronization and an actual request.
