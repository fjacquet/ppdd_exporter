# 0007. Realign collectors to the documented DD 7.3 REST API contract

- **Status:** accepted (amended by [`ADR-0009`](0009-validate-against-8.7.0-openapi.md))
- **Date:** 2026-06-03
- **Deciders:** Fred Jacquet

## Context and problem statement

Collector endpoints were modeled as a uniform `/api/v1/...` (see
[0003](0003-provisional-api-mappings.md)). The full PowerProtect DD 7.3 REST API guide shows the
`/rest/` prefix (documented as supported across DD OS versions) with **per-resource** API
versions — `mtrees` is **v3.0**, `stats/capacity` is **v2.0**, `system`/`alerts`/`auth` are
**v1.0** — plus list pagination (default 20, max 200) and an `is_active` alerts filter. No live
appliance is available, so the guide is treated as authoritative while undocumented mappings stay
provisional.

## Considered options

- **Central path registry + pagination helper** — one file owns all paths; a shared helper follows pages; undocumented mappings stay provisional and best-effort.
- **Operator-configurable endpoint map in config** — paths in `config.yaml`; flexible but adds config surface for a one-time decision.
- **Minimal in-place edits** — rewrite each collector's path inline; duplicates pagination and scatters the correction point.

## Decision outcome

Chosen option: **central path registry + pagination helper**. All paths live in
`internal/ppdd/endpoints.go` with per-resource versions; a `paginate()` helper fixes silent
data-loss for every list collector; capacity sources from `/system`; alerts fetch `is_active=true`
with a `class` label; MTrees use the v3.0 metadata list plus per-MTree v2.0 stats. Undocumented
metrics (compression split, GC cleaning, MTree `physical_used`, quotas) are kept as flagged
best-effort fetches — no metric removed. Auth aligns to the documented flat `{username,password}`
body at `/rest/v1.0/auth`.

### Consequences

- Good — pagination fixes silent truncation past 20 items on every list endpoint.
- Good — corrections against a live appliance are a one-file edit (`endpoints.go`) or one struct.
- Good — no exported metric is removed, so existing dashboards keep working.
- Bad — per-MTree usage is N+1 requests (one stats call per MTree), fetched best-effort.
- Bad — the auth body change is the highest-risk mapping; if wrong, all logins fail.
- Neutral — values remain provisional until validated against a live DD (see checklist below).

## Validate against a live DD (revert points, in priority order)

1. **Auth** — `internal/ddclient/auth.go`: flat body + `/rest/v1.0/auth`. If logins fail, revert here first (prior shape: `{"auth_info":{...}}` at `/api/v1/auth`).
2. **Prefix/versions** — `internal/ppdd/endpoints.go`: confirm `/rest` vs `/api` and the v1.0/v2.0/v3.0 tokens.
3. **Capacity** — confirm `/system` fields (`physical_capacity.{total,used,available}`, `compression_factor`).
4. **MTrees** — confirm v3.0 list fields (`id`, `is_degraded`, `mtree_rl_detail.rl_status`) and v2.0 stats (`stats_capacity[].tier_capacity_usage[].logical_capacity.used`).
5. **Alerts** — confirm the `is_active` filter and the `class` field.
6. **Provisional endpoints** — `replications`, `hardware/disks`, `stats/system-stats`, `file-system`: confirm or correct paths and field names.

## Related

- [0003. Treat DD API mappings as provisional until validated](0003-provisional-api-mappings.md)
- [0002. Modular per-domain ResourceCollectors](0002-modular-resource-collectors.md)
- [0006. One label-key set per metric name](0006-label-key-consistency-invariant.md)
