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

## Quota-aware selection

Codex model traffic also starts shared background quota refreshes for eligible pool accounts, without waiting for provider IO. At most two quota reads run concurrently within the existing eight upstream slots; duplicate reads coalesce, and existing cooldown/backoff rules apply. There is no idle polling loop. Claude and Antigravity remain eligible under the existing model/capacity policies because their quota readers are not implemented.

Selection reads the persisted normalized **main** subscription limit in the same short transaction as group policy and affinity. An explicit denial, a reached-limit flag, or a window at 100% used excludes the account from new selection only while the observation is fresh (two minutes, shortened by a reset). Unknown, stale, future-dated or already-reset data does not mean zero quota. Additional named limits are not assumed to map to models and cannot exclude an entire account. Low-but-positive quota does not alter round-robin weights. A failed refresh retains the last successful observation; it can affect scheduling only until its original freshness deadline, never indefinitely.

A bound conversation whose account is exhausted returns HTTP 429 `quota_exhausted`, with `Retry-After` bounded by the next freshness/reset check; this is a recheck hint, not a promise that quota will have recovered. It never moves to another account. Quota rejection does not increase transient failures, and **Clear cooldown** does not erase subscription quota. Accounts show a distinct quota-exhausted scheduling state.

Successful observations are saved before publication and survive restart. Quota rows carry the existing account lifecycle revision; disable/enable and reauthorization invalidate prior observations, and conditional writes reject older in-flight refreshes. Migration `019_request_diagnostics.sql` attaches revision metadata to existing quota rows and adds request diagnostics without introducing another table.

## Recent request records

Records cover authenticated model attempts that reach gateway scheduling, including account-capacity/cooldown rejection. Authentication failures, HTTP body/content-type validation failures, global admission rejection, model discovery, quota queries, and WebSocket prewarm do not create model-call records.

Stored metadata is limited to user/key/group/account IDs, provider, a bounded model identifier, protocol/operation, start time, duration, request correlation ID, nullable first output latency, outcome, sanitized error code, upstream HTTP status, and reported input/output/cached token counts. Prompts, response bodies, raw provider errors, headers, IP addresses, session identifiers and credentials are not recorded. Missing token counts remain null. Counts describe individual observed responses; they are not billing or per-member token budget enforcement. Independent daily summaries and member request limits are described in [team controls](team-controls.md). A stream may have HTTP 200 yet finish with an error, so outcome and upstream status are distinct.

Personal records are filtered by the authenticated session user in SQL before pagination; a client-supplied user ID or scope cannot expand access. Subscription account IDs and names are cleared in the personal API response. Users can filter their own history by outcome, exact model, API key, request ID and time interval, including calls through keys that were subsequently revoked. Administrators can separately filter by member and subscription account. Caller options derive only from retained history inside the same ownership scope. Both views are paginated in batches of 50. At most 5,000 records are retained in a seven-day window. Cleanup runs atomically when recording, and on reading the history; idle data is reclaimed on the next such operation. Monotonic record IDs are not reused after retention cleanup. Deleted account names render as deleted, without deleting historical metadata.

Personal and administrator request tables show an input-token cache hit rate beside the cached-token count: cached input tokens divided by total input tokens, formatted as a percentage with at most one decimal place. This measures the share of input tokens served from the upstream cache, not the share of requests that hit a cache. A reported zero cache count with positive input shows 0%; missing counts, zero input or a cache count exceeding total input show an em dash. The rate is derived from existing request metadata and adds no stored column or upstream request.

Each new model attempt receives a server-generated `req_…` identifier, ignoring caller-supplied IDs. HTTP responses return it in `X-Request-ID`; WebSocket turns add `request_id` to created/completed/incomplete events and gateway errors. The upstream response ID is unchanged. Records from before the upgrade have no correlation ID or first-output measurement. The request table shows a compact ID beneath each start time with a copy action for the complete value. Clicking the start time opens its details with the full ID.

`first_token_ms` measures gateway processing start to the first nonempty normalized text, reasoning, reasoning-summary or tool-argument delta, including scheduling and upstream waiting. It is not time to the first lifecycle event or client-rendered token. Compaction, streams without an observed delta and older records keep null; the UI calls it **First output**. Total duration still covers body closure.

## API and storage

- `GET /api/accounts/runtime`: current scheduling state and model-request counts.
- `PATCH /api/accounts/{id}/limits`: set `max_concurrency` (integer 1–8).
- `POST /api/accounts/{id}/resume`: explicitly clear cooldown/failures.
- `GET /api/me/requests?cursor=0&outcome=`: current user’s request metadata, for either enabled role.
- `GET /api/requests?cursor=0&account_id=&outcome=`: administrator request metadata; outcome may be success, incomplete, error, canceled, or rejected.

Both history endpoints also accept `model`, `request_id`, `key_id`, `from` (inclusive Unix seconds) and `until` (exclusive Unix seconds); the administrator endpoint accepts `user_id`. Retention remains seven days regardless of the requested range. `GET /api/me/requests/filters` and administrator-only `GET /api/requests/filters` return bounded caller/key choices with no subscription identity or secret values.

Except for the explicitly personal history endpoints, these routes require an enabled administrator session; mutations retain origin/JSON protections. Migration `009_pool_runtime.sql` preserves existing credentials, groups and keys, adds the account limit, and creates runtime/history tables. Back up the stopped database and `credentials.key` together before upgrading.
