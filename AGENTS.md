# Paperless Knowledge Bridge

## Purpose

This private service is the only supported retrieval boundary between the
personal Paperless archive and AI consumers. It owns indexing, deletion
propagation, bounded retrieval and audit metadata. Its canonical deployment
configuration lives in `~/code/brandymint/infra`.

## Security boundary

- Consumers never receive a Paperless API token.
- The service must not expose originals, previews or attachments. Responses are
  limited to document metadata, a Paperless source URL and short text excerpts.
- The archive belongs to one personal principal. An explicit consumer tool call
  is the gate for sending selected excerpts to an active LLM route.
- Secrets belong in `pass` and Kubernetes Secrets; do not commit them or print
  them.

## Engineering

- Keep the public API versioned under `/v1` and validate all input/output.
- Make synchronization idempotent. A deleted Paperless document must be removed
  from downstream indexes within 15 minutes.
- Add tests for access boundaries, result caps and deletion propagation.
- Run repository checks before committing. Keep application code here and
  Kubernetes/Helmfile changes in `~/code/brandymint/infra`.
