# Account pool runtime

SubLane tracks model-request leases and transient account failures across every group sharing an account. The administrator can inspect scheduling state from **Accounts**, change a per-account limit through **Scheduling settings**, and inspect all completed calls under **Administration → All requests**. Every signed-in user, including the administrator, has a personal **General → Requests** view.

## Admission and affinity

Each account defaults to two simultaneous model requests, configurable from 1 to 8. The existing instance-wide limit of eight upstream operations remains. An account slot is reserved atomically with selection and retained until the response body closes, including HTTP/SSE, Chat Completions, compaction, and WebSocket turns. Cancellation closes/releases it exactly once. Reducing a limit allows existing requests to finish and stops additional admission until capacity is available.

A bound conversation keeps its account. Saturation returns `account_busy`; cooldown returns `account_cooling`, both with HTTP 429 and a bounded Retry-After header. No request automatically switches an existing conversation to another account. Group permissions and membership changes retain their existing guards.

Without `Session_id` or a nonempty `prompt_cache_key`, every call gets independent selection and a random upstream correlation ID. It does not read or create an affinity entry. New native selections rotate within each group among eligible accounts supporting the model, across providers. Explicit legacy prefixes limit selection to their provider. Existing explicit default-group conversations retain their historical digest.

The limit covers model requests; account verification, model discovery, and quota reads remain bounded by the existing global admission policy and do not consume model-request slots. These metadata operations do not clear model-request cooldowns.

## Cooldown and recovery

| Observation | Behavior |
| --- | --- |
| Upstream 429 | Immediately cool down; use integer/date Retry-After, bounded to 1–3600 seconds; default 60 seconds |
| 5xx, upstream/network failure, timeout or interrupted/invalid stream | Three consecutive logical failures trigger 30 seconds; subsequent failures back off exponentially up to five minutes |
| Valid completed or incomplete response | Reset failure state when it belongs to a request started after the last failure and cooldown has expired |
| Client cancellation or downstream write failure | Release capacity without penalizing the upstream account |
| Authentication/entitlement/input errors | Retain existing credential/request error behavior; do not treat these as transient server failures |

After cooldown expires, an account with no in-flight requests may admit one recovery probe. No background generation or provider polling occurs; a normal incoming request performs the probe. A successful probe restores normal concurrency; another failure cools the account again. Older in-flight success cannot clear newer failures, even within the same clock tick.

**Clear cooldown** also clears the failure streak and advances the runtime revision so older outcomes cannot reinstate it. It does not enable an account or repair authorization. Account settings/cooldowns are stored in SQLite. Native conversation bindings use the existing affinity table with an `auto` scope and survive restarts. In-flight counters and per-group cursors are process-local, and restart starts them empty. Run one SubLane process per database. Persistence failures are reported through sanitized application logs; in-memory cooldown still blocks admission if its write fails.

## Recent request records

Records cover authenticated model attempts that reach gateway scheduling, including account-capacity/cooldown rejection. Authentication failures, HTTP body/content-type validation failures, global admission rejection, model discovery, quota queries, and WebSocket prewarm do not create model-call records.

Stored metadata is limited to user/key/group/account IDs, provider, a bounded model identifier, protocol/operation, start time, duration, outcome, sanitized error code, upstream HTTP status, and reported input/output/cached token counts. Prompts, response bodies, raw provider errors, headers, IP addresses, session identifiers and credentials are not recorded. Missing token counts remain null. Counts describe individual observed responses; they are not billing or per-member token budget enforcement. Independent daily summaries and member request limits are described in [team controls](team-controls.md). A stream may have HTTP 200 yet finish with an error, so outcome and upstream status are distinct.

Personal records are filtered by the authenticated session user in SQL before pagination; a client-supplied user ID or scope cannot expand access. Subscription account IDs and names are cleared in the personal API response. Users can filter their own history by outcome, including calls through keys that were subsequently revoked. Administrators can separately inspect all records and filter by account and outcome. Both views are paginated in batches of 50. At most 5,000 records are retained in a seven-day window. Cleanup runs atomically when recording, and on reading the history; idle data is reclaimed on the next such operation. Monotonic record IDs are not reused after retention cleanup. Deleted account names render as deleted, without deleting historical metadata.

Personal and administrator request tables show an input-token cache hit rate beside the cached-token count: cached input tokens divided by total input tokens, formatted as a percentage with at most one decimal place. This measures the share of input tokens served from the upstream cache, not the share of requests that hit a cache. A reported zero cache count with positive input shows 0%; missing counts, zero input or a cache count exceeding total input show an em dash. The rate is derived from existing request metadata and adds no stored column or upstream request.

## API and storage

- `GET /api/accounts/runtime`: current scheduling state and model-request counts.
- `PATCH /api/accounts/{id}/limits`: set `max_concurrency` (integer 1–8).
- `POST /api/accounts/{id}/resume`: explicitly clear cooldown/failures.
- `GET /api/me/requests?cursor=0&outcome=`: current user’s request metadata, for either enabled role.
- `GET /api/requests?cursor=0&account_id=&outcome=`: administrator request metadata; outcome may be success, incomplete, error, canceled, or rejected.

Except for the explicitly personal history endpoint, these routes require an enabled administrator session; mutations retain origin/JSON protections. Migration `009_pool_runtime.sql` preserves existing credentials, groups and keys, adds the account limit, and creates runtime/history tables. Back up the stopped database and `credentials.key` together before upgrading.
