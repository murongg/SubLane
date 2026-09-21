# Native client protocols

These endpoints were introduced after `v0.1.0-rc.1`; that release does not include them. Use a source build until a newer release includes this change.

SubLane accepts Claude Messages and Gemini generation requests alongside its OpenAI-compatible endpoints. Incoming protocol and subscription provider are independent: requests still select an eligible account reporting the requested model within the API key's group. The public CLIProxyAPI executors handle provider translation; native Claude requests do not pass through an intermediate Responses representation.

## Endpoints and authentication

| Protocol        | Endpoint                                                    | Credentials                                                         |
| --------------- | ----------------------------------------------------------- | ------------------------------------------------------------------- |
| Claude Messages | `POST /v1/messages`                                         | `x-api-key` or `Authorization: Bearer`                              |
| Gemini JSON     | `POST /v1beta/models/{model}:generateContent`               | `x-goog-api-key`, `Authorization: Bearer`, or `key` query parameter |
| Gemini SSE      | `POST /v1beta/models/{model}:streamGenerateContent?alt=sse` | Same as Gemini JSON                                                 |

Use a personal **SubLane** API key. If multiple credential locations are supplied, they must agree; an invalid Authorization header cannot fall back to another key. Browser session cookies are not gateway credentials. Prefer headers to query-string keys because other proxies may log URLs. SubLane does not forward client keys, cookies or query strings to providers.

Use `GET /v1/models` with Bearer authentication to discover permitted native model IDs. For Gemini requests, the model in the URL is authoritative; a body `model` or `stream` field cannot override the route. `streamGenerateContent` returns SSE; `alt` may be omitted or set to `sse`.

## Claude example

Set `SUBLANE_URL` to the instance origin and `SUBLANE_API_KEY` to your personal key. Replace `YOUR_MODEL_ID` with a model from the catalog:

```sh
curl "$SUBLANE_URL/v1/messages" \
  -H "x-api-key: $SUBLANE_API_KEY" \
  -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  --data '{"model":"YOUR_MODEL_ID","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'
```

Add `"stream": true` for named Messages SSE events. Native request content includes system blocks, tools, tool results, cache controls and thinking configuration. Claude-format responses preserve content blocks and thinking signatures, subject to the selected executor's provider-specific handling. Errors use the Anthropic envelope and named SSE error events. `request-id` and `X-Request-ID` identify the corresponding request record.

## Gemini example

```sh
curl "$SUBLANE_URL/v1beta/models/YOUR_MODEL_ID:generateContent" \
  -H "x-goog-api-key: $SUBLANE_API_KEY" \
  -H 'content-type: application/json' \
  --data '{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}'
```

For streaming, use `:streamGenerateContent?alt=sse` and `curl --no-buffer`. Responses contain native `candidates`, parts and `usageMetadata`; Antigravity's internal `response` envelope is removed by the SDK. Tool calls and thought signatures stay in their Gemini representation. Errors use Google-style `error.code`, `error.status` and `error.message`. SSE chunks have `data:` framing without OpenAI's `[DONE]` marker.

## Shared behavior

- Enabled members and keys, key expiry, group grants, model policies, account availability, concurrency and member request limits are rechecked through the existing gateway services.
- For a new conversation or sessionless request, Responses/Chat prefer eligible Codex accounts, Messages prefers Claude, and Gemini prefers Antigravity. All candidates must support the requested model. Other eligible providers remain a fallback when the preferred pool is unavailable; accounts rotate within the selected tier. Explicit provider prefixes and existing conversation bindings take precedence over this preference.
- Conversation affinity uses `Session_id`; Claude also uses `metadata.user_id` when no explicit session is supplied. Identifiers are scoped to the member and group. Requests without an affinity identifier remain sessionless.
- Cancellation closes upstream work and releases account/member leases. Missing terminal events and malformed or oversized events are failures, not successful empty responses. Streaming errors are sanitized after headers have been sent.
- Request history labels operations as `messages` or `gemini`, with request IDs, first-output latency and available token usage. Anthropic input totals include cache reads and creation; Gemini input already includes cached tokens, and its output total includes thinking tokens. Trailing Gemini usage chunks are consumed after `finishReason`.
- Existing 8 MiB request/event limits and ten-minute operation deadlines apply. Streaming generation is not buffered into a complete response for the client.

Migration `020_native_protocols.sql` extends the request-history operation constraint while retaining existing rows, indexes and the autoincrement high-water mark. No new persistent service is required.

## Scope and verification

The current synthetic compatibility matrix covers JSON and SSE request/response flows, tool-call output and protocol termination:

| Client protocol         | Codex account | Claude account | Antigravity account |
| ----------------------- | ------------- | -------------- | ------------------- |
| OpenAI Responses        | Tested        | Tested         | Tested              |
| OpenAI Chat Completions | Tested        | Tested         | Tested              |
| Claude Messages         | Tested        | Tested         | Tested              |
| Gemini generation       | Tested        | Tested         | Tested              |

`TestClientProtocolMatrix` exercises all 24 provider/protocol/stream combinations against mocked upstreams. Separate tests cover native signatures, cache usage, permissions, retries, cancellation and malformed streams. These checks establish the protocol behaviors they exercise; they do not certify every tool, thinking mode or provider-specific field. Full real-account client acceptance remains pending for each combination.

Coverage uses synthetic accounts and mocked providers, including native JSON/SSE, tools/thinking, cross-provider translation, authorization, usage, interruption and cancellation. It does not establish complete real-account Claude Code, Gemini CLI or desktop compatibility. Advanced native features depend on the selected provider and SDK conversion path.

In **API keys → Setup guide**, choose the API format your client supports. The guide provides Codex configuration or native Claude/Gemini request examples, with the correct base URL and authentication header. Select a model from the key's group. The guide never fetches a key's secret; examples read `SUBLANE_API_KEY` from the client environment. CC Switch quick import remains Codex-specific.

Token-count endpoints, Message Batches, Files, Gemini Live and Antigravity desktop connection protocols are not implemented by this change. Unsupported actions are rejected rather than treated as generation requests. Native model-discovery resources are not exposed; use the group-scoped `/v1/models` catalog.

Protocol references: [Claude streaming](https://platform.claude.com/docs/en/build-with-claude/streaming), [Claude errors](https://platform.claude.com/docs/en/api/errors), and [Gemini generation](https://ai.google.dev/api/generate-content). Routing and compatibility boundaries were compared with [sub2api](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/server/routes/gateway.go) and [New API](https://github.com/QuantumNous/new-api/blob/main/router/relay-router.go); SubLane retains its own permission, lifecycle and storage implementation.
