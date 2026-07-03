# systemd (EL9 host)

Docker is **not** required — the exporter is a single static (`CGO_ENABLED=0`) binary. For a
non-container deployment on Enterprise Linux 9, use the unit shipped in `deploy/`.

## Install

```bash
# user + binary
sudo useradd --system --no-create-home --shell /usr/sbin/nologin ppdd
sudo install -m 0755 bin/ppdd_exporter /usr/local/bin/ppdd_exporter

# config + secrets
sudo install -d -o root -g ppdd -m 0750 /etc/ppdd_exporter
sudo install -m 0640 -o root -g ppdd config.yaml /etc/ppdd_exporter/config.yaml
sudo install -m 0600 -o root -g ppdd deploy/ppdd_exporter.env.example /etc/ppdd_exporter/ppdd_exporter.env
# edit /etc/ppdd_exporter/ppdd_exporter.env to set PPDD1_PASSWORD=...

# service
sudo install -m 0644 deploy/ppdd_exporter.service /etc/systemd/system/ppdd_exporter.service
sudo systemctl daemon-reload
sudo systemctl enable --now ppdd_exporter
```

Set `logName: ""` in `config.yaml` so logs go to the journal.

## Operate

```bash
journalctl -u ppdd_exporter -f         # follow logs
sudo systemctl reload ppdd_exporter    # live config reload (sends SIGHUP)
sudo systemctl status ppdd_exporter
```

## Hardening

The unit runs as the unprivileged `ppdd` user inside a sandbox:

- `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`
- `PrivateTmp`, `PrivateDevices`, `ProtectKernel*`, `ProtectControlGroups`
- `RestrictAddressFamilies=AF_INET AF_INET6`, `RestrictNamespaces`, `LockPersonality`
- `Restart=on-failure`

Secrets are supplied through the `EnvironmentFile` and referenced as `${PPDD1_PASSWORD}`
in `config.yaml`. Keep that file mode `0600`.

## macOS (launchd / Homebrew)

On macOS run it under **launchd** (the systemd equivalent). `brew services` is not wired up:
the Homebrew cask only installs the binary on your PATH — it defines no service block — so
register a `launchd` job yourself, e.g. `~/Library/LaunchAgents/com.fjacquet.ppdd_exporter.plist`
with `ProgramArguments` `[/opt/homebrew/bin/ppdd_exporter, --config, <path>/config.yaml]` and
`RunAtLoad`/`KeepAlive` set, then `launchctl load` it.
