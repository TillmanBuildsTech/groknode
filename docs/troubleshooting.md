# Troubleshooting

Start every diagnosis with:

```bash
groknode check --local      # compatibility, runtime, and shared libs (no cloud)
groknode check              # the above plus a live sign-in check
journalctl -u groknode -n 50 --no-pager
```

## The service won't start / shows `inactive (dead)`

Look for this in `systemctl status groknode`:

```
Condition: start condition failed
  └─ ConditionPathExists=/var/lib/groknode/.config/groknode/credentials.json was not met
```

This is expected on a box that hasn't signed in yet. The service deliberately
refuses to start without credentials. Run `groknode login`, then
`sudo systemctl start groknode`.

## Missing shared libraries

The Electron runtime links GTK/X11/NSS at load even in headless mode. On a slim
server or LXC these may be absent. `groknode check --local` reports them under
`missingSharedLibs` with an install hint, for example:

```bash
# Debian / Ubuntu
sudo apt-get install -y libgtk-3-0 libnss3 libatk1.0-0 libatk-bridge2.0-0 \
  libgbm1 libasound2 libxkbcommon0 libdrm2 libcups2 libatspi2.0-0 \
  libpango-1.0-0 libcairo2 libx11-6 libxcomposite1 libxdamage1 libxrandr2 \
  libxtst6 libxfixes3

# Arch / CachyOS
sudo pacman -S --needed gtk3 nss atk at-spi2-atk at-spi2-core mesa alsa-lib \
  libxkbcommon libdrm libcups pango cairo libx11 libxcomposite libxdamage \
  libxrandr libxtst libxfixes
```

No display server or Xvfb is required — only the libraries themselves.

## `electronRuntime: MISSING` / no compatible runtime

Fetch the stock Electron that matches the installed Grok Bot:

```bash
groknode fetch-runtime
groknode check --local      # electronRuntime should now resolve to a path
```

If you keep Electron somewhere specific, pin it with `GROKBOT_ELECTRON` in
[`/etc/groknode/environment`](configuration.md#environment-overrides).

## Grok can't see the node

1. Confirm the daemon is live: `groknode status` → `daemonAlive: true`.
2. Confirm sign-in: `groknode check` → `"authentication": "verified"`.
3. Make sure you're looking at the same xAI account you signed in with.
4. Follow the logs while it connects: `journalctl -u groknode -f`.

## Grok can't write to a project folder

Tasks are sandboxed to groknode's own state directory by default. Grant access
to the folder by adding it to `ReadWritePaths=` — see
[Granting access to your projects](configuration.md#granting-access-to-your-projects).

## Starting over

Remove credentials and local state, then sign in again:

```bash
sudo systemctl stop groknode
groknode logout
groknode login
sudo systemctl start groknode
```
