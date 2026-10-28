# grokbot-node

A single-binary Go controller that runs the official Grok Bot local-computer
daemon headlessly on an always-on Linux box — so the capability keeps working
when your desktop is off. It is an independent, unofficial management layer; it
ships no proprietary code and drives xAI's own daemon over its internal protocol.

A native rewrite of the idea behind
[`grokbot-headless`](https://github.com/andreanjos/grokbot-headless), with one
essential change forced by current Grok Bot builds (see below).

## How it works

```
 groknode (Go)                     stock Electron (BYO, run_as_node enabled)
   | mints credentials (api2.cursor.sh)        |
   | writes descriptor  -------------------->  | runs vendor dist/local-exec-daemon/main.cjs
   | Node-IPC bootstrap (socketpair, fd3) ---> | (ELECTRON_RUN_AS_NODE=1, headless)
   | heartbeat + renew + restart               |
```

groknode authenticates (PKCE browser-link), registers a machine identity, mints
the daemon + user-computer credentials, launches the vendor daemon under an
ABI-matched Electron runtime, bootstraps it over Node IPC, and supervises it.

## The key constraint (why we bring our own Electron)

The official desktop binary (`/opt/Grok Bot/grok-bot`) on current builds (0.75.8)
has the Electron **`run_as_node` fuse DISABLED**, so `ELECTRON_RUN_AS_NODE=1`
is ignored and it cannot be used as a Node runtime — the approach the original
project relied on. Instead we run the vendor's daemon code under a **stock
Electron of the same major version** (stock Electron ships `run_as_node`
enabled; same major = same Node ABI, so it loads the vendor's prebuilt native
modules). The official package is still required — it is the only source of the
proprietary daemon — but it is never executed. See `SPIKES.md` for the full
investigation and `PLAN.md` for the design.

## Status

- **Installer: fully tested** on Debian, auto-downloads the latest stable Grok
  Bot from Cursor's official update feed, installs all Electron runtime libs,
  configures systemd service, handles missing dependencies automatically.
- **Controller MVP: validated** against the real 0.75.8 install on Linux/Arch:
  compatibility detection (asar version read, required-Electron-major scan,
  runtime discovery), Node-IPC transport, headless daemon launch, checksum and
  protocol client (vs a fake backend).
- **End-to-end flow ready:** once Grok Bot is on disk, `groknode fetch-runtime` →
  `login` → `policy` → `systemctl start grokbot-node` should work with a real
  account on an always-on LXC/VM.

## Build & test

```
go build ./...
go test ./...            # unit + guarded real-install tests
make build               # produces ./groknode
```

## Install as a system service

### Minimal install (Grok Bot already present)

```
sudo ./scripts/install.sh            # dedicated user, hardened unit, wrapper
```

### Full install (downloads Grok Bot + all dependencies)

```
sudo ./scripts/install.sh --with-grokbot     # download & install Grok Bot + libs
```

The installer auto-detects your distro and architecture, then:
- Resolves the latest stable release from Cursor's update feed and downloads the
  appropriate Grok Bot package (.deb, .rpm, or AppImage)
- Installs all required Electron runtime libraries (no display server needed)
- Configures the systemd service

The installer puts a single command on your `$PATH` — **`groknode`** — which
runs everything as the dedicated service user with the right environment. (The
daemon binary itself lives in `/usr/local/libexec/grokbot-node/` and is invoked
only by systemd; you never call it directly. The vendor's own `grok-bot` binary
is separate and unrelated to this project.)

After install, continue with:

```
groknode fetch-runtime               # download matching stock Electron
groknode check --local               # verify compatibility
groknode login                       # browser-link sign-in (prints URL)
groknode policy ask                  # always | ask | never
sudo systemctl start grokbot-node
```

**Supported platforms:**
- Debian/Ubuntu (x64, arm64) — `.deb` packages
- Fedora/RHEL (x64, arm64) — `.rpm` packages  
- Arch/CachyOS/Manjaro (x64, arm64) — AppImage

After any Grok Bot update: `groknode fetch-runtime && sudo systemctl restart grokbot-node`.

## Commands

All commands are subcommands of `groknode` (e.g. `groknode status`):

| Command | Purpose |
|---------|---------|
| `login` | PKCE browser-link sign-in; stores credentials |
| `run` | Supervise the daemon (service entrypoint) |
| `status` | JSON node/daemon status |
| `check [--local]` | Compatibility (and auth) check |
| `policy <always\|ask\|never>` | Set local tool permission |
| `fetch-runtime` | Download the matching stock Electron |
| `logout` | Remove credentials/state |
| `checksum` / `ipc-probe` / `daemon-probe` | Diagnostics |

## Legal

Independent and unofficial; not endorsed by xAI/Cursor. Uses an undocumented
internal protocol that a vendor update can change. MIT licensed.
