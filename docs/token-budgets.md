# Token budgets

Administrators open **Members → More actions → Token budgets** to assign UTC calendar-day or calendar-month allowances. Members see their effective rules under **Usage**. No rule means unlimited tokens. Existing RPM, concurrency, group permissions and model allowlists still apply independently. Administrators are not assigned member budgets.

## Scope and counting

A rule belongs to one member and optionally selects an account group, an exact native model ID, or both. An empty model selects all models; group ID `0` selects all groups. All matching rules apply at once. A group rule is a separate allowance for that member, not a shared pool for everyone in the group. All API keys and transports share the same member counters. Native IDs and legacy provider-prefixed requests use the same model budget across providers; model variants remain distinct. Wildcards are not supported.

Usage is normalized input plus output tokens. Cached input already included in a provider's input total is not added twice. Claude cache reads/writes are included in normalized input, and Gemini thoughts are included in normalized output. This is a token allowance, not a monetary balance or a replacement for upstream subscription quota.

Each member may have 64 stable scope/period rules. The interface uses **M tokens** (1 M = 1,000,000 tokens) for limits, usage and settlements. Decimal inputs support six fractional digits, preserving single-token precision. The API and accounting ledger continue to use integer tokens; limits are positive integers up to 1,000,000,000,000. Saving the same scope/period updates the rule; it does not create a new allowance. Scope and period are immutable in the edit form. To use another scope, disable the old rule and add the new one. Disabled rules continue counting matching usage but stop enforcing their limit. Re-enabling, changing the limit, changing keys or removing/reinstating group membership never clears usage. There is no delete/reset-budget action.

New scopes begin tracking when created; existing statistics are not backfilled. Already admitted requests are not retroactively enrolled in new rules. The UI shows the creation and reset times. Limits reset at midnight UTC or the first day of the next UTC month. Requests are assigned to the calendar window containing their start time, including requests that finish in a later window. Day and month rules can coexist.

## Admission and uncertainty

All model operations, including HTTP/SSE, each WebSocket turn, Chat, compaction, native Messages and native Gemini, use the shared gateway admission path. Metadata reads and local WebSocket prewarm do not consume tokens. Every enabled matching rule must have remaining allowance and no unresolved usage. Quota rejection does not consume member RPM/concurrency or penalize upstream account health.

An already admitted request may finish, so simultaneous or long generations may take the recorded usage above its limit. This is **stop-new-requests enforcement**, not an exact maximum on generation. The gateway does not claim reliable output caps across subscription providers.

Before dispatch, matching scopes receive durable request entries. Completion settles them once in the same transaction as request history and statistics. Locally rejected requests and explicit upstream HTTP rejections settle without inventing token use. Complete reported usage is counted even when the response is incomplete because an output limit was reached.

A canceled, interrupted, ambiguous or missing-usage generation becomes **pending settlement**, retaining any observed tokens. Enabled rules affected by that request block new requests until an administrator settles it, including after the period resets. Other scopes remain usable. On process restart, unfinished durable entries conservatively become pending. One SubLane process must own a database, as with the existing gateway counters. Failed accounting commits stop gateway admission with `token_accounting_unavailable`; restore database access and restart to recover unfinished entries for settlement.

In the member's Token budgets dialog, an administrator enters the request's **total** input plus output tokens, including known tokens. The total cannot be below the already observed amount and must be at most 2,000,000,000. This single correction applies atomically to every original matching rule and original window, and is audited. Repeating the same correction is idempotent; conflicting or cross-owner corrections are rejected. Manual corrections remain budget adjustments, not fabricated provider reports in historical usage charts. Administrators must use available evidence when choosing the total; unknown usage is never automatically zeroed.

Unresolved entries are retained until settlement. Admission stops at 248 pending requests per member even if their limits are disabled, leaving room for the gateway's eight in-flight requests. Settled request entries and unused older period counters are cleaned after 90 days; pending entries and their counters survive cleanup. Rules themselves remain stable.

## API

| Route | Access and behavior |
| --- | --- |
| `GET /api/me/budgets` | Enabled session; identity always comes from the session |
| `GET /api/members/{id}/budgets` | Administrator; current rules/counters and up to 256 pending requests |
| `PUT /api/members/{id}/budgets` | Administrator; upsert one scope/period rule |
| `POST /api/members/{id}/budgets/settle` | Administrator; settle all entries for one pending request |

Rule input: `{"group_id":0,"model":"","period":"month","limit":1000000,"enabled":true}`. Settlement input: `{"request_id":"req_example","tokens":1200}`. Both mutations require same-origin JSON requests and write a management audit event in the domain transaction.

Budget exhaustion returns HTTP `429 token_quota_exceeded` with `Retry-After`, `X-Token-Budget-Id` and `X-Token-Budget-Reset`. Responses/Chat JSON and WebSocket errors include the matched scope in a `quota` field. Native protocol responses retain their native error envelopes and expose budget information through the headers. Pending usage returns `429 token_usage_pending` without a misleading automatic retry time. Accounting failures return `503 token_accounting_unavailable`.

Migration `021_token_budgets.sql` adds independent rules, period counters and request entries. Existing users, keys, request limits and usage statistics are preserved, with no budgets imposed on upgrade. The accounting ledger never depends on lossy model report labels or request-history retention.

## Allocation schemes

Team-resource keys use exactly their configured resource rule and bypass these legacy token rules. Unbound keys retain these rules only in unmanaged pools. See [team resources](https://sublane-website.vercel.app/docs/allocations).
