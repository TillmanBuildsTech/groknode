# Contributing to groknode

Thanks for your interest in improving groknode. This document covers the
architecture, how to build and test, and the diagnostics tooling. For what the
project is and how to use it, see the [README](README.md).

---

## Architecture

groknode is a native Go rewrite of the local-computer management layer. It
replaces **only** the supervision/management code; the actual local-exec daemon
is xAI's proprietary code and is never reimplemented, bundled, or committed.

```
 groknode (Go)                     stock Electron (BYO, run_as_node enabled)
   | mints credentials (api2.cursor.sh)        |
   | writes descriptor  -------------------->  | runs vendor dist/local-exec-daemon/main.cjs
   | Node-IPC bootstrap (socketpair, fd3) ---> | (ELECTRON_RUN_AS_NODE=1, headless)
   | heartbeat + renew + restart               |
```

The flow: authenticate (PKCE browser-link) → register a machine identity →
mint the daemon + user-computer credentials from the cloud → launch the vendor
daemon under an ABI-matched Electron runtime → bootstrap it over Node IPC →
heartbeat + credential renewal + crash-restart supervision.

### Why we bring our own Electron

The official desktop binary (`/opt/Grok Bot/grok-bot`) on current builds
(0.75.8) has the Electron **`run_as_node` fuse DISABLED**, so
`ELECTRON_RUN_AS_NODE=1` is ignored and it cannot be used as a Node runtime —
the approach the original project relied on.

Instead groknode runs the vendor's daemon code under a **stock Electron of the
same major version**. Stock Electron ships `run_as_node` enabled, and the same
major version means the same Node ABI, so it loads the vendor's prebuilt native
modules. The official Grok Bot package is still required — it is the only legal
source of the proprietary daemon — but it is never executed.

See [`archived/SPIKES.md`](archived/SPIKES.md) for the full de-risking
investigation and [`PLAN.md`](PLAN.md) for the complete design.

### Key implementation gotchas

- **Node `fork()` IPC from a non-Node parent.** The daemon expects an inherited
  socket on fd `NODE_CHANNEL_FD` speaking newline-delimited JSON
  (`NODE_CHANNEL_SERIALIZATION=json`). groknode creates a `socketpair`, passes
  the child end as fd 3 via `exec.Cmd.ExtraFiles`, and writes one bootstrap
  message. Implemented in `internal/ipc`.
- **`cursorChecksum` relies on JS 32-bit shift wraparound.** In JS, shift counts
  are taken mod 32, so a naive 64-bit Go port yields different bytes and the
  server rejects the request. The port replicates `int32` semantics exactly and
  is pinned with golden-vector tests in `internal/protocol`.
- **Electron links GTK/X11/NSS at load.** Even in `run_as_node` mode the binary
  dynamically links those libraries, so they must be present — but that mode
  skips Chromium init, so no X11/Xvfb display is required.

### Repo layout

```
cmd/groknode/        CLI entrypoint (login run status check policy logout + diagnostics)
internal/auth/       PKCE, OAuth token/refresh, credential store
internal/protocol/   checksum, Connect RPC client, descriptor mint, flows
internal/ipc/        Node-IPC channel (socketpair, fd3, newline-framed JSON)
internal/daemon/     daemon spawn + lifecycle
internal/supervisor/ run loop: heartbeat, refresh, restart w/ backoff, signals
internal/compat/     installed-version probe, runtime discovery, lib preflight, drift checks
internal/config/     env + file config, path resolution
internal/state/      atomic 0600 JSON writes, status reads
packaging/systemd/   the hardened system unit
scripts/install.sh   installer (distro/arch detection, Grok Bot download, service setup)
```

---

## Build & test

```
make build              # produces ./groknode (CGO_ENABLED=0)
go build ./...
go test ./...           # unit + guarded real-install tests
go test -race ./...     # concurrency check (IPC reader goroutine)
```

### Full gate set

Mirrors the barge project conventions. Run before opening a PR:

```
make ci                 # build + vet + lint + vuln + race
```

Individual gates:

```
make vet                # go vet ./...
make lint               # golangci-lint (needs $(go env GOPATH)/bin/golangci-lint)
make vuln               # govulncheck (needs $(go env GOPATH)/bin/govulncheck)
make race               # go test -race ./...
make fmt                # gofmt -w .
```

`golangci-lint` and `govulncheck` install into `$(go env GOPATH)/bin`; make
sure that's on your `PATH`.

### Testing philosophy

- **Unit:** checksum golden vectors, PKCE, JWT decode, token-expiry logic, URL
  validation, atomic-write perms, config/path resolution.
- **IPC conformance:** a fake daemon stub asserts it receives exactly the
  bootstrap message, newline-framed; the same test runs against the real daemon
  in a gated integration job.
- **Fake backend:** an `httptest` server implements the OAuth, poll, RPC, and
  daemon-credential-mint endpoints; tests assert header format (checksum,
  client-type, request-id) and the minted descriptor shape.

---

## Status

- **Installer:** fully tested on Debian; auto-downloads the latest stable Grok
  Bot from Cursor's official update feed, installs all runtime libs, configures
  the systemd service, and handles missing dependencies automatically.
- **Controller MVP:** validated against the real 0.75.8 install on Linux/Arch —
  compatibility detection (asar version read, required-Electron-major scan,
  runtime discovery), Node-IPC transport, headless daemon launch, and the
  checksum + protocol client (against a fake backend).
- **End-to-end:** once Grok Bot is on disk, `fetch-runtime` → `login` →
  `policy` → `systemctl start groknode` is expected to work with a real account
  on an always-on LXC/VM.

See [`PLAN.md`](PLAN.md) for the phased roadmap.

---

## Drift watch

A nightly GitHub Actions job (`.github/workflows/nightly.yml`) downloads the
current stable Grok Bot and verifies the assumptions groknode relies on:
package layout, the in-asar daemon entry, the embedded Electron version, and
that a matching stock Electron is still published. If Cursor ships a breaking
change, it opens a tracking issue before users hit it. Keep this job green — it
is the project's early-warning system.

---

## Diagnostics

Beyond the user-facing commands, `groknode` exposes low-level probes for
debugging protocol and runtime issues:

| Command | Purpose |
|---------|---------|
| `groknode checksum <machineId>` | Print the computed `cursorChecksum` |
| `groknode ipc-probe <runtime> <script> [--run-as-node]` | Exercise the Node-IPC bootstrap against a runtime |
| `groknode daemon-probe <runtime> <daemon.cjs> <clientVersion> <seconds>` | Drive the real daemon for N seconds and report |
| `groknode run` | The supervisor loop (the systemd service entrypoint) |

---

## Pull requests

- Keep changes focused; match existing style.
- Run `make ci` and make sure it passes.
- Add or update tests for behavior you change — especially protocol/checksum
  code, which is pinned with golden vectors.
- Never commit proprietary bundles, extracted package contents, or account
  state. The `.gitignore` covers the usual paths; double-check before pushing.
