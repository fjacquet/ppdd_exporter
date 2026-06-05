BIN := bin/ppdd_exporter
DIST := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Pinned tool versions (installed by `make tools`).
GOLANGCI_LINT_VERSION   ?= v2.12.2
CYCLONEDX_GOMOD_VERSION ?= latest
GOVULNCHECK_VERSION     ?= latest

.PHONY: tools tools-sbom cli test test-race vet fmt-check lint vuln sbom \
        sure ci release release-snapshot demo demo-ghcr demo-down clean clean-dist

# --- tooling ---

# Install pinned dev/CI tooling into $(GOBIN)/$GOPATH/bin.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Just the SBOM generator — used by the release pipeline (GoReleaser sboms hook).
tools-sbom:
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

# --- quality gates (used by CI) ---

fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)"
vet:
	go vet ./...
lint:
	golangci-lint run ./...
test:
	go test ./...
test-race:
	go test -race -cover ./...
vuln:
	govulncheck ./...

# Local convenience gate.
sure: fmt-check vet test cli
# Aggregate gate run by CI: fmt + vet + lint + race tests + vuln + build.
ci: fmt-check vet lint test-race vuln cli

# --- artifacts ---

cli:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

# CycloneDX SBOM for the Go module (source/dependency SBOM).
sbom:
	@mkdir -p $(DIST)
	cyclonedx-gomod mod -licenses -json -output $(DIST)/sbom.cdx.json
	@echo "wrote $(DIST)/sbom.cdx.json"

# Cross-compiled binaries + archives + SBOM + checksums + GitHub Release.
# Driven by GoReleaser (.goreleaser.yaml). Real releases run from a `v*` tag in CI;
# this target reproduces that pipeline locally — needs a tag and GITHUB_TOKEN.
release: tools-sbom
	goreleaser release --clean

# Local dry-run: full pipeline (build, archive, SBOM, checksums) without publishing.
release-snapshot: tools-sbom
	goreleaser release --snapshot --clean
	@echo "release artifacts in $(DIST)/"

# End-to-end demo stacks (mockdd -> exporter -> Prometheus -> Grafana).
# Grafana: http://localhost:3000 (admin/admin). Requires a running Docker daemon.
demo:
	docker compose up --build
demo-ghcr:
	docker compose -f docker-compose.ghcr.yml up
demo-down:
	docker compose down --remove-orphans
	docker compose -f docker-compose.ghcr.yml down --remove-orphans

clean-dist:
	rm -rf $(DIST)
clean: clean-dist
	rm -f $(BIN)
