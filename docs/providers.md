# Subscription providers

SubLane supports three subscription channels through CLIProxyAPI v7.3.7. Authentication channels are distinct from model families: Antigravity exposes models made available to that account, including Gemini; it is not a Gemini API-key connection.

| Channel | Browser authorization callback | Import | Quota display |
| --- | --- | --- | --- |
| Codex | `http://localhost:1455/auth/callback` | Codex `auth.json` (nested tokens or flat export) | Supported |
| Claude | `http://localhost:54545/callback` | CLIProxyAPI flat Claude credential JSON | Not yet supported |
| Antigravity | `http://localhost:51121/oauth-callback` | CLIProxyAPI flat Antigravity credential JSON | Not yet supported |

## Connect

As an administrator, open **Accounts → Add account**, select the provider, enter a name, and choose browser authorization or JSON import. After browser authorization, copy the complete localhost callback URL into the dialog. SubLane does not start a localhost callback receiver, so a browser connection error at that final address is expected. Both local and remote deployments use this flow.

All authorization attempts have random, session-bound, single-use state, a ten-minute deadline, and bounded concurrency. Codex and Claude also use PKCE. Antigravity follows the SDK’s Google installed-app authorization flow with state, offline access, and consent. A callback from a different provider or administrator session is rejected.

Imports must contain subscription access and refresh tokens. Claude/Antigravity imports use the flat CLIProxyAPI export with the matching `type`, `access_token`, `refresh_token`, identity (`email`, and optionally Claude `account_uuid`/`organization_uuid`), and `expired` (RFC3339) or `expires_at` (Unix seconds). Antigravity may include `project_id`; missing project metadata is prepared before requests. Only credential and allowlisted metadata fields survive import. API-key credentials and imported endpoint/proxy overrides are not supported. SubLane never discovers local credentials automatically.

Verify the connection to fetch the provider’s model catalog. Reauthorization must keep the same provider and upstream identity. Existing Codex accounts migrate automatically; their encrypted credentials, quota snapshots, and conversation bindings are preserved.

## Choose a model

Use a personal SubLane key to request `GET /v1/models`, then use an exact native ID from that response. SubLane does not add `codex/`, `claude/` or `antigravity/` prefixes. Identical model IDs across providers appear once, and requests select an available account reporting that model within the key's group and model policy.

Existing explicitly prefixed requests remain compatible and select their named provider. Prefixes are removed before upstream execution. Models from healthy providers remain visible when another provider is unavailable. Native conversations keep one account across providers and never switch after account saturation, disablement or deletion. See [model catalogs](models.md) for legacy conversation migration and ambiguity handling.

All three channels expose OpenAI-compatible `POST /v1/responses` and `POST /v1/chat/completions`, with JSON or SSE, plus the existing Responses WebSocket endpoint. This does not add a native Anthropic `/v1/messages` endpoint. Clients requiring that endpoint need a compatible Responses/Chat adapter. `/v1/responses/compact` remains Codex-only; clients using Claude or Antigravity must manage compaction locally or send a complete transcript.

The API keys page’s Codex configuration remains a starting point for Codex clients. Replace `model` with the native ID returned by the model catalog. Arbitrary upstream models do not automatically inherit Codex-specific tools or capabilities.

## Credentials and verification

SubLane owns OAuth exchange, refresh, encrypted persistence, and account selection. Model execution receives persisted access tokens without refresh tokens. Antigravity authorization-code exchange and explicit token refresh reuse public SDK interfaces under SubLane’s bounded transport; their results return to the durable owner. Refreshed credentials are saved before use; failed database writes stop the request. No live credentials are registered in the SDK manager, and no plaintext credential files are created. See [architecture](architecture.md) and [gateway limits](codex.md).

Automated coverage uses synthetic identities, temporary databases, and mocked provider responses. It exercises migrations, authorization ownership, provider routing, JSON/SSE translation, model catalog isolation, and failed persistence. Live OAuth grants and real provider model/tool behavior are separate acceptance steps; implementation and mocked tests do not establish universal provider or desktop compatibility.

## Local resource observation

On 2026-09-19, the macOS arm64 production build using CLIProxyAPI v7.3.7 measured 108,880 KiB RSS (about 106.3 MiB) after roughly three minutes in an isolated instance with three synthetic accounts, administrator setup/login, and browser UI checks. No model generation ran during this observation. This includes retained allocations from those operations; it is not a steady-state idle baseline, concurrency budget, or cross-platform guarantee. The former lightweight v6 observation is not a like-for-like comparison. Measure the intended workload before setting a deployment memory limit.
