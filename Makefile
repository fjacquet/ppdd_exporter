BIN := bin/ppdd_exporter
VERSION ?= dev

COMPOSE_BUILD := deploy/compose/docker-compose.build.yml
COMPOSE_GHCR  := deploy/compose/docker-compose.ghcr.yml

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
ci: fmt-check vet test-race

# End-to-end demo stacks (mockdd -> exporter -> Prometheus -> Grafana).
# Grafana: http://localhost:3000 (admin/admin). Requires a running Docker daemon.
demo:
	docker compose -f $(COMPOSE_BUILD) up --build
demo-ghcr:
	docker compose -f $(COMPOSE_GHCR) up
demo-down:
	docker compose -f $(COMPOSE_BUILD) down --remove-orphans
	docker compose -f $(COMPOSE_GHCR) down --remove-orphans
