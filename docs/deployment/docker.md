# Docker deployment

The image is distroless and runs as a non-root user.

```bash
docker run -d --name ppdd_exporter -p 9441:9441 \
  -e PPDD1_HOSTNAME=dd01.example.com \
  -e PPDD1_USERNAME=ppdd-monitor \
  -e PPDD1_PASSWORD=secret \
  -v /etc/ppdd_exporter/config.yaml:/etc/ppdd_exporter/config.yaml:ro \
  ghcr.io/fjacquet/ppdd_exporter:latest
```

The default `config.yaml` references the host, user, and password as `${PPDD1_*}`, so
pass each one into the container (`-e …`, or `--env-file .env`) — every `${VAR}` the
config references must exist in the container's environment or the exporter exits at
load with `unset environment variable(s): …`. (Alternatively, bake the literal host and
username into `config.yaml` and pass only `-e PPDD1_PASSWORD`.)

Health and metrics are on the same port (`/health`, `/metrics`).

## Full stack on a server (Compose)

`docker-compose.server.yml` runs the exporter against a **real** PowerProtect DD
appliance alongside Prometheus and Grafana — the same stack as the laptop demo
(`docker-compose.yml`) but without the bundled `mockdd` fake appliance, with secrets
read from a gitignored `.env`, and with Grafana reachable from other machines.

```bash
cp .env.example .env        # set PPDD1_PASSWORD, Grafana creds, and GF_SERVER_ROOT_URL
$EDITOR config.yaml         # set your appliance host(s) under `systems:`
docker compose -f docker-compose.server.yml up -d
```

Open Grafana at the URL you set in `GF_SERVER_ROOT_URL` (its admin login comes from
`.env`) and pick **PowerProtect DD — Overview**. The exporter's `${PPDD1_HOSTNAME}`,
`${PPDD1_USERNAME}`, and `${PPDD1_PASSWORD}` are all forwarded from `.env` into the
container; each fails fast if unset. Add more systems to `config.yaml`? Forward their
vars too in `docker-compose.server.yml` (or switch that block to `env_file: .env`).

`GF_SERVER_ROOT_URL` must point at how you actually reach the server (e.g.
`http://dd-mon.example.com:3000`), not `localhost` — otherwise Grafana's login
redirects and share links break when accessed remotely.

> The stack stores no data in named volumes, so Prometheus history and Grafana
> changes are lost on `down`. Add volumes if you need persistence.
