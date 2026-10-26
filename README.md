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
 grokbotd (Go)                     stock Electron (BYO, run_as_node enabled)
   | mints credentials (api2.cursor.sh)        |
   | writes descriptor  -------------------->  | runs vendor dist/local-exec-daemon/main.cjs
   | Node-IPC bootstrap (socketpair, fd3) ---> | (ELECTRON_RUN_AS_NODE=1, headless)
   | heartbeat + renew + restart               |
```

grokbotd authenticates (PKCE browser-link), registers a machine identity, mints
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

- Implemented & validated against the real 0.75.8 install on Linux/Arch:
  compatibility detection (asar version read, required-Electron-major scan,
  runtime discovery), Node-IPC transport, headless daemon launch, checksum and
  protocol client (vs a fake backend).
- Needs a real account to exercise end-to-end: `login`, full `check`, `policy`,
  and `run` holding ownership. Intended target: an always-on LXC/VM.

## Build & test

```
go build ./...
go test ./...            # unit + guarded real-install tests
go build -o grokbotd ./cmd/grokbotd
```

## Install as a system service

```
sudo ./scripts/install.sh            # dedicated user, hardened unit, wrapper
grokbotctl fetch-runtime             # download matching stock Electron
grokbotctl check --local             # verify compatibility
grokbotctl login                     # browser-link sign-in (prints URL)
grokbotctl policy ask                # always | ask | never
sudo systemctl start grokbot-node
```

Prerequisite: the official Grok Bot Linux package installed on the box (provides
the daemon). After any Grok Bot update: `grokbotctl fetch-runtime && sudo systemctl restart grokbot-node`.

## Commands

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
