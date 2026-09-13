# Paperless Knowledge Bridge

Private, read-only retrieval boundary between the personal Paperless archive and
AI consumers. The governing cross-consumer contract is maintained in
[`~/code/brandymint/infra/docs/specs/2026-09-13-paperless-knowledge-bridge.md`](../brandymint/infra/docs/specs/2026-09-13-paperless-knowledge-bridge.md).

## Guarantees

- Paperless access stays inside this service; consumers do not receive its API
  token.
- `POST /v1/search` returns at most five short excerpts with metadata and a
  Paperless source link—never files, previews or attachments.
- A consumer must make an explicit tool call to retrieve excerpts. This is the
  gate before any selected excerpt reaches an active LLM route.
- Synchronization is read-only and idempotent. A full reconciliation removes
  index chunks for documents that no longer exist in Paperless.
- `PILOT_LIMIT=10` through `20` prevents a full-archive reconciliation and is
  mandatory for the first deployment. Set it to `0` only after accepting the
  pilot.

## Runtime configuration

| Variable | Purpose |
| --- | --- |
| `PAPERLESS_URL` | Internal Paperless base URL. |
| `PAPERLESS_API_TOKEN` | Read-only Paperless token. |
| `LITELLM_URL` | OpenAI-compatible LiteLLM base URL (including `/v1`). |
| `LITELLM_API_KEY` | LiteLLM authentication, if enabled. |
| `EMBEDDING_MODEL` | Defaults to dedicated `paperless-embedding`. |
| `BRIDGE_OPENWEBUI_TOKEN` | Bearer token required for Open WebUI search. |
| `BRIDGE_CODEX_TOKEN` | Bearer token required for Codex search. |
| `BRIDGE_HERMES_TOKEN` | Bearer token required for Hermes search. |
| `PAPERLESS_WEBHOOK_TOKEN` | Bearer token required for a Paperless workflow webhook. |
| `PILOT_LIMIT` | `1..20` for the approved pilot; `0` after acceptance. |
| `RECONCILE_INTERVAL` | Full reconciliation cadence; defaults to `15m`. |
| `INDEX_PATH` | Durable private index path; defaults to `/data/index.json`. |

Secrets are injected from Kubernetes Secrets and originate in `pass`; do not
put them in a local config file or this repository.

## HTTP contract

`GET /openapi.json` describes the single external tool operation.

`POST /v1/search` requires `Authorization: Bearer <consumer-specific token>`,
`X-Paperless-Knowledge-Consumer: open-webui|codex|hermes`, and a JSON object
such as `{ "query": "договор аренды", "limit": 5 }`.

`POST /v1/hooks/paperless` requires
`Authorization: Bearer <PAPERLESS_WEBHOOK_TOKEN>`. It starts an asynchronous
read-only reconciliation. Paperless should invoke it from a workflow after a
document create/update/delete event; a scheduled reconciliation remains the
deletion-safety net.

## Local checks

```sh
go test ./...
go vet ./...
docker build -t paperless-knowledge-bridge:dev .
```
