# Installation

## Binary

```bash
make cli
./bin/ppdd_exporter --version
```

## Docker

```bash
docker build -t ppdd_exporter:dev .
docker run --rm -p 9099:9099 \
  -e DD01_PASSWORD=secret \
  -v "$PWD/config.yaml:/etc/ppdd_exporter/config.yaml:ro" \
  ppdd_exporter:dev
```

Requires Go 1.26+ to build from source.
