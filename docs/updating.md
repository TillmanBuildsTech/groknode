# Keeping it current

There are two independent things that update: **Grok Bot** (the vendor package)
and **groknode** itself.

## After a Grok Bot update

When Grok Bot updates on the box, the embedded runtime version may change.
Refresh the matching runtime and restart:

```bash
groknode fetch-runtime && sudo systemctl restart groknode
```

`fetch-runtime` is idempotent — if a compatible runtime is already present it
does nothing.

## Updating groknode

Re-run the one-line installer to pull the latest release; it replaces the
binary, installer, and systemd unit in place. Your credentials and node
identity are preserved:

```bash
curl -fsSL https://raw.githubusercontent.com/TillmanBuildsTech/groknode/main/scripts/get.sh | sudo bash
sudo systemctl restart groknode
```

To move to a specific version, add `--version vX.Y.Z`.

## If something breaks after an update

groknode rides an undocumented vendor protocol, so a Grok Bot update can
occasionally break compatibility. A [nightly automated
check](https://github.com/TillmanBuildsTech/groknode/actions) downloads the
current Grok Bot release and verifies everything groknode depends on, opening a
tracking issue when it detects a break — so fixes usually land before you hit
the problem.

If a run fails, start with `groknode check --local` and the
[Troubleshooting](troubleshooting.md) guide.
