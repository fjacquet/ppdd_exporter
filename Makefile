BIN := bin/ppdd_exporter
VERSION ?= dev

.PHONY: cli test test-race vet fmt-check sure ci
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
