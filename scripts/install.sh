#!/usr/bin/env bash
# Install grokbot-node as a hardened system service with a dedicated user.
# Optionally downloads and installs the Grok Bot package + shared libraries.
# Usage: sudo ./scripts/install.sh [--with-grokbot] [path-to-grokbotd-binary]
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "This installer must run as root (sudo)." >&2
  exit 1
fi

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
bin_src="${1:-}"
with_grokbot=0
svc_user="grokbot"
state_dir="/var/lib/grokbot-node"
conf_dir="/etc/grokbot-node"
unit_dst="/etc/systemd/system/grokbot-node.service"
libexec_dir="/usr/local/libexec/grokbot-node"
bin_dst="$libexec_dir/groknode"     # daemon binary, OFF $PATH (systemd calls it directly)
cli_dst="/usr/local/bin/groknode"   # the one command users type

# Parse flags
if [[ "$bin_src" == "--with-grokbot" ]]; then
  with_grokbot=1
  bin_src="${2:-}"
fi

# --- Distro & Arch Detection -----------------------------------------------

detect_distro() {
  if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    echo "$ID"
  else
    echo "unknown"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64) echo "x64" ;;
    aarch64) echo "arm64" ;;
    *) echo "unknown" ;;
  esac
}

distro="$(detect_distro)"
arch="$(detect_arch)"

echo "Detected: distro=$distro arch=$arch"

# --- Grok Bot Installation -------------------------------------------------

fetch_grokbot_urls() {
  local linux_arch="$1"
  # Known recent stable version (0.39.0) with working URLs from cursor CDN.
  # For latest: check https://cursor.com/download/bot or https://github.com/falser101/grok-bot-linux
  local commit="d8bc9c753edddb313047c9c69b480b7f8f321087"
  local version="0.39.0"
  
  if [[ "$linux_arch" == "arm64" ]]; then
    local deb_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/arm64/grok-bot_${version}_arm64.deb"
    local rpm_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/arm64/grok-bot-${version}-1.aarch64.rpm"
    local ai_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/arm64/Grok_Bot_${version}.AppImage"
  else
    local deb_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/x64/grok-bot_${version}_amd64.deb"
    local rpm_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/x64/grok-bot-${version}-1.x86_64.rpm"
    local ai_url="https://downloads.cursor.com/grokbot/stable/${commit}/linux/x64/Grok_Bot_${version}.AppImage"
  fi
  
  echo "$deb_url|$rpm_url|$ai_url"
}

install_grokbot() {
  case "$distro" in
    debian|ubuntu)
      echo "Installing Grok Bot (.deb)..."
      install_grokbot_deb "$arch"
      ;;
    fedora|rhel|centos)
      echo "Installing Grok Bot (.rpm)..."
      install_grokbot_rpm "$arch"
      ;;
    arch|cachyos|manjaro)
      echo "Installing Grok Bot (AppImage)..."
      install_grokbot_appimage "$arch"
      ;;
    *)
      echo "Unsupported distro: $distro. Install Grok Bot manually." >&2
      return 1
      ;;
  esac
}

install_grokbot_deb() {
  local arch="$1"
  
  # Check if already installed
  if dpkg -l 2>/dev/null | grep -q "grok-bot"; then
    echo "Grok Bot already installed (deb)."
    return 0
  fi
  
  local urls deb_url
  urls=$(fetch_grokbot_urls "$arch") || return 1
  deb_url="${urls%%|*}"
  
  echo "Downloading Grok Bot .deb from: $deb_url"
  local tmp_deb="/tmp/grok-bot-${arch}.deb"
  
  if ! curl -fsSL -o "$tmp_deb" "$deb_url"; then
    echo "Failed to download from $deb_url" >&2
    return 1
  fi
  
  dpkg -i "$tmp_deb" || {
    echo "dpkg install failed, running apt-get install -f to fix dependencies..." >&2
    apt-get install -f -y
  }
  rm -f "$tmp_deb"
}

install_grokbot_rpm() {
  local arch="$1"
  
  # Check if already installed
  if rpm -q grok-bot >/dev/null 2>&1; then
    echo "Grok Bot already installed (rpm)."
    return 0
  fi
  
  local urls rpm_url
  urls=$(fetch_grokbot_urls "$arch") || return 1
  rpm_url=$(echo "$urls" | cut -d'|' -f2)
  
  echo "Downloading Grok Bot .rpm from: $rpm_url"
  local tmp_rpm="/tmp/grok-bot-${arch}.rpm"
  
  if ! curl -fsSL -o "$tmp_rpm" "$rpm_url"; then
    echo "Failed to download from $rpm_url" >&2
    return 1
  fi
  
  dnf install -y "$tmp_rpm" || rpm -ivh "$tmp_rpm" || return 1
  rm -f "$tmp_rpm"
}

install_grokbot_appimage() {
  local arch="$1"
  
  # Check if already installed
  if [[ -f /opt/Grok\ Bot/grok-bot ]]; then
    echo "Grok Bot already installed (AppImage)."
    return 0
  fi
  
  local urls ai_url
  urls=$(fetch_grokbot_urls "$arch") || return 1
  ai_url=$(echo "$urls" | cut -d'|' -f3)
  
  echo "Downloading Grok Bot AppImage from: $ai_url"
  local tmp_ai="/tmp/Grok_Bot-${arch}.AppImage"
  
  if ! curl -fsSL -o "$tmp_ai" "$ai_url"; then
    echo "Failed to download from $ai_url" >&2
    return 1
  fi
  
  chmod +x "$tmp_ai"
  mkdir -p "/opt/Grok Bot"
  "$tmp_ai" --appimage-extract >/dev/null 2>&1 || {
    echo "Warning: AppImage extraction may require manual setup" >&2
  }
  if [[ -d squashfs-root ]]; then
    mv squashfs-root/* "/opt/Grok Bot/" || true
    rm -rf squashfs-root
  fi
  rm -f "$tmp_ai"
}

install_grokbot_libs() {
  case "$distro" in
    debian|ubuntu)
      echo "Installing Electron runtime libraries (apt)..."
      apt-get update
      apt-get install -y \
        libgtk-3-0 libnss3 libatk1.0-0 libatk-bridge2.0-0 libgbm1 \
        libasound2 libxkbcommon0 libdrm2 libcups2 libatspi2.0-0 \
        libpango-1.0-0 libcairo2 libx11-6 libxcomposite1 libxdamage1 \
        libxrandr2 libxtst6 libxfixes3 libexpat1 libglib2.0-0 libdbus-1-3
      ;;
    fedora|rhel|centos)
      echo "Installing Electron runtime libraries (dnf/yum)..."
      dnf install -y \
        gtk3 nss atk at-spi2-atk at-spi2-core mesa alsa-lib libxkbcommon \
        libdrm libcups pango cairo libx11 libxcomposite libxdamage libxrandr \
        libxtst libxfixes expat glib2 dbus-libs || \
      yum install -y \
        gtk3 nss atk at-spi2-atk at-spi2-core mesa alsa-lib libxkbcommon \
        libdrm libcups pango cairo libx11 libxcomposite libxdamage libxrandr \
        libxtst libxfixes expat glib2 dbus-libs
      ;;
    arch|cachyos|manjaro)
      echo "Installing Electron runtime libraries (pacman)..."
      pacman -Sy --needed \
        gtk3 nss atk at-spi2-atk at-spi2-core mesa alsa-lib libxkbcommon \
        libdrm libcups pango cairo libx11 libxcomposite libxdamage libxrandr \
        libxtst libxfixes expat glib2 dbus
      ;;
    *)
      echo "Cannot install libs for distro: $distro" >&2
      return 1
      ;;
  esac
}

# --- grokbotd Binary -------------------------------------------------------

# 1. Obtain the binary.
if [[ -z "$bin_src" ]]; then
  if command -v go >/dev/null 2>&1; then
    echo "Building groknode..."
    ( cd "$repo_dir" && CGO_ENABLED=0 go build -o /tmp/groknode ./cmd/groknode )
    bin_src=/tmp/groknode
  else
    echo "No binary given and 'go' not found. Pass a prebuilt groknode path." >&2
    exit 1
  fi
fi

# 2. Grok Bot package (if requested)
if ((with_grokbot)); then
  install_grokbot || {
    echo "Warning: Grok Bot installation failed. Continue without it?" >&2
    read -p "Continue? [y/N] " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && exit 1
  }
  install_grokbot_libs
fi

# 3. Dedicated system user (no login shell, home = state dir).
if ! id "$svc_user" >/dev/null 2>&1; then
  useradd --system --home-dir "$state_dir" --shell /usr/sbin/nologin "$svc_user"
fi

# 4. Directories. Credentials/state live under state_dir, owned by grokbot.
install -d -m 0750 -o root -g "$svc_user" "$conf_dir"
install -d -m 0700 -o "$svc_user" -g "$svc_user" "$state_dir"

# 5. Binary + env file + unit.
# Remove binaries from any pre-rename install (old grokbotd/grokbotctl on $PATH).
rm -f /usr/local/bin/grokbotd /usr/local/bin/grokbotctl
install -d -m 0755 "$libexec_dir"
install -m 0755 "$bin_src" "$bin_dst"
cat > "$conf_dir/environment" <<EOF
GROKBOT_CONFIG_DIR=$state_dir/.config/grokbot-node
GROKBOT_DATA_ROOT=$state_dir/.local/share/grokbot-node
# Override the vendor paths here if Grok Bot is installed elsewhere:
# GROK_BOT_BINARY=/opt/Grok Bot/grok-bot
# Pin a specific stock Electron instead of auto-detect:
# GROKBOT_ELECTRON=/usr/lib/electron42/electron
EOF
chmod 0640 "$conf_dir/environment"
chown root:"$svc_user" "$conf_dir/environment"
install -m 0644 "$repo_dir/packaging/systemd/grokbot-node.service" "$unit_dst"

# 6. The single user-facing command. Runs the daemon binary as the service
# user with the service environment, so state lands in the grokbot-owned dirs.
cat > "$cli_dst" <<'WRAPPER_EOF'
#!/usr/bin/env bash
# groknode: manage the headless Grok Bot node (runs as the service user).
set -a; . /etc/grokbot-node/environment; set +a
if [[ $EUID -eq 0 ]]; then
  # Already root: run as the service user with environment
  export HOME=/var/lib/grokbot-node
  exec su -s /bin/bash - grokbot -c "/usr/local/libexec/grokbot-node/groknode $(printf '%q ' "$@")"
else
  # Not root: escalate with sudo
  exec sudo -E -u grokbot /usr/local/libexec/grokbot-node/groknode "$@"
fi
WRAPPER_EOF
chmod 0755 "$cli_dst"

# 7. Shared-library preflight. Electron links GTK/X11/NSS at load even in
# run_as_node mode; a slim server/LXC may lack them. No display server needed.
preflight_libs() {
  command -v ldconfig >/dev/null 2>&1 || return 0
  local needed=(libgtk-3.so.0 libnss3.so libatk-1.0.so.0 libatk-bridge-2.0.so.0 \
    libgbm.so.1 libasound.so.2 libxkbcommon.so.0 libdrm.so.2 libcups.so.2 \
    libatspi.so.0 libpango-1.0.so.0 libcairo.so.2)
  local missing=()
  local cache; cache="$(ldconfig -p 2>/dev/null)"
  for lib in "${needed[@]}"; do
    grep -q "$lib" <<<"$cache" || missing+=("$lib")
  done
  if ((${#missing[@]})); then
    echo
    echo "WARNING: Electron runtime libraries not found on this host:"
    printf '  %s\n' "${missing[@]}"
    echo "Install them (no display server/Xvfb required, just the libraries):"
    echo "  Debian/Ubuntu: sudo apt-get install -y libgtk-3-0 libnss3 libatk1.0-0 \\"
    echo "                   libatk-bridge2.0-0 libgbm1 libasound2 libxkbcommon0 libdrm2 \\"
    echo "                   libcups2 libatspi2.0-0 libpango-1.0-0 libcairo2 libx11-6 \\"
    echo "                   libxcomposite1 libxdamage1 libxrandr2 libxtst6 libxfixes3"
    echo "  Arch:          sudo pacman -S --needed gtk3 nss atk at-spi2-atk at-spi2-core \\"
    echo "                   mesa alsa-lib libxkbcommon libdrm libcups pango cairo libx11 \\"
    echo "                   libxcomposite libxdamage libxrandr libxtst libxfixes"
    echo "('groknode check --local' re-checks this after fetch-runtime.)"
  fi
}
preflight_libs

systemctl daemon-reload
systemctl enable grokbot-node.service

cat <<EOF

Installed grokbot-node.

Next steps (run on this box):
  1. Verify Grok Bot is installed and available.
  2. groknode fetch-runtime     # download the matching stock Electron
  3. groknode check --local     # verify compatibility
  4. groknode login             # browser-link sign-in (prints a URL)
  5. groknode policy ask        # or: always | never
  6. systemctl start grokbot-node

After any Grok Bot update, re-run:  groknode fetch-runtime && systemctl restart grokbot-node
EOF
