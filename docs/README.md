# groknode

**Keep Grok's local-computer capability running 24/7 — without leaving your desktop on.**

groknode is a single, self-contained Go binary that runs Grok Bot's official
local-computer daemon headlessly on an always-on Linux box — a VM, an LXC, a
home server, a Raspberry Pi. Install it once, sign in, and Grok can keep
working on your machine whenever you need it, not just while your laptop is
awake with the app open.

It is an independent, unofficial management layer. It ships **no proprietary
code**, drives xAI's own daemon, and is MIT-licensed and fully open source.

> Ready to install? Jump to **[Install &amp; setup](getting-started.md)**.

## Why you'd want this

Grok Bot's "local computer" feature lets Grok run commands and work with files
on a machine you control. Out of the box, that machine has to be *your
desktop*, with the Grok Bot app open and your screen on. Close the laptop and
the capability goes away.

groknode moves that capability onto a box that's always on:

- **Always available.** Grok's local-computer tasks keep working whether or not
  your desktop is awake. Run it on a cheap VM or an LXC and forget about it.
- **No desktop, no display server.** Runs completely headless — no X11, no
  Xvfb, no GUI. Just the binary and a systemd service.
- **One command to install.** The installer detects your distro and
  architecture, pulls the latest official Grok Bot release, installs every
  runtime library it needs, and wires up a hardened system service.
- **Set-and-forget supervision.** It signs in, mints and renews credentials
  automatically, launches the daemon, watches it, and restarts it with
  back-off if it ever crashes.
- **Runs locked down.** A dedicated, no-login service user; credentials stored
  at `0600`; a hardened systemd unit; outbound connections only — no inbound
  listener.

## Can you trust it?

That's the right question to ask of anything that can run commands on your
machine. Here's the honest answer:

- **It's fully open source (MIT).** Every line that runs as groknode is in the
  repository. Read it, audit it, build it yourself.
- **It redistributes no proprietary code.** The actual local-computer daemon is
  xAI's, and it comes only from the official Grok Bot package you install on
  your own box. groknode never bundles, ships, or modifies it — it only
  supervises it.
- **Your data goes exactly where it already did.** groknode talks to the same
  xAI/Cursor cloud endpoints the official app uses, with the same account. It
  adds no telemetry and no third parties.
- **It's locked down by default.** The service runs as a dedicated `grokbot`
  user with no login shell, under a hardened systemd unit. Grok's tasks can
  only touch directories you explicitly grant.
- **It warns you before it breaks.** A nightly automated check downloads the
  current Grok Bot release and verifies everything groknode depends on — so
  breakage gets caught and tracked *before* it reaches you.

It is **unofficial** and **not endorsed by xAI or Cursor**. Because it rides an
undocumented internal protocol, a future Grok Bot update can break it until
groknode is updated to match. That's the one real caveat, and the nightly
watch exists specifically to manage it.

## How it works (in brief)

groknode handles the sign-in, credential, and supervision work, and hands the
actual local-computer execution to xAI's own daemon:

1. Signs you in (browser link), registers a machine identity, and mints the
   credentials the daemon needs — renewing them automatically before they
   expire.
2. Launches the official daemon headlessly and connects to it over a local
   channel.
3. Supervises it: heartbeats, credential renewal, and crash-restart with
   back-off, all under a hardened systemd service.

The daemon itself is proprietary and comes only from the Grok Bot package on
your machine; groknode never executes the official desktop app and never ships
any of its code.
