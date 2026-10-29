# FAQ

### Is this official / made by xAI?

No. groknode is an independent, unofficial project. It is **not affiliated
with, endorsed by, or supported by xAI or Cursor.** It supervises xAI's own
daemon but ships none of its code.

### Does it send my data anywhere new?

No. groknode talks to the same xAI/Cursor cloud endpoints the official Grok Bot
app already uses, with the same account. It adds no telemetry and no third
parties. The data that flows to xAI is the same data that flows when you use
the official app.

### Do I need a monitor, X server, or desktop on the box?

No. It runs fully headless — no X11, no Xvfb, no GUI. It only needs the shared
libraries the runtime links at load, which the installer handles (see
[Troubleshooting](troubleshooting.md#missing-shared-libraries)).

### Does groknode include Grok Bot itself?

No. The proprietary daemon comes only from the official Grok Bot package on your
machine. Use `--with-grokbot` at install time to have the installer download and
install that package for you, or install it yourself first.

### Can I run it on a Raspberry Pi / arm64 box?

Yes — arm64 (aarch64) builds are published alongside x64. Any always-on Linux
box works: a VM, an LXC, a home server, or a Pi.

### Will a Grok Bot update break it?

It can, because groknode rides an undocumented internal protocol. A nightly
automated check catches breaks early and opens a tracking issue. If you hit one,
update groknode (see [Keeping it current](updating.md)) — a matching fix is
usually already out.

### How do I control what Grok is allowed to do?

Two levers: the [execution policy](configuration.md#execution-policy)
(`ask` / `always` / `never`) and the filesystem
[sandbox](configuration.md#granting-access-to-your-projects) (`ReadWritePaths`).

### How do I completely remove it?

```bash
sudo systemctl disable --now groknode
groknode logout
sudo rm -f /usr/local/bin/groknode /etc/systemd/system/groknode.service
sudo rm -rf /usr/local/libexec/groknode /etc/groknode /var/lib/groknode
sudo systemctl daemon-reload
```

### Where are the developer/architecture docs?

In [`CONTRIBUTING.md`](https://github.com/TillmanBuildsTech/groknode/blob/main/CONTRIBUTING.md)
in the repository — build instructions, architecture, testing, and the
diagnostics tooling.
