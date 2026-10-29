# Security Policy

groknode supervises a daemon that can run commands and touch files on the
machine it's installed on. We take its security posture seriously and welcome
responsible disclosure.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately through GitHub's
[security advisories](https://github.com/TillmanBuildsTech/groknode/security/advisories/new)
("Report a vulnerability"). Include:

- a description of the issue and its impact,
- steps to reproduce (a proof of concept if you have one),
- affected version/commit and platform.

We aim to acknowledge a report within 72 hours and to provide a remediation
plan or assessment within 14 days. Please give us a reasonable window to ship a
fix before any public disclosure. We're happy to credit reporters who want it.

## Supported versions

groknode is pre-1.0 and moves with upstream Grok Bot. Security fixes land on
`main` and in the latest tagged release only. Because the project rides an
undocumented vendor protocol, running a current build is itself part of staying
secure — older builds may break or fall out of step with upstream.

## Scope

**In scope** — issues in groknode's own code and packaging:

- credential handling (minting, storage, renewal, file permissions),
- the OAuth/PKCE login flow and token refresh,
- URL/endpoint validation (HTTPS enforcement, userinfo rejection),
- the Node-IPC bootstrap and daemon launch path,
- privilege boundaries: the dedicated service user, the systemd hardening, the
  `groknode` setuid-style wrapper, and `ReadWritePaths` scoping,
- the installer (`scripts/install.sh`), which runs as root and downloads the
  official Grok Bot package.

**Out of scope** — not groknode's code, report to the vendor instead:

- vulnerabilities in the proprietary xAI/Cursor local-exec daemon, Grok Bot, or
  Electron themselves — groknode supervises that daemon but neither ships nor
  modifies it,
- the fact that the local-exec daemon executes approved commands and file
  operations: that is its designed purpose, not a flaw. Issues in how groknode
  *scopes or constrains* that execution are in scope; the capability existing is
  not.

## Security model

What groknode does to protect you:

- **No proprietary code.** groknode ships none of xAI's code. The daemon comes
  only from the official package on your machine; it is supervised, never
  modified, and never committed to this repo.
- **Least privilege.** The service runs as a dedicated `grokbot` user with no
  login shell, under a hardened systemd unit: `NoNewPrivileges`,
  `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, an empty
  `CapabilityBoundingSet`, `RestrictNamespaces`, `LockPersonality`, and a scoped
  `ReadWritePaths` sandbox. Grok's tasks can only reach paths you explicitly add.
- **Credentials at rest.** Credential and state files are written atomically
  (temp + rename) at `0600`, owned by the service user, never world-readable.
- **Network posture.** Outbound HTTPS only, to the same xAI/Cursor endpoints the
  official app uses. No inbound listener (the only local socket is an optional
  unix status socket). Service URLs are validated as HTTPS-only — plain HTTP is
  refused unless explicitly opted in for localhost — and URLs with embedded
  userinfo are rejected.
- **No added telemetry.** groknode sends nothing to any third party. The data
  that reaches xAI is the same data that reaches it when you use the official
  app with the same account.
- **No secrets in the repo.** The `.gitignore` excludes state, credentials, and
  any extracted package contents. Never commit those.

## Your responsibilities

- Keep the host patched and restrict who can log into it — anyone with root on
  the box can read the service user's credentials.
- Only add directories you trust Grok to modify to `ReadWritePaths`.
- Understand that `policy always` lets local-computer actions run without
  per-action confirmation. Use `ask` if you want a prompt in the loop.
- Review `scripts/install.sh` before running it as root; it downloads the
  official Grok Bot package over HTTPS from Cursor's update feed.
