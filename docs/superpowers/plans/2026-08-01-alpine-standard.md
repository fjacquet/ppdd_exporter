# Alpine Standard — ppdd_exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert ppdd_exporter's published and local container images from `gcr.io/distroless/static:nonroot` to Alpine, matching the family standard, and add `HEALTHCHECK`/`healthcheck:` across all three of this repo's compose files.

**Architecture:** Both `Dockerfile` and `Dockerfile.goreleaser` swap their final `FROM gcr.io/distroless/static:nonroot` stage for the family's canonical Alpine runtime stage; builder stages are untouched. `docker-compose.yml`, `docker-compose.ghcr.yml`, and `docker-compose.server.yml` all define a `ppdd_exporter` service and all gain `healthcheck:`.

**Tech Stack:** Docker, Alpine (`wget`/busybox), Go 1.26.5.

**Spec:** `docs/superpowers/specs/2026-08-01-alpine-standard-design.md` in `obs_exporter` (family-wide design).

## Global Constraints

- `HEALTHCHECK`/`healthcheck:` target `http://127.0.0.1:9441/livez`, never `localhost` — Alpine's busybox `wget` resolves `localhost` via `::1` first, and the exporter only binds IPv4.
- Timing: `--interval=30s --timeout=5s --start-period=10s --retries=3`.
- Builder stages do not change — only the final `FROM` and everything after it.
- Uid `10001`, named user `ppdd` (was `nonroot:nonroot`/`65532`) — **breaking change** for the published image; no Helm chart impact (confirmed: `charts/ppdd-exporter/values.yaml`'s `runAsUser`/`fsGroup` are commented-out generic defaults, never active).
- `/livez` and `/readyz` are already wired in `main.go` — confirmed, no Go code changes needed.
- No inline `nosemgrep`/`//nolint` suppressions.
- `make ci` must stay green.

## File Structure

| File | Responsibility |
| --- | --- |
| `Dockerfile` | Rewrite runtime stage: distroless → Alpine, add `HEALTHCHECK` |
| `Dockerfile.goreleaser` | Rewrite runtime stage: distroless → Alpine, add `HEALTHCHECK` |
| `docker-compose.yml` | Add `healthcheck:` to `ppdd_exporter` |
| `docker-compose.ghcr.yml` | Add `healthcheck:` to `ppdd_exporter` |
| `docker-compose.server.yml` | Add `healthcheck:` to `ppdd_exporter` |
| `docs/adr/000N-alpine-standard.md` | Records the decision (breaking) |
| `CHANGELOG.md` | `Breaking` entry |

---

### Task 1: Rewrite the local ./Dockerfile to Alpine

**Files:**
- Modify: `Dockerfile`

**Interfaces:** none.

- [ ] **Step 1: Replace the runtime stage**

Current file:

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ppdd_exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/ppdd_exporter /ppdd_exporter
USER nonroot:nonroot
EXPOSE 9441
ENTRYPOINT ["/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
```

(Note: this repo's build line has no `-s -w`, unlike its siblings — preserve exactly as-is, that's an existing, unrelated choice.)

Replace with:

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ppdd_exporter .

FROM alpine:latest

# Create the runtime user and log dir. These are busybox builtins (no network).
RUN adduser -D -u 10001 ppdd && \
    mkdir -p /var/log/ppdd_exporter && \
    chown ppdd:ppdd /var/log/ppdd_exporter

# Copy the CA bundle from the builder stage instead of `apk add ca-certificates`.
# The latter fetches from the Alpine CDN over TLS, which fails behind a corporate
# MITM proxy: the bare alpine image has no CA bundle yet to validate the proxy
# cert (chicken-and-egg). The Debian-based golang builder already ships the bundle.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=build /out/ppdd_exporter /usr/bin/ppdd_exporter
COPY config.yaml /etc/ppdd_exporter/config.yaml

EXPOSE 9441

# /livez never depends on target reachability or the collection cycle, so it
# can never flag a healthy process as down over an unreachable DD appliance
# (see ADR-000N).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9441/livez || exit 1

USER ppdd

ENTRYPOINT ["/usr/bin/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
```

The local image now bakes `config.yaml` in — it didn't before. Intentional, additive.

- [ ] **Step 2: Lint**

Run: `hadolint Dockerfile`
Expected: no findings on the lines just added.

- [ ] **Step 3: Build and verify at runtime**

```bash
docker build -t ppdd_exporter:alpine-test .
docker run -d --name ppdd-hc-test -p 19441:9441 \
  -v "$(pwd)/config.demo.yaml:/etc/ppdd_exporter/config.yaml:ro" \
  ppdd_exporter:alpine-test
sleep 15
docker inspect --format='{{.State.Health.Status}}' ppdd-hc-test
docker exec ppdd-hc-test whoami
```

Expected: `healthy`, `whoami` prints `ppdd`.

- [ ] **Step 4: Clean up test artifacts**

```bash
docker rm -f ppdd-hc-test
docker rmi ppdd_exporter:alpine-test
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "feat(docker)!: rewrite local Dockerfile to Alpine (was distroless)

BREAKING CHANGE: container UID changes from 65532 (nonroot) to 10001 (named user ppdd)."
```

---

### Task 2: Rewrite Dockerfile.goreleaser to Alpine

**Files:**
- Modify: `Dockerfile.goreleaser`

**Interfaces:** none.

- [ ] **Step 1: Replace the file**

Current file:

```dockerfile
# Used by GoReleaser (dockers_v2). The binary is cross-compiled by the build pipe;
# buildx lays it out per-platform as ${TARGETPLATFORM}/ppdd_exporter in the context.
# For local/dev builds from source, use the multi-stage ./Dockerfile instead.
FROM gcr.io/distroless/static:nonroot

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/ppdd_exporter /ppdd_exporter
COPY config.yaml /etc/ppdd_exporter/config.yaml

USER nonroot:nonroot
EXPOSE 9441

ENTRYPOINT ["/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
```

Replace with:

```dockerfile
# Used by GoReleaser (dockers_v2). The binary is cross-compiled by the build pipe;
# buildx lays it out per-platform as ${TARGETPLATFORM}/ppdd_exporter in the context.
# For local/dev builds from source, use the multi-stage ./Dockerfile instead.
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 ppdd && \
    mkdir -p /var/log/ppdd_exporter && \
    chown ppdd:ppdd /var/log/ppdd_exporter

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/ppdd_exporter /usr/bin/ppdd_exporter
COPY config.yaml /etc/ppdd_exporter/config.yaml

EXPOSE 9441

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9441/livez || exit 1

USER ppdd

ENTRYPOINT ["/usr/bin/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
```

- [ ] **Step 2: Lint**

Run: `hadolint Dockerfile.goreleaser`
Expected: no new findings.

- [ ] **Step 3: Build and verify at runtime**

```bash
CGO_ENABLED=0 go build -o ppdd_exporter .
mkdir -p linux/amd64 && cp ppdd_exporter linux/amd64/ppdd_exporter
docker build -f Dockerfile.goreleaser --build-arg TARGETPLATFORM=linux/amd64 -t ppdd_exporter:goreleaser-test .
docker run -d --name ppdd-gr-hc-test -p 19442:9441 \
  -v "$(pwd)/config.demo.yaml:/etc/ppdd_exporter/config.yaml:ro" \
  ppdd_exporter:goreleaser-test
sleep 15
docker inspect --format='{{.State.Health.Status}}' ppdd-gr-hc-test
```

Expected: `healthy`.

- [ ] **Step 4: Clean up test artifacts**

```bash
docker rm -f ppdd-gr-hc-test
docker rmi ppdd_exporter:goreleaser-test
rm -rf linux ppdd_exporter
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.goreleaser
git commit -m "feat(docker)!: rewrite the published image to Alpine (was distroless)

BREAKING CHANGE: container UID changes from 65532 (nonroot) to 10001 (named user ppdd)."
```

---

### Task 3: Add healthcheck to all three compose files

**Files:**
- Modify: `docker-compose.yml`
- Modify: `docker-compose.ghcr.yml`
- Modify: `docker-compose.server.yml`

**Interfaces:** none.

- [ ] **Step 1: docker-compose.yml**

In the `ppdd_exporter` service, after `restart: unless-stopped` (line 32):

```yaml
    depends_on:
      - mockdd
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9441/livez"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

- [ ] **Step 2: docker-compose.ghcr.yml**

Same block, in the `ppdd_exporter` service after its `restart: unless-stopped` (line 33).

- [ ] **Step 3: docker-compose.server.yml**

Same block, in the `ppdd_exporter` service after its `restart: unless-stopped` (line 39). This file has no `mockdd`/`depends_on`, so the block simply follows `restart: unless-stopped`.

- [ ] **Step 4: Validate**

Run: `for f in docker-compose.yml docker-compose.ghcr.yml docker-compose.server.yml; do docker compose -f "$f" config -q || echo "FAIL: $f"; done`
Expected: no `FAIL` output.

- [ ] **Step 5: Smoke-test docker-compose.yml**

```bash
docker compose up -d --build ppdd_exporter mockdd
sleep 20
docker inspect --format='{{.State.Health.Status}}' $(docker compose ps -q ppdd_exporter)
docker compose down
```

Expected: `healthy`.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml docker-compose.ghcr.yml docker-compose.server.yml
git commit -m "feat(docker): add healthcheck to all three compose stacks"
```

---

### Task 4: ADR + CHANGELOG

**Files:**
- Create: `docs/adr/000N-alpine-standard.md`
- Modify: `CHANGELOG.md`

**Interfaces:** none.

- [ ] **Step 1: Find the next ADR number**

Run: `ls docs/adr/ | sort -V | tail -3`

- [ ] **Step 2: Write the ADR**

```markdown
# Standardize container base image on Alpine

## Status

Accepted (2026-08-01)

## Context

The exporter family had two published-image patterns — Alpine (5 repos) and
`gcr.io/distroless/static:nonroot` (this repo and 2 others: pmax_exporter,
ppdm_exporter) — as undocumented per-repo author choice, with no written
criterion. Alpine has a shell and `wget`, so it can carry a Docker `HEALTHCHECK`
pointed at `/livez` (already wired in `main.go`); distroless cannot.

## Decision

Both `Dockerfile` and `Dockerfile.goreleaser` move from
`gcr.io/distroless/static:nonroot` to `alpine:latest`. Named user `ppdd`, uid
`10001` (was `nonroot:nonroot`/`65532`). `HEALTHCHECK`/`healthcheck:` against
`/livez` via `127.0.0.1` (never `localhost` — Alpine's busybox `wget` resolves
`localhost` via `::1` first, and the exporter only binds IPv4), applied to all
three of this repo's compose files (`docker-compose.yml`,
`docker-compose.ghcr.yml`, `docker-compose.server.yml`).

## Consequences

- **Breaking**: the published image's container UID changes from `65532` to
  `10001`. Checked this repo's Helm chart (`charts/ppdd-exporter/values.yaml`)
  for a hardcoded `runAsUser`/`fsGroup` referencing the old UID — none found;
  the chart's security-context fields are commented-out generic defaults,
  never active, so no chart change is required.
- The image gains a shell and `apk` — larger attack surface, larger image —
  accepted family-wide as the trade for `HEALTHCHECK` and shell-based
  debuggability.
- The full family standard and per-repo work breakdown live in
  `obs_exporter`'s `docs/superpowers/specs/2026-08-01-alpine-standard-design.md`.
```

- [ ] **Step 3: Add the CHANGELOG entry**

Under `## [Unreleased]`:

```markdown
### Breaking

- The published Docker image's base changes from
  `gcr.io/distroless/static:nonroot` to `alpine:latest`. The container UID
  changes from `65532` to a named user at `10001`. If you pin `runAsUser`,
  `fsGroup`, or similar in your own deployment manifests against the old UID,
  update it. See ADR-000N.

### Added

- `HEALTHCHECK` on both images, checking `/livez`; `healthcheck:` on all three
  compose stacks (`docker-compose.yml`, `docker-compose.ghcr.yml`,
  `docker-compose.server.yml`).
```

- [ ] **Step 4: Commit**

```bash
git add docs/adr/000N-alpine-standard.md CHANGELOG.md
git commit -m "docs: record ADR-000N (Alpine standard, breaking UID change)"
```

---

### Task 5: Full gate

- [ ] **Step 1: Run the CI gate**

Run: `make ci`
Expected: clean.

- [ ] **Step 2: Commit any fixes**

```bash
git commit -am "fix: address CI gate findings for the Alpine standard change"
```

(Skip if clean.)

## Self-Review

- Spec coverage: ppdd_exporter's row (full conversion, both Dockerfiles; healthcheck on all three compose files) — Tasks 1–3. Documentation — Task 4.
- No placeholders: ADR number confirmed by a one-command check; the `-s -w` omission in this repo's build line is preserved deliberately, not silently "fixed".
- Breaking change called out explicitly in commits and CHANGELOG.
- Scope: single repo; matches the family plan's per-repo row exactly.
