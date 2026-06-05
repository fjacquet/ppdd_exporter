# Installation

Requires a reachable Dell PowerProtect DD appliance and a DD user with read access.
To build from source you need a Go 1.26+ toolchain.

## With Homebrew (macOS)

The cask is published from the `fjacquet/homebrew-tap` tap on each release.
**Homebrew casks are macOS-only** — on Linux, use the [release archive](#from-a-release-archive),
the [container image](#container-image), or `go install` instead.

```bash
# Install (the tap is added automatically by the qualified name):
brew install --cask fjacquet/tap/ppdd_exporter

# ...or add the tap once, then install/upgrade by short name:
brew tap fjacquet/tap
brew install --cask ppdd_exporter

# Upgrade to the latest release:
brew upgrade --cask ppdd_exporter

# Verify and uninstall:
ppdd_exporter --version
brew uninstall --cask ppdd_exporter
```

The binary lands on your `PATH` (Homebrew strips the macOS quarantine bit
automatically, so Gatekeeper won't block it). Pair it with a `config.yaml` — see
[Configuration](configuration.md).

## From a release archive

Each release ships both `tar.gz` archives (bundling `LICENSE`, `README.md`, and a sample
`config.yaml`) and the raw binaries, for all four `os/arch` targets. Download from the
[releases page](https://github.com/fjacquet/ppdd_exporter/releases), verify against
`checksums.txt`, then extract and install:

```bash
sha256sum -c checksums.txt --ignore-missing
tar -xzf ppdd_exporter_*_linux_amd64.tar.gz
sudo install ppdd_exporter /usr/local/bin/ppdd_exporter
ppdd_exporter --version
```

Prefer the bare binary? Download `ppdd_exporter_<version>_linux_amd64` directly, `chmod +x`,
and run it. Each release also ships a CycloneDX SBOM (`ppdd_exporter_<version>.sbom.cdx.json`).

## From source

```bash
git clone https://github.com/fjacquet/ppdd_exporter
cd ppdd_exporter
make cli            # -> bin/ppdd_exporter
./bin/ppdd_exporter --version
```

## Container image

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR with SBOM and
provenance attestations:

```bash
docker pull ghcr.io/fjacquet/ppdd_exporter:0.2.0   # or :latest

docker run --rm -p 9099:9099 \
  -e DD01_PASSWORD=secret \
  -v "$PWD/config.yaml:/etc/ppdd_exporter/config.yaml:ro" \
  ghcr.io/fjacquet/ppdd_exporter:latest
```

## Next steps

- [Configure](configuration.md) your appliances.
- [Quick start](quickstart.md) to run it.
- Deploy via [Docker](../deployment/docker.md) or the [Compose demo](../deployment/compose-demo.md).
