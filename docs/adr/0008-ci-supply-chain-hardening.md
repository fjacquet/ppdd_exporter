# 0008. CI/CD supply-chain hardening: scanning, SHA-pinned Actions, GoReleaser

- **Status:** accepted
- **Date:** 2026-06-05
- **Deciders:** Frederic Jacquet

## Context and problem statement

The CI gate ran only `gofmt`, `go vet`, race tests, and a build — no vulnerability
scanning, no SAST, no linter. The release pipeline was a hand-rolled matrix
(`softprops/action-gh-release` + a `go build` loop + `anchore/sbom-action`), and every
GitHub Action across `ci.yml`, `release.yml`, and `docs.yml` was referenced by a
**mutable tag** (`@v6`, `@v3`, …). A re-pointed tag (owner action or repo compromise)
would run unreviewed code with the workflow token and secrets, and known-CVE
dependencies or insecure code patterns could merge unflagged. This brings the project
to parity with the sibling `pflex_exporter` (its ADR-0001).

## Considered options

- Keep mutable tags and the minimal gate — accept the supply-chain and CVE exposure.
- Add scanning + SHA-pinning but keep the bespoke release shell.
- Add scanning + SHA-pinning **and** migrate the release to GoReleaser.

## Decision outcome

Chosen option: **"scanning + SHA-pinning + GoReleaser"**, because it is cheap,
high-value hardening that closes all three gaps at once and matches the established
`pflex_exporter` pipeline.

1. **CI scanning.** `make ci` now also runs `golangci-lint` (pinned `v2.12.2`) and
   `govulncheck`; `ci.yml` adds a dedicated **Semgrep** job (`--config auto --error`)
   and an SBOM artifact job (`cyclonedx-gomod`).
2. **Pin every Action to a full commit SHA** with a trailing `# vX.Y.Z` comment. A
   `.github/dependabot.yml` (ecosystems: `github-actions`, `gomod`, `docker`) reads the
   comment and bumps both SHA and comment weekly, so pinning is not stagnation.
3. **Migrate the release to GoReleaser** (`.goreleaser.yaml`, schema v2): cross-compile
   `linux,darwin × amd64,arm64`, `tar.gz` archives bundling `LICENSE`/`README.md`/
   `config.yaml`) **plus the raw binaries** for each target, `checksums.txt`, the
   CycloneDX SBOM (kept on `cyclonedx-gomod`, not syft, so its content matches
   `make sbom`), reproducible-build flags (`-trimpath`, `mod_timestamp`), and a macOS
   **Homebrew cask** gated on `HOMEBREW_TAP_GITHUB_TOKEN`.
   The multi-arch GHCR image job (build-time SBOM + max-mode provenance) is retained,
   now SHA-pinned and using `docker/metadata-action` for tags. The Dockerfile base is
   pinned to `golang:1.26.4`.

### Consequences

- Good — workflows execute only reviewed, immutable Action code; tag-repoint attacks are
  neutralised, with Dependabot keeping pins fresh via reviewable PRs.
- Good — CVE-bearing dependencies and flagged code patterns now fail CI before merge.
- Good — release assets gain archive metadata, bundled docs/licence, checksums, and
  reproducible builds; `make release-snapshot` reproduces the pipeline locally.
- Neutral — releases gain `tar.gz` archives alongside the raw binaries (same base name);
  the SBOM document name and checksum coverage change, but direct binary downloads keep
  working.
- Neutral — the injected `main.version` now uses GoReleaser's `{{ .Version }}` (no `v`
  prefix) instead of the raw tag; the Homebrew cask self-skips until the
  `fjacquet/homebrew-tap` repo and `HOMEBREW_TAP_GITHUB_TOKEN` secret exist.

## Pros and cons of the options (optional)

### Keep mutable tags and minimal gate
- Good: zero effort.
- Bad: leaves the supply-chain and CVE exposure open; diverges from `pflex_exporter`.

### Scanning + SHA-pinning, keep bespoke release shell
- Good: closes the scanning and tag-repoint gaps.
- Bad: the hand-rolled matrix keeps drifting from the image path, with no archive
  metadata, checksums, or reproducible-build affordances.
