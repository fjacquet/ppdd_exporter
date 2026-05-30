# Docker deployment

The image is distroless and runs as a non-root user.

```bash
docker run -d --name ppdd_exporter -p 9099:9099 \
  -e DD01_PASSWORD=secret \
  -v /etc/ppdd_exporter/config.yaml:/etc/ppdd_exporter/config.yaml:ro \
  ghcr.io/fjacquet/ppdd_exporter:latest
```

Health and metrics are on the same port (`/health`, `/metrics`).
