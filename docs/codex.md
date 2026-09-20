# Codex subscriptions and gateway

SubLane uses CLIProxyAPI v7.3.7’s public executors and translators behind `internal/upstream`. SubLane owns authorization, encrypted credential storage, refresh, account selection, and the public gateway. The SDK runs as a private executor registry with no live credentials, a no-op watcher, blocked loopback routes, and automatic refresh disabled. See [architecture](architecture.md) for the execution boundary and [provider setup](providers.md) for Claude and Antigravity. This guide’s authorization and quota details describe Codex.

## Add an account

Sign in as the administrator, open **Accounts**, and choose **Add account**. Members cannot access subscription management endpoints or credentials.

- **Browser authorization:** name the account, start authorization, and open the OpenAI link. After signing in, copy the complete `http://localhost:1455/auth/callback?...` address from the browser address bar and paste it into SubLane. A localhost connection error at this point is expected: SubLane intentionally does not open another callback listener. This manual-return flow works for local and remote deployments.
- **Import auth.json:** choose a Codex subscription credential file or paste its JSON. Both the CLI's nested `tokens` structure and the flat Codex token format are accepted. Official API keys are rejected. SubLane never scans local client directories; it reads only the content explicitly submitted by the administrator. Imported endpoint/proxy settings are discarded.

Account authorization/import starts model discovery automatically. Use **Verify connection** to refresh and persist the account's model catalog, or open **Models** to inspect it. Imported credentials show **Not verified** until successful use. The account menu provides enable/disable, reauthorization, and removal. Reauthorization must retain the same upstream account identity. Removing an account deletes its stored credentials from SubLane; it does not revoke the ChatGPT login itself.

Codex model discovery sends the instance's effective client version in the catalog query and request headers. Administrators can configure it in **Administration → System settings**: an explicit version takes priority over the last synchronized official stable release, with `0.155.1` as the built-in fallback. Automatic release checks default to every six hours. This changes protocol metadata without installing Codex or updating the embedded SDK; see [system settings](settings.md).

The upstream catalog depends on that version: an obsolete version can return HTTP 200 with only hidden models or a partial visible catalog, producing an empty or incomplete `/v1/models` list after visibility filtering. Persisted catalogs include the actual discovery version, so changing the effective version triggers refresh on the next catalog read or model request even when their timestamps remain fresh. Changing group permissions does not resolve this compatibility issue.

OAuth attempts use PKCE, 256-bit random state, a ten-minute expiry, one active attempt per administrator browser session, and a maximum of eight pending attempts. The callback is bound to the session that started it and consumed before exchanging its code. Closing the form requests cancellation; expiry is the fallback after a lost connection. Pending OAuth state is held only in memory.

## Subscription usage

The accounts page loads each enabled account's upstream usage limits and offers **Refresh usage**. It shows the actual window duration, remaining percentage, reset countdown, and snapshot time, including additional model limits when reported. Missing limits or percentages remain unknown; failures are separate from quota exhaustion. A failed refresh retains the previous snapshot with an explicit warning. Reaching a reset timestamp does not fabricate replenished quota; refresh to retrieve the new state.

The usage adapter reads `https://chatgpt.com/backend-api/wham/usage`, matching the [official Codex backend client](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs). It uses the selected account's credential, the existing refresh/retry owner, bounded gateway admission, a 20-second deadline, and a 128 KiB response bound. Only normalized quota fields are returned; raw upstream metadata and credentials are excluded. This backend contract may evolve independently of SubLane.

Migration `006_usage.sql` stores one bounded, normalized snapshot per account in SQLite. Snapshots include the last successful observation time and survive process restarts; deleting an account cascades to its snapshot. The process keeps at most 100 snapshot entries in memory and lazily reloads them from SQLite. No credentials or raw provider responses are stored in this table.

`GET /api/accounts/{id}/usage` returns a fresh snapshot for up to two minutes (or until a reported upcoming reset, if sooner). An expired snapshot is returned immediately with `stale` and `refreshing` flags while a background refresh runs. A first fetch without a snapshot waits for the result. `POST /api/accounts/{id}/usage/refresh` bypasses freshness and waits for the shared refresh, while honoring a five-second cooldown. Both routes require an enabled administrator session; POST also requires the existing origin/JSON checks.

Concurrent callers for the same account share one upstream operation. Refreshes use the existing eight-operation admission bound and a 45-second overall deadline, survive an individual browser disconnect, and are canceled/awaited during process shutdown. A failed refresh preserves the previous values and timestamp, marks them stale, and backs off for 30 seconds (or a longer valid upstream Retry-After, up to one hour). Admission saturation uses a five-second retry delay. Cache hits still check account existence and enablement. Transient failure/backoff state is process-local; the last successful snapshot is durable.

The browser retains its one-minute query freshness period. It polls the local snapshot endpoint once per second only while a refresh is active, stopping on completion or a network error. There is no periodic provider polling when the page is idle. Observation timestamps and response server time are separate so a restored snapshot does not restart the reset countdown. Disabled accounts are not queried. Model traffic also warms the shared quota cache without waiting. Fresh main-limit exhaustion excludes an account from new selection; bound conversations return a quota-specific rejection. Upstream usage limits describe the subscription, not per-member accounting or token budgets. See [quota-aware selection](pool-runtime.md#quota-aware-selection).

## Credential storage and refresh

Migration `005_accounts.sql` adds subscription accounts and bounded account-affinity records. Access, refresh, and ID tokens are encrypted with AES-256-GCM and account-bound authenticated data. The instance creates a private 32-byte `credentials.key` file next to its database. The key is not a setup password and requires no manual entry.

Back up the database and `credentials.key` together after stopping the instance. An existing account database will not start with a missing, invalid, or mismatched key. Do not replace the key to recover access. Encryption protects credential contents in a database-only copy; access to both the database and the key permits decryption.

SubLane is the sole refresh owner. It serializes credential refresh with administrator changes and persists rotated tokens before publishing them to requests. Tokens near expiry refresh before use. An upstream 401 permits one refresh and one retry on the same account; concurrent rejections of an old token reuse the already refreshed credential. Older in-flight results cannot invalidate a later refresh or reauthorization.

## Client configuration

Create a personal API key in **API keys**. Use the instance origin plus `/v1` as the base URL. Configure a model ID returned by `GET /v1/models`.

Set `SUBLANE_API_KEY` in the client process environment. Merge the following settings into the user-level `~/.codex/config.toml`, replacing matching existing values:

```toml
model = "<model-id-from-your-account>"
model_provider = "sublane"

[model_providers.sublane]
name = "SubLane"
base_url = "http://127.0.0.1:8080/v1"
env_key = "SUBLANE_API_KEY"
wire_api = "responses"
requires_openai_auth = false
supports_websockets = true
```

Use your instance's actual URL, with HTTPS for network deployments. For HTTP/SSE transport, set `supports_websockets = false`. Codex CLI and desktop must read the intended user-level configuration. A desktop app already running does not automatically inherit environment variables exported in another terminal; restart it in an environment where the key is available.

The API keys page provides a configuration snippet without embedding or retaining the secret. It also offers an optional [CC Switch import](api-keys.md#import-into-cc-switch) that passes the chosen key to the locally installed application after explicit preparation. CC Switch generates its own Codex authentication-file configuration; it does not preserve the environment-variable or WebSocket settings in the manual snippet above. See the official [advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced) and [authentication](https://learn.chatgpt.com/docs/auth) guidance for client configuration and credential storage.

Account catalogs are cached and survive restarts. Discovery, group aggregation and model-aware account selection are described in [model catalogs](models.md).

## Gateway contract

All gateway routes require a SubLane bearer key, independently of browser cookies:

| Route | Behavior |
| --- | --- |
| `GET /v1/models` | Return deduplicated native model IDs from persisted account catalogs within the group and its policy |
| `POST /v1/responses` | Responses JSON or SSE using the requested `stream` setting |
| `POST /v1/responses/compact` | Non-streaming Codex compaction |
| `GET /v1/responses` with WebSocket upgrade | Responses turns, local prewarm, incremental input reconstruction, and compact transcript replacement |
| `POST /v1/chat/completions` | SDK translation to/from the selected provider protocol |

HTTP requests must include complete input. HTTP `previous_response_id` is rejected instead of silently losing context because the Codex HTTP backend is stateless. WebSocket continuations use bounded, connection-local history; unknown continuation IDs require full transcript input. Every WebSocket turn rechecks the key and member enablement. A revoked key cannot start a new turn on an existing connection.

Account affinity is scoped to the member, group and client session/prompt-cache key, retained for 24 hours of activity, and capped at 4,096 bindings. Native requests use one binding across providers; explicit legacy-prefix requests retain provider-specific bindings. New sessions distribute across eligible accounts reporting the requested model within their group, respecting per-account concurrency and cooldown. Sessionless requests select independently without creating affinity. Existing sessions fail when their account is disabled or removed instead of switching upstream identity. Restart the conversation after an intentional account change. Retrying a request never moves it to another account automatically.

The initial resource limits are 100 accounts, eight concurrent upstream operations, eight WebSocket connections, 8 MiB request/event/history bounds, 30-second request-body reads, 20-second upstream response-header/model-catalog deadlines, bounded 20–30-second token exchange deadlines, and ten-minute generation deadlines. WebSocket pings maintain a five-minute read deadline. These are safety bounds, not measured throughput or memory guarantees. Per-account concurrency, cooldown, and bounded request metadata are described in [pool runtime](pool-runtime.md). Member request limits and operational usage summaries are described in [team controls](team-controls.md). Token budgets, billing and advanced failover remain later work.

Provider credentials, browser cookies, arbitrary client headers, and raw provider error bodies are not forwarded to members. Client bearer keys are replaced by the selected upstream credential. The application does not log prompts, responses, access tokens, refresh tokens, or OAuth callback URLs.

## Verification boundary

Automated tests use temporary databases, synthetic credentials, and local fake upstreams. They cover OAuth ownership/expiry/replay, encryption and key mismatch, refresh serialization, immutable identity, account affinity, role/ownership isolation, streaming cancellation, SSE completion repair, and WebSocket revocation between turns.

An opt-in integration test runs an installed Codex CLI with a fresh child-process home, no inherited credentials, and a local synthetic model response:

```sh
SUBLANE_TEST_CODEX=1 go test ./internal/server -run TestCodexCLIProtocol -count=1 -v
```

Codex CLI 0.152.1 passed both HTTP/SSE and WebSocket modes against the synthetic upstream. Actual subscription authorization and live model requests require an explicitly authorized test account. The real desktop application has not yet been validated; shared configuration and protocol tests alone do not establish live desktop compatibility.

Before the v7 upgrade, a local macOS arm64 preview (CLIProxyAPI v6.10.9, Go 1.26.0, production binary, one encrypted synthetic account) measured 23,376 KiB RSS, approximately 22.8 MiB, after about twelve minutes idle on 2026-09-19. This is an idle observation only; real models, concurrency, payload size, and platform change memory use. Docker Compose configuration validated locally, but the Docker daemon was unavailable for a local image build.
