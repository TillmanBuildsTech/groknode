#!/usr/bin/env bash
# Install grokbot-node as a hardened system service with a dedicated user.
# Usage: sudo ./scripts/install.sh [path-to-grokbotd-binary]
# If no binary is given and `go` is available, it builds one.
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "This installer must run as root (sudo)." >&2
  exit 1
fi

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
bin_src="${1:-}"
svc_user="grokbot"
state_dir="/var/lib/grokbot-node"
conf_dir="/etc/grokbot-node"
unit_dst="/etc/systemd/system/grokbot-node.service"
bin_dst="/usr/local/bin/grokbotd"
ctl_dst="/usr/local/bin/grokbotctl"

# 1. Obtain the binary.
if [[ -z "$bin_src" ]]; then
  if command -v go >/dev/null 2>&1; then
    echo "Building grokbotd..."
    ( cd "$repo_dir" && CGO_ENABLED=0 go build -o /tmp/grokbotd ./cmd/grokbotd )
    bin_src=/tmp/grokbotd
  else
    echo "No binary given and 'go' not found. Pass a prebuilt grokbotd path." >&2
    exit 1
  fi
fi

# 2. Dedicated system user (no login shell, home = state dir).
if ! id "$svc_user" >/dev/null 2>&1; then
  useradd --system --home-dir "$state_dir" --shell /usr/sbin/nologin "$svc_user"
fi

# 3. Directories. Credentials/state live under state_dir, owned by grokbot.
install -d -m 0750 -o root -g "$svc_user" "$conf_dir"
install -d -m 0700 -o "$svc_user" -g "$svc_user" "$state_dir"

# 4. Binary + env file + unit.
install -m 0755 "$bin_src" "$bin_dst"
cat > "$conf_dir/environment" <<EOF
GROKBOT_CONFIG_DIR=$state_dir
GROKBOT_DATA_ROOT=$state_dir
# Override the vendor paths here if Grok Bot is installed elsewhere:
# GROK_BOT_BINARY=/opt/Grok Bot/grok-bot
# Pin a specific stock Electron instead of auto-detect:
# GROKBOT_ELECTRON=/usr/lib/electron42/electron
EOF
chmod 0640 "$conf_dir/environment"
chown root:"$svc_user" "$conf_dir/environment"
install -m 0644 "$repo_dir/packaging/systemd/grokbot-node.service" "$unit_dst"

# 5. Admin wrapper so you can run CLI commands as the service user with its env.
cat > "$ctl_dst" <<EOF
#!/usr/bin/env bash
# Run grokbotd as the service user with the service environment.
set -a; . "$conf_dir/environment"; set +a
exec sudo -E -u "$svc_user" "$bin_dst" "\$@"
EOF
chmod 0755 "$ctl_dst"

systemctl daemon-reload
systemctl enable grokbot-node.service

cat <<EOF

Installed grokbot-node.

Next steps (run on this box):
  1. Ensure the official Grok Bot package is installed (provides the daemon).
  2. grokbotctl fetch-runtime     # download the matching stock Electron
  3. grokbotctl check --local     # verify compatibility
  4. grokbotctl login             # browser-link sign-in (prints a URL)
  5. grokbotctl policy ask        # or: always | never
  6. systemctl start grokbot-node

After any Grok Bot update, re-run:  grokbotctl fetch-runtime && systemctl restart grokbot-node
EOF
