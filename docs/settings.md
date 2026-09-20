# System settings

Administrators expand **Administration → System settings** in the sidebar, then select **Codex client version** (`/admin/settings/codex`). The menu opens automatically when visiting the child page directly; the previous `/admin/settings` address redirects there. These settings apply to the whole instance. Members cannot open either route or call its APIs. The header's **Preferences** shortcut remains personal: theme, language and password.

## Codex client version

The effective version is chosen in this order:

1. An explicit manual version, if configured.
2. The last synchronized stable version, provided it is at least the built-in default.
3. The built-in default (`0.155.1`).

Manual versions use `major.minor.patch` without a `v` prefix or prerelease suffix. Clear the input to return to automatic selection. A manual pin may select an older version deliberately; automatic synchronization never lowers the remembered stable version or the built-in baseline. The page shows the effective version, its source, the last synchronized version, and check timestamps.

**Automatically follow stable releases** is enabled by default. At startup, the service checks if due and otherwise resumes the saved schedule. Successful checks schedule the next attempt six hours later. Failed checks preserve the previous successful version and retry after fifteen minutes, respecting longer upstream rate limits. Turning automatic checks off cancels pending automatic work and keeps the known version. It does not prevent an explicit **Check now** action or override a manual pin.

Manual checks share an in-flight operation and have a sixty-second cooldown after a completed attempt; upstream rate limits or a storage failure can extend it. Disconnected callers do not cancel shared checks. Shutdown cancels and joins all checks before SQLite closes.

The service reads only the official `openai/codex` GitHub release metadata. It accepts stable `rust-v…` tags and excludes drafts and prereleases. The latest-release endpoint has a 2 MiB response bound; if it points to another component, one fallback page is limited to thirty releases and 16 MiB, decoded one release at a time. Checks have a thirty-second deadline, reject redirects, and send no subscription credentials or GitHub token. Network failures, invalid responses and persistence failures retain the prior effective version. No binary is downloaded or installed, and the CLIProxyAPI dependency is unchanged.

## Persistence and request behavior

One bounded JSON value at `settings['codex.version']` stores the configuration, last successful version and attempt timing. No new table or migration is required. Successful configuration changes and their `settings.update` audit event commit together. Automatic release observations do not create administrator audit events. The runtime version is published only after persistence succeeds.

Codex model discovery, quota metadata, and forwarding use the effective version. Each discovery request captures it once for the query, headers and persisted source marker. Changing the version invalidates catalog freshness: the next catalog read or model request triggers rediscovery. An in-flight result from a superseded version is discarded. Other providers and administrator model allowlists are unchanged. See [model catalogs](models.md) for stale-data and routing behavior.

## Browser API

| Endpoint | Purpose |
| --- | --- |
| `GET /api/settings/codex` | Read configuration, effective version and check state |
| `PATCH /api/settings/codex` | Save both `manual_version` (string) and `auto_sync` (boolean) |
| `POST /api/settings/codex/sync` | Check official releases with an empty JSON object |

All endpoints require an enabled administrator session. Mutations also require the exact origin. Invalid versions return 400; check cooldowns return 429 with `Retry-After`; release or persistence failures return sanitized 503 errors. A reported release version is compatibility metadata, not proof that every account supports every model in that release.
