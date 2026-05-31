# 0004. Token auth with a retry policy that excludes 4xx

- **Status:** accepted
- **Date:** 2026-05-30
- **Deciders:** Fred Jacquet

## Context and problem statement

The DD REST API is token-authenticated: `POST /api/v1/auth` returns an `X-DD-AUTH-TOKEN`
header, which must accompany subsequent requests until it is rejected. Network calls to the
appliance can fail transiently (transport/TLS errors, 5xx), but some failures are permanent and
must not be retried — repeatedly retrying a bad credential (401) or a forbidden request (403)
wastes time and can lock accounts.

## Considered options

- **Blanket retry** — retry any failed request N times.
- **Retry transient only** — retry transport errors and 5xx; never retry 4xx; re-authenticate once on 401.

## Decision outcome

Chosen option: **retry transient only** (`internal/ddclient/system.go`):

- Auth is lazy: the client authenticates on first `Get`, attaches the token to every request, and on a `401` clears the token, re-authenticates **once**, and retries. `DELETE /api/v1/auth` releases the session on `Close()`.
- The resty retry condition retries on transport errors and `>= 500`, and **never on 4xx**.
- The retry predicate is nil-safe: resty passes a `nil` `*resty.Response` on transport/TLS errors, so the condition guards the dereference (transport errors retry; a present response retries only on 5xx).
- `insecureSkipVerify` is configurable per system but defaults to off (see [ADR notes / configuration](../getting-started/configuration.md)).

### Consequences

- Good — no retry storms or account lockouts on bad credentials/permissions.
- Good — resilient to transient appliance/network blips without masking real auth failures.
- Bad — only a single automatic re-login attempt on 401; a persistently rotating/invalid token surfaces as an error rather than looping.
- Neutral — carried over deliberately from the sibling `pflex_exporter`; do not add a blanket retry-after-error condition.

## Related

- [0001. Snapshot model](0001-prometheus-snapshot-model.md)
