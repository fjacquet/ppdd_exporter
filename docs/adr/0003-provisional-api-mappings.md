# 0003. Treat DD API mappings as provisional until validated

- **Status:** superseded by [`ADR-0009`](0009-validate-against-8.7.0-openapi.md)
- **Date:** 2026-05-30
- **Deciders:** Fred Jacquet

## Context and problem statement

The exporter targets the Dell PowerProtect DD Series REST API (developer.dell.com API **4118**,
version **8.3.0**, DD OS 7.2+, base `https://<dd-host>:3009/api/v1/`). It was built without
access to a live appliance, so endpoint paths and JSON field names are modeled **from the
published documentation only**. They are best-effort and may not match a real DD exactly.

## Considered options

- **Block on hardware** — do not ship until every mapping is confirmed against a live DD.
- **Ship as provisional** — model from docs, make the uncertainty explicit, and structure the code so corrections are cheap.

## Decision outcome

Chosen option: **ship as provisional**, and make "provisional" a load-bearing project
convention. Fixtures live as `internal/ppdd/testdata/*.json` modeled from the documented schema;
each future correction is **one struct change plus one fixture** in a single module (enabled by
[ADR-0002](0002-modular-resource-collectors.md)). The status is called out in CLAUDE.md, the
design spec, and the metrics docs.

### Consequences

- Good — the project can ship, demo (via `mockdd`), and iterate without blocking on hardware.
- Good — the blast radius of any wrong mapping is one file plus one fixture.
- Bad — exported metric values may be incorrect for a given field until validated against a real appliance.
- Neutral — **action required before production reliance:** validate each domain's endpoint and fields against a live DD and update the corresponding fixture.

## Related

- [0002. Modular per-domain ResourceCollectors](0002-modular-resource-collectors.md)
