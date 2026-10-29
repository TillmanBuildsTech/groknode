# Configuration

## Execution policy

`groknode policy` controls whether Grok's local-computer actions require your
confirmation before they run:

```bash
groknode policy ask        # Grok asks before each action (default, safest)
groknode policy always     # actions run without prompting
groknode policy never      # local actions are blocked entirely
```

The setting is synced to the backend and applied locally on the next service
restart.

## Granting access to your projects

By default Grok's tasks run sandboxed to groknode's own state directory — it
cannot see or modify the rest of your filesystem. To let Grok work in specific
project folders, add those paths to `ReadWritePaths=` in the systemd unit:

1. Edit `/etc/systemd/system/groknode.service` and extend the line:

   ```ini
   ReadWritePaths=/var/lib/groknode /run/groknode /srv/projects/my-app
   ```

2. Reload and restart:

   ```bash
   sudo systemctl daemon-reload && sudo systemctl restart groknode
   ```

Anything not listed stays read-only or hidden from Grok's tasks.

## Environment overrides

The installer writes `/etc/groknode/environment`, read by both the service and
the `groknode` command. Uncomment and edit entries to override defaults:

```ini
GROKBOT_CONFIG_DIR=/var/lib/groknode/.config/groknode
GROKBOT_DATA_ROOT=/var/lib/groknode/.local/share/groknode
# Override the vendor paths here if Grok Bot is installed elsewhere:
# GROK_BOT_BINARY=/opt/Grok Bot/grok-bot
# Pin a specific stock Electron instead of auto-detect:
# GROKBOT_ELECTRON=/usr/lib/electron42/electron
```

| Variable | Purpose |
|----------|---------|
| `GROKBOT_CONFIG_DIR` | Where credentials and machine identity are stored |
| `GROKBOT_DATA_ROOT` | Daemon state + the managed runtime cache |
| `GROK_BOT_BINARY` | Path to the vendor `grok-bot` binary, if not under `/opt/Grok Bot` |
| `GROKBOT_ELECTRON` | Pin an explicit stock Electron runtime instead of auto-detecting |

After changing this file, restart the service:

```bash
sudo systemctl restart groknode
```
