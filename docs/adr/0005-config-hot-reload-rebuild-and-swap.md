# 0005. Hot reload rebuilds and swaps runtime components

- **Status:** accepted
- **Date:** 2026-05-31
- **Deciders:** Fred Jacquet
- **Supersedes:** the initial "log-only on reload" behavior

## Context and problem statement

The exporter watches its config file and reloads on `SIGHUP` or file change. The original
implementation only **logged** that a change was detected — the running process kept its
original clients, collector timings, and server settings until a manual restart. That made the
advertised "hot reload" misleading: nothing was actually applied. A code review (PR #1) flagged
this. We needed reload to either truly apply changes or be honestly scoped to "restart required".

## Considered options

- **Log only** (status quo) — detect and log; operator restarts to apply. Misleading.
- **Full live swap of everything** — including rebinding the HTTP listener on host/port/uri changes.
- **Rebuild-and-swap the collection runtime; flag server changes as restart-required** — apply what can be applied safely; be explicit about what cannot.

## Decision outcome

Chosen option: **rebuild-and-swap the collection runtime**. A `collectorRunner` (`main.go`)
owns the live collection loop and its DD clients. On a successful reload it builds fresh clients
and a fresh collector from the new config, runs one immediate cycle, then atomically swaps them
in and tears down the previous loop and clients. The shared `SnapshotStore` is never replaced,
so `/metrics` and `/health` keep serving the last snapshot across the swap. Changes to the
`server` section (host/port/uri) cannot be applied without rebinding the listener and are
logged as **restart-required**. The watcher follows the parent directory rather than the file
inode, so atomic "write-temp + rename" updates still trigger a reload. A reload that fails to
load/validate is logged and dropped — the running config stays in effect.

### Consequences

- Good — `systems` and `collection` (interval/timeout) changes apply without a restart, picked up within one reload.
- Good — metrics serving is uninterrupted during a swap; failed reloads never degrade the running config.
- Good — robust to editor/config-manager update patterns (inode replacement).
- Bad — `server` host/port/uri changes still require a restart (accepted: rebinding a live listener is out of scope).
- Neutral — reloads are serialized by a single watcher goroutine; the runner only locks to guard against a concurrent shutdown.

## Related

- [0001. Snapshot model](0001-prometheus-snapshot-model.md) — the shared store is what makes a seamless swap possible.
- Configuration → [Hot reload](../getting-started/configuration.md)
