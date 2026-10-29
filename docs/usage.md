# Everyday use

Everything is a subcommand of `groknode`. You run it as a normal user (it
escalates to the service account itself where needed).

| Command | What it does |
|---------|--------------|
| `groknode login` | Browser sign-in; stores your credentials |
| `groknode status` | Current node and daemon status (JSON) |
| `groknode check [--local]` | Verify compatibility (and sign-in) |
| `groknode policy <always\|ask\|never>` | Set how Grok is allowed to act locally |
| `groknode fetch-runtime` | Download/refresh the matching runtime |
| `groknode logout` | Remove credentials and local state |
| `groknode version` | Print version, commit, and build date |

## Checking status

`groknode status` prints a JSON snapshot — handy for a quick health check or a
monitoring probe:

```bash
groknode status
```

```json
{
  "label": "llm-proxy",
  "machineId": "7ca20bb8-2957-45c7-a127-255de3a1e41b",
  "daemonAlive": true,
  "daemonPid": 20609,
  "localToolPermission": "ask",
  "dataRoot": "/var/lib/groknode/.local/share/groknode"
}
```

- `daemonAlive` / `daemonPid` — whether the vendor daemon is currently running.
- `localToolPermission` — your current [execution policy](configuration.md#execution-policy).
- `machineId` — this node's registered identity.

## Managing the service

Start, stop, and inspect the service with systemd as usual:

```bash
sudo systemctl status groknode
sudo systemctl stop groknode
sudo systemctl restart groknode
journalctl -u groknode -f          # follow the logs live
```

The service refuses to start until you have signed in — it gates on the
presence of your credentials file, so a freshly installed but not-yet-logged-in
box will show as `inactive (dead)` with a `ConditionPathExists` note. That is
expected; run `groknode login` first.
