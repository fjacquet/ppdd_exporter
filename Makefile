BIN := bin/ppdd_exporter
VERSION ?= dev

.PHONY: cli test test-race vet fmt-check sure ci demo demo-ghcr demo-down
cli:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) .
test:
	go test ./...
test-race:
	go test -race -cover ./...
vet:
	go vet ./...
fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)"
sure: fmt-check vet test cli
ci: fmt-check vet test-race cli

# End-to-end demo stacks (mockdd -> exporter -> Prometheus -> Grafana).
# Grafana: http://localhost:3000 (admin/admin). Requires a running Docker daemon.
demo:
	docker compose up --build
demo-ghcr:
	docker compose -f docker-compose.ghcr.yml up
demo-down:
	docker compose down --remove-orphans
	docker compose -f docker-compose.ghcr.yml down --remove-orphans
