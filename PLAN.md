# grokbot-node — headless Grok Bot local-computer node (Go)

A single-binary Go rewrite of the `grokbot-headless` management layer, designed to
run the official Grok Bot local-execution daemon on an always-on Linux box with no
system Node.js, no desktop/Electron app integration, and the proprietary runtime
isolated inside a container sandbox.

Status: PLAN (no code yet). Decisions locked: **Go**; target is a **headless
server** where we avoid installing the full desktop package; service runs under a
**dedicated locked-down identity**.

---

## 1. What this is (and the hard constraints)

`grokbot-headless` is not Grok. It is a ~450-line Node supervisor around xAI's
**proprietary** local-exec daemon (internally Cursor's "local computer" tech:
endpoints are `api2.cursor.sh`, headers are `x-cursor-*`, RPC services are
`aiserver.v1.GrokBotService` / `DashboardService`). The daemon ships closed-source
inside the Grok Bot Linux package at
`/opt/Grok Bot/resources/app.asar/dist/local-exec-daemon/main.cjs` and only runs
under the package's bundled Electron binary via `ELECTRON_RUN_AS_NODE=1`.

Our Go program replaces ONLY the management/supervision layer. It performs:
OAuth (PKCE browser-link) login → machine identity → mint daemon + user-computer
credentials from the cloud → launch the daemon headlessly → bootstrap it over Node
IPC → heartbeat + credential renewal + crash restart.

Four constraints that shape the whole design:

1. **The proprietary daemon cannot be removed.** It is the product; the only legal
   source is the official package. "No install" therefore means: no system Node,
   no desktop integration, runtime isolated — NOT a Grok-Bot-free box. A fully
   daemon-free reimplementation would mean reverse-engineering and reimplementing
   xAI's execution protocol end-to-end: huge, brittle, legally grey, and it throws
   away the "thin unofficial controller, zero proprietary code" posture. Explicitly
   out of scope.

2. **Node `fork()` IPC must be reproduced from a non-Node process.** The daemon is
   `fork`ed and expects an inherited socket on fd `NODE_CHANNEL_FD` speaking
   newline-delimited JSON (`NODE_CHANNEL_SERIALIZATION=json`). The parent sends one
   bootstrap message: `{"type":"sand-local-exec-file-key","key":null,"computerId":<machineId>}`.
   This is the single biggest technical risk — see §5.3.

3. **`cursorChecksum` depends on JavaScript 32-bit shift wraparound.** In JS,
   `epoch >> 40` is actually `epoch >> 8` (shift count mod 32). A naive 64-bit Go
   port yields different bytes and the server rejects the request. Must replicate
   `int32` semantics exactly — see §5.4.

4. **Electron links GTK/X11/NSS `.so`s at load.** Even in `ELECTRON_RUN_AS_NODE`
   mode the binary dynamically links those libraries, so they must exist on the
   box. BUT that mode skips Chromium init, so a running X11/Xvfb display is almost
   certainly NOT required (unlike `electron`/`orca serve`). Confirm via spike S1.

---

## 2. Target architecture (recommended: container-isolated daemon)

```
  Host (CachyOS / any distro, clean)
  └─ systemd system unit  grokbot-node.service   (User=grokbot, hardened)
       └─ podman/quadlet (or barge) runs ONE container:
            ┌─────────────────────────────────────────────┐
            │ container image: grokbot-node                │
            │                                              │
            │  PID 1: groknode  (our Go binary)            │
            │     │  socketpair(AF_UNIX,SOCK_STREAM)        │
            │     │  NODE_CHANNEL_FD=3, _SERIALIZATION=json │
            │     ▼                                          │
            │  child: ./grok-bot  (Electron-as-node)        │
            │     └─ main.cjs  local-exec-daemon (xAI)      │
            │                                              │
            │  + extracted app.asar(.unpacked)             │
            │  + GTK/X11/NSS libs (bundled in image)       │
            └─────────────────────────────────────────────┘
                 │ outbound HTTPS → api2.cursor.sh
                 │ volumes: /var/lib/grokbot-node (creds/state)
                 │ optional bind-mounts: project dirs Grok may touch
```

Why container-first:
- Host stays clean; the fat Electron+libs tree never touches the host (crucial on
  Arch where the official `.deb`/`.rpm` won't install natively).
- The container filesystem **is** the locked-down sandbox the identity decision
  asked for: Grok's local-computer tasks run inside it and see only what we mount.
- Distro-agnostic and reproducible; redeployable with barge.
- The image is built locally from the package the user already downloaded — we
  never redistribute proprietary bits.

### Fallback topology B — host-native minimal extract
For when containers aren't wanted: extract the package to `/opt/grokbot-runtime`,
install the GTK/X11/NSS libs via the host package manager, run `groknode` directly
under the dedicated `grokbot` user via the systemd unit in §6. More host pollution,
distro-specific lib names (the Arch equivalents differ from the Debian `libgtk-3-0t64`
etc.), weaker sandbox. Supported but not recommended.

Both topologies share the exact same Go binary and supervision logic; only the
launch wrapper and file paths differ, selected by config.

---

## 3. Project layout

```
grokbot-proxy/                      (repo root; module github.com/TillmanBuildsTech/grokbot-node)
  cmd/groknode/main.go              CLI entrypoint (cobra): login run status check policy logout
  internal/config/                  env + file config, path resolution (system vs user vs container)
  internal/auth/                    PKCE, OAuth token+refresh, JWT payload decode, credential store
  internal/protocol/                checksum, headers, Connect RPC unary client, descriptor mint
  internal/identity/                machine.json create/load
  internal/daemon/                  spawn via Electron-node, Node-IPC channel, lifecycle/terminate
  internal/supervisor/             run loop: heartbeat, refresh, restart w/ backoff, signals
  internal/state/                   atomic 0600 JSON writes, status reads
  internal/compat/                  installed-version probe (asar read or electron -e), tested pins
  packaging/
    Containerfile                   builds image from user-supplied package dir
    systemd/grokbot-node.service    system unit (host→container launcher)
    systemd/groknode.service        system unit (topology B, host-native)
    quadlet/grokbot-node.container  podman quadlet
  test/
    fakebackend/                    httptest server mimicking api2.cursor.sh + oauth + RPC
    fakedaemon/                     tiny node/go stub that speaks Node IPC, asserts bootstrap msg
  PLAN.md  README.md  LICENSE(MIT)  SECURITY.md
```

---

## 4. CLI surface (parity + improvements)

Keep the original verbs so muscle memory and docs transfer:

- `groknode login`   — print browser-link URL, poll `/auth/poll`, store creds.
- `groknode run`     — the supervisor (PID 1 in container / ExecStart in topology B).
- `groknode status`  — JSON: label, machineId, daemon pid/alive, policy, cred expiry.
- `groknode check [--local]` — compatibility probe; `--local` skips cloud auth.
- `groknode policy <always|ask|never>` — register machine + set local tool permission.
- `groknode logout`  — stop service, assert daemon dead, wipe creds/state.

Improvements over the Node version:
- `--json`/structured logging (slog) everywhere; `status` already JSON.
- `groknode doctor` — one-shot: libs present? daemon entry resolvable? IPC bootstrap
  handshake succeeds against the real daemon for N seconds then exits? cloud reachable?
- Exponential backoff + crash-loop detection on daemon restart (Node version is a
  flat 5s). Trip to a cooldown + loud log after K failures in a window.
- Optional local status socket (unix socket `/run/grokbot-node/status.sock`) so the
  host can query health without parsing files.
- Config file (`/etc/grokbot-node/config.toml`) in addition to env, so the systemd
  unit stays clean.

---

## 5. Key implementation details

### 5.1 Config & paths (system-service model)
Dedicated user `grokbot`. Paths relocated off `$HOME`:
- config:  `/etc/grokbot-node/` (config.toml, 0750 root:grokbot)
- state:   `/var/lib/grokbot-node/` (0700 grokbot:grokbot) — credentials.json,
  machine.json, local-exec-daemon-credential.json, heartbeat, discovery, settings.
- runtime: `/run/grokbot-node/` (status socket)
- In the container these map to a single mounted volume.
All secret files written atomically at 0600 (temp + rename), mirroring the Node
`atomicJson`. Overridable via env (`SAND_DATA_ROOT`, `GROK_BOT_HEADLESS_CONFIG_DIR`,
`GROK_BOT_BINARY`, `GROK_BOT_DAEMON_SCRIPT`) for test and container flexibility.

### 5.2 Auth & protocol client
- PKCE: verifier = base64url(32 random bytes); challenge = base64url(sha256(verifier)).
- Login URL on website host `/loginDeepControl?challenge&uuid&mode=login&redirectTarget=sand&supportsSelectedTeamLogin=true`.
- Poll `GET {API}/auth/poll?uuid&verifier` until != 404; backoff 1s→×1.2→cap 10s,
  up to 150 attempts (matches original).
- Refresh: `POST /oauth/token` grant_type=refresh_token when JWT `exp` within 5min;
  honor `shouldLogout`.
- Connect RPC unary = plain `POST /{service}/{method}` with header
  `connect-protocol-version: 1` and JSON body. Calls used:
  `GrokBotService/IssueGrokBotUserComputerCredential`,
  `DashboardService/RegisterSandMachine`, `DashboardService/UpdateSandMachineLocalToolPermission`.
- Daemon credential mint: `POST /sand-box/local-exec-daemon-credential`.
- Write descriptor `local-exec-daemon-credential.json` the daemon reads (daemon
  credential + backendUrl + userComputer{credential,machineId,expiresAtMs,accountScope,serverAuthoritative}).

Auth header set (exact): `authorization: Bearer <access>`, optional
`x-cursor-team-id`, `x-cursor-checksum`, `x-cursor-client-type: sand`,
`x-cursor-client-version: <ver>`, `x-sand-box-namespace: prod`,
`x-ghost-mode: true`, `x-request-id: <uuid>`.

### 5.3 Node-IPC channel (the risk) — concrete Go approach
```
sp, _ := syscall.Socketpair(AF_UNIX, SOCK_STREAM|SOCK_CLOEXEC, 0)  // [parent,child]
parent := os.NewFile(uintptr(sp[0]), "ipc-parent")
child  := os.NewFile(uintptr(sp[1]), "ipc-child")
cmd := exec.Command(appBinary, daemonScript)      // Electron binary as argv0, script as argv1
cmd.Env = append(baseEnv,
   "ELECTRON_RUN_AS_NODE=1", "SAND_PACKAGED=1",
   "SAND_DATA_ROOT="+dataRoot, "SAND_CLIENT_APP_VERSION="+ver,
   "NODE_CHANNEL_FD=3", "NODE_CHANNEL_SERIALIZATION=json")
cmd.ExtraFiles = []*os.File{child}                 // becomes fd 3 in child
cmd.Stdout, cmd.Stderr = log, log                  // stdin ignored
cmd.Start(); child.Close()                          // parent keeps sp[0]
// on child 'spawn': write bootstrap, newline-framed JSON
parent.Write(append(json.Marshal(bootstrapMsg), '\n'))
```
Notes / unknowns to validate in spike S2:
- Confirm the daemon uses default `json` serialization (newline-framed) vs `advanced`
  (v8-serialized, length-prefixed). Original passes no serialization override and
  relies on Node's default, which is `json` — replicate that; be ready to implement
  the `advanced` framing if the daemon forces it.
- Confirm the daemon never sends OS handles over IPC (SCM_RIGHTS). A SOCK_STREAM
  unix socket can carry them if needed, but our reader should at minimum not choke.
- Match Node's message framing exactly: one JSON doc per `\n`-terminated line; the
  reader must buffer partial lines.

### 5.4 cursorChecksum — JS-faithful port
```
epoch := int32(nowMs / 1_000_000)                 // deliberately 32-bit
b := []byte{ byte(epoch>>40 & 255 ... }            // WRONG in Go
```
Correct: shift counts are taken mod 32 in JS. Implement as:
```
e := uint32(nowMs / 1_000_000)
sh := func(n uint) byte { return byte((e >> (n % 32)) & 255) }
bytes := []byte{sh(40), sh(32), sh(24), sh(16), sh(8), sh(0)}
prev := byte(165)
for i := range bytes { bytes[i] = byte((int(bytes[i]^prev)+i) & 255); prev = bytes[i] }
return base64url(bytes) + machineId
```
Add a golden-vector unit test pinned against the Node implementation's output for a
fixed timestamp so regressions are caught immediately.

### 5.5 Compatibility probe without launching the GUI
Two options for reading the installed version (inside app.asar):
- (a) shell out `appBinary -e '...require(pkgJson).version...'` with
  `ELECTRON_RUN_AS_NODE=1` (what Node version does; simplest, always correct).
- (b) implement a tiny asar reader in Go (asar = 8-byte header + JSON index +
  concatenated payload) and read `package.json` directly (faster, no subprocess).
Start with (a); add (b) later if startup latency matters. Pin `testedVersions`
(currently `0.30.0`) and warn (don't fail) on mismatch.

### 5.6 Supervisor loop
`run`: assert app present → mint descriptor → write heartbeat → spawn daemon →
goroutines: heartbeat every 20s, descriptor refresh every 60s (re-mint, which also
refreshes OAuth), daemon-exit watcher with backoff restart. SIGTERM/SIGINT →
disconnect IPC, SIGTERM child, 5s grace, SIGKILL, remove heartbeat, exit. Use
`context.Context` cancellation rather than a boolean, and `errgroup`.

---

## 6. systemd (system service) + hardening

Topology A unit launches the container (podman quadlet preferred). Topology B unit
(host-native) runs the binary directly:

```
[Unit]
Description=Headless Grok Bot local-computer node
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=grokbot
Group=grokbot
ExecCondition=/usr/bin/groknode check --local
ExecStart=/usr/bin/groknode run
Restart=on-failure
RestartSec=5s
TimeoutStopSec=15s
KillMode=mixed
# hardening (relax only what the daemon provably needs)
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/grokbot-node /run/grokbot-node
StateDirectory=grokbot-node
RuntimeDirectory=grokbot-node
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
SystemCallFilter=@system-service
```
CAUTION: the local-exec daemon's whole job is running approved commands / file ops.
Hardening that is too aggressive will break legitimate tasks. The sandbox boundary
should come from the container (topology A) or from `ProtectHome`/`ReadWritePaths`
scoping (topology B) — NOT from a syscall filter so tight the daemon can't spawn
subprocesses. Treat the hardening set as a tune-with-tests item, not fire-and-forget.

Startup before desktop login: a system unit needs no `loginctl enable-linger`
(that was a consequence of the Node version using `systemctl --user`). This is a
real improvement from moving to a true system service.

---

## 7. Security model
- Credentials at 0600, owned by `grokbot`, never world-readable; atomic writes.
- Service URLs validated: HTTPS-only (localhost-http only behind an explicit
  opt-in flag), reject embedded userinfo — port the Node `validatedServiceUrl`.
- No proprietary code or extracted bundles in the repo; image built locally from
  the user's own package. `.gitignore` the package dir and all state.
- Outbound only; no inbound listener except the optional local unix status socket.
- Document the data that flows to xAI (same as using the official app).

---

## 8. Testing
Mirror the original's philosophy (it ships protocol-helper tests + isolated
installer test + e2e against fake backend/daemon):
- Unit: checksum golden vectors, PKCE, JWT decode, token-expiry logic, URL validation,
  atomic-write perms, config/path resolution.
- IPC conformance (S2): a fake daemon stub that reads `NODE_CHANNEL_FD`, asserts it
  receives exactly the bootstrap message newline-framed, echoes back; our supervisor
  must drive it. Run the same test against the REAL daemon in a gated integration job.
- Fake backend: httptest server implementing `/oauth/token`, `/auth/poll`,
  `/loginDeepControl`, the three RPC methods, and daemon-credential mint; assert
  headers (checksum format, client-type, request-id) and the minted descriptor shape.
- Reuse your existing gates: `make ci`, `golangci-lint run`, `govulncheck`,
  `go test -race`, integration behind a build tag. CI matrix should include a job
  that builds the Containerfile.

---

## 9. Packaging & distribution
- `groknode` as a single static binary (CGO_OFF) — GoReleaser for checksummed
  release artifacts (matches your barge conventions).
- `Containerfile` that takes the user's extracted package dir as build context /
  arg, layers in the GTK/X11/NSS libs, drops in `groknode`, sets it as entrypoint.
- Podman quadlet + system unit for host deployment; optional `barge` deploy recipe.
- `groknode install` subcommand to lay down user, dirs, and unit for topology B.

---

## 10. Phased roadmap
1. **Spikes (validate the risky unknowns before committing):**
   - S1: does the real daemon run under `ELECTRON_RUN_AS_NODE` with libs-but-no-Xvfb?
   - S2: Node-IPC handshake from Go — default `json` framing, no handle passing?
   - S3: confirm the Grok Bot Linux package format/URL and what's inside app.asar.unpacked
     (native `.node` modules → confirms we must use the Electron binary as runtime).
2. **Protocol core:** auth + checksum + RPC client + descriptor mint, all unit-tested
   against the fake backend. (Pure, no daemon needed — highest confidence first.)
3. **Daemon lifecycle:** spawn + IPC + supervise against fake daemon, then real daemon.
4. **CLI + system service + hardening tuning** (topology B working end-to-end).
5. **Containerize** (topology A), quadlet/barge deploy, docs.
6. **Hardening pass, doctor command, release tooling.**

Ship 2→3→4 as a working host-native node first; containerization (5) is the polish
that delivers the "clean host" goal.

## 11. Open questions / spikes to resolve (carry into build)
- S1/S2/S3 above.
- Does `logout`/`policy` need the systemd user-service semantics replaced by
  `systemctl` (system) calls, or should the CLI stay service-manager-agnostic?
- "Communicate over the local network": confirm intent. The node talks to xAI's
  cloud; the win is the local-computer capability surviving your desktop being off.
  If you also want it to act on OTHER LAN hosts, that's bind-mounts / network scope
  on the container, not a protocol change.

## 12. Legal / ToS
Independent, unofficial, uses an undocumented internal protocol; an official update
can break it (same caveat as grokbot-headless). No proprietary code redistributed.
MIT, consistent with the upstream project we're reimplementing the open layer of.
