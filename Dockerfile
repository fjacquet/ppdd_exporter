# syntax=docker/dockerfile:1
FROM docker.io/library/golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ppdd_exporter .

FROM docker.io/library/alpine:latest

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
# (see ADR-0011).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9441/livez || exit 1

USER ppdd

ENTRYPOINT ["/usr/bin/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
