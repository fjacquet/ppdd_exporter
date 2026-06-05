# CLAUDE.md

Guidance for working in `ppdd_exporter`.

## Commands
- `make cli` — build `bin/ppdd_exporter`.
- `make test` / `make test-race` — tests.
- `make tools` — install pinned dev/CI tooling (golangci-lint, cyclonedx-gomod, govulncheck).
- `make ci` — gofmt check + vet + lint + race tests + govulncheck + build (the CI gate).
- `make release-snapshot` — local GoReleaser dry-run (binaries + archives + SBOM + checksums).
- Run: `./bin/ppdd_exporter --config config.yaml [--once] [--debug]`. Secrets are `${ENV}`
  refs in `config.yaml` (or `passwordFile`).
- Docs: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`.

## Architecture
Prometheus exporter for Dell PowerProtect DD. A background **collection loop**
(`internal/ppdd/collector.go`) polls every system on `collection.interval` and publishes an
immutable **snapshot** to a `SnapshotStore` (RWMutex pointer-swap). The `/metrics` handler
(`prometheus.go`, an *unchecked* collector) reads the latest snapshot, decoupling DD API load
from scrape count. `main.go` wires the server, loop, hot reload, and `/health`.

Collection is **modular**: each domain implements `ResourceCollector` (`resource.go`) — owns
its endpoint path, JSON struct, and `parse → []Sample`. Modules: `capacity`, `mtrees`,
`replication`, `health`. A module failure degrades to `ppdd_collector_up{collector}=0`, never
crashing the cycle.

## Conventions (load-bearing)
- **Provisional API mappings.** Endpoint paths/fields are modeled from Dell docs only — confirm
  against a live DD. Each correction is one struct + one `testdata/*.json` fixture in one module.
- **Label-key consistency.** A metric name must carry one label-key set across all series;
  `labels_test.go` guards this.
- **Auth.** Token via `X-DD-AUTH-TOKEN`; retry excludes 4xx (never retry auth failures).
- **Always update docs + `CHANGELOG.md`** in the same change as a feature.

## Adding a metric domain
Add a `ResourceCollector` in `internal/ppdd/<name>.go` (+ test + `testdata/<name>.json`),
register it in `Registry()`, document it in `docs/metrics.md`, and add a `CHANGELOG.md` entry.

---

<!-- rtk-instructions v2 -->
# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)
```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (60-99% savings)
```bash
rtk cargo test          # Cargo test failures only (90%)
rtk go test             # Go test failures only (90%)
rtk jest                # Jest failures only (99.5%)
rtk vitest              # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk pytest              # Python test failures only (90%)
rtk rake test           # Ruby test failures only (90%)
rtk rspec               # RSpec test failures only (60%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)
```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)
```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)
```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma              # Prisma without ASCII art (88%)
```

### Files & Search (60-75% savings)
```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%). Format flags (-c, -l, -L, -o, -Z) run raw.
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)
```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff                # Ultra-compact diffs
```

### Infrastructure (85% savings)
```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl get         # Compact resource list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)
```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands
```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% token reduction** on common development operations.
<!-- /rtk-instructions -->
