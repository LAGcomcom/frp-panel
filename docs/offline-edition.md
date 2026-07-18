# Update-Center Offline Edition

The offline edition is built from the same source with the `offline` build tag:

```sh
CGO_ENABLED=0 go build -tags offline -trimpath -ldflags="-s -w" -o frp-panel-offline ./cmd/panel/
```

Release artifacts use these names:

- `frp-panel-offline-linux-amd64`
- `frp-panel-offline-linux-arm64`

For complete manual file deployment, optional source rebuilding, service configuration, verification, upgrade, and troubleshooting steps, see [offline-source-deployment.md](offline-source-deployment.md).

The customer ZIP also includes `install-offline.sh`, which installs or updates from the local prebuilt files without downloading the panel.

## Disabled update-center connections

The offline binary clears the update-center configuration before services start. It does not start the update supervisor and does not register these routes:

- update checks and package downloads;
- instance registration and five-second heartbeat reporting;
- signed bootstrap discovery;
- lease renewal and update-center feature checks.

The admin frontend reads the build edition from the local settings API and does not start its update-check timer in offline mode.

## Preserved connections and core behavior

The offline edition keeps local login, users, groups, plans, tunnels, FRPS plugin authentication, per-user/per-plan bandwidth enforcement, traffic accounting, node polling, Agent reporting, and SSH node management.

Only update-center communication is removed. GitHub and configured mirrors remain available for FRPS downloads, and administrator-configured payment and SMTP providers continue to work.
