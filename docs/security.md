# Security

groknode is built to run something powerful — a tool that can execute commands
on your machine — as safely as possible.

## Posture

- **Dedicated service user.** The daemon runs as a dedicated `grokbot` system
  user with no login shell. It is not your user and not root.
- **Hardened systemd unit.** `NoNewPrivileges`, `ProtectSystem=strict`,
  `ProtectHome`, an empty `CapabilityBoundingSet`, restricted address families
  and namespaces, and a scoped `ReadWritePaths` sandbox.
- **Filesystem sandbox.** Grok's tasks can only touch directories you
  explicitly grant via `ReadWritePaths` — everything else is read-only or
  hidden. See [Configuration](configuration.md#granting-access-to-your-projects).
- **Credentials at rest.** Credentials and state are written atomically at
  `0600`, owned by the `grokbot` user, never world-readable.
- **Network.** Outbound HTTPS only — there is no inbound listener. Service
  endpoints are validated as HTTPS-only; plain HTTP is refused unless you
  explicitly opt in for localhost.
- **No proprietary bundles.** No vendor code or account state is ever committed
  to the project.

## Reporting a vulnerability

Please report security issues **privately** rather than opening a public issue.
See
[`SECURITY.md`](https://github.com/TillmanBuildsTech/groknode/blob/main/SECURITY.md)
in the repository for the current disclosure process.

## Legal

groknode is an independent, unofficial project. It is **not affiliated with,
endorsed by, or supported by xAI or Cursor.** It uses an undocumented internal
protocol that a vendor update can change at any time. No proprietary code is
redistributed. Licensed under the
[MIT License](https://github.com/TillmanBuildsTech/groknode/blob/main/LICENSE).
