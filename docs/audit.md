# Management audit

Administrators open **Administration → Audit log** to inspect changes to gateway keys, groups, members, subscription accounts and access settings. Members cannot access the page or API. Request history remains separate: it describes model calls rather than management changes.

## Recorded actions

- Personal key creation, metadata changes, full-key retrieval and permanent revocation. Full-key retrieval logs only actor/target metadata, never the value.
- Group creation, pool/model-policy changes and member group grants.
- Member creation, enable/disable, rate/concurrency changes and password reset.
- Personal password changes and local administrator password recovery.
- Subscription account import/authorization, reauthorization, enable/disable, deletion, concurrency changes and manual cooldown reset.

Each event contains the authenticated actor's ID, username/role snapshot, source (`user` or `local`), a fixed action, resource type/ID, outcome and timestamp. Failed authenticated attempts to recognized mutation endpoints include the HTTP status. Invalid/unrecognized target paths, origin rejection, public setup/login/logout, OAuth initiation/cancellation, read-only checks, automatic credential refresh and runtime bookkeeping are outside this log. Initial data and past actions are not backfilled.

There are no request/response bodies, credentials, password hashes, full keys, OAuth parameters, raw URLs/errors, account email addresses, or before/after secret snapshots. Account IDs are opaque SubLane record IDs; deleted targets remain identifiable by ID. Failed account import/OAuth completion attempts omit the target ID because it cannot be obtained safely from an untrusted body.

Successful events commit in the same transaction as the mutation. A failed audit write rolls back the change. Domain background operations without a user actor do not invent one. Authenticated HTTP failures are best-effort, with a bounded persistence timeout; persistence failure emits a fixed server diagnostic without request data. Local recovery records `local-cli` as the actor rather than falsely attributing it to the administrator.

## Storage and API

Migration `013_audit.sql` adds a SQLite table with monotonic event IDs. Keep the latest 90 days, capped at 10,000 events; pruning runs on inserts and list reads. There is no deletion API, external logging service or background polling dependency. Retention is a bounded operational record, not an immutable external archive.

`GET /api/audit?cursor=0&resource=&outcome=` requires an enabled administrator browser session and returns `events` plus `next_cursor`. Pages contain at most 50 entries, newest first. Resources are `key`, `group`, `member`, `user`, or `account`; outcomes are `success` or `failure`; an empty filter selects all. Invalid filters return HTTP 400. The frontend loads records only on navigation, explicit refresh, filter changes or pagination.
