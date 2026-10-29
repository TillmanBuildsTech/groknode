#!/usr/bin/env bash
# get.sh — one-line remote installer for groknode.
#
#   curl -fsSL https://raw.githubusercontent.com/TillmanBuildsTech/groknode/main/scripts/get.sh | sudo bash
#
# Detects your architecture, downloads the matching release archive from GitHub,
# verifies it against the published checksums, extracts it, and runs the bundled
# installer (scripts/install.sh) to set up the hardened systemd service.
#
# Options (pass after `bash -s --`):
#   --with-grokbot      also download/install the official Grok Bot package
#   --version vX.Y.Z    install a specific tag instead of the latest release
#
# Example:
#   curl -fsSL .../get.sh | sudo bash -s -- --with-grokbot
set -euo pipefail

REPO="TillmanBuildsTech/groknode"
VERSION="latest"
PASSTHROUGH=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-grokbot) PASSTHROUGH+=("--with-grokbot"); shift ;;
    --version) VERSION="${2:?--version requires a tag}"; shift 2 ;;
    --version=*) VERSION="${1#*=}"; shift ;;
    *) echo "get.sh: unknown option: $1" >&2; exit 2 ;;
  esac
done

die() { echo "get.sh: $*" >&2; exit 1; }

[[ "$(uname -s)" == "Linux" ]] || die "groknode is Linux-only (detected $(uname -s))."

case "$(uname -m)" in
  x86_64)  ARCH="x64" ;;
  aarch64) ARCH="arm64" ;;
  *)       die "unsupported architecture: $(uname -m) (supported: x86_64, aarch64)" ;;
esac

for tool in curl tar sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

# Resolve the tag to install.
if [[ "$VERSION" == "latest" ]]; then
  api="https://api.github.com/repos/${REPO}/releases/latest"
  tag="$(curl -fsSL "$api" | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' | head -n1 | sed -E 's/.*"([^"]+)"$/\1/')"
  [[ -n "$tag" ]] || die "could not resolve the latest release tag from $api"
else
  tag="$VERSION"
fi

ver="${tag#v}"
archive="groknode_${ver}_linux_${ARCH}.tar.gz"
base="https://github.com/${REPO}/releases/download/${tag}"

echo "Installing groknode ${tag} (linux/${ARCH})..."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${archive}..."
curl -fsSL -o "$tmp/$archive" "${base}/${archive}" \
  || die "download failed: ${base}/${archive}"

# Verify against the published checksums (fail closed if the entry is present).
if curl -fsSL -o "$tmp/checksums.txt" "${base}/checksums.txt" 2>/dev/null \
   && grep -q "  ${archive}\$" "$tmp/checksums.txt"; then
  echo "Verifying checksum..."
  ( cd "$tmp" && grep "  ${archive}\$" checksums.txt | sha256sum -c - ) \
    || die "checksum verification failed for ${archive}"
else
  echo "WARNING: no checksum entry found for ${archive}; skipping verification." >&2
fi

echo "Extracting..."
tar -xzf "$tmp/$archive" -C "$tmp"

installer="$tmp/scripts/install.sh"
binary="$tmp/groknode"
[[ -f "$installer" ]] || die "installer not found in archive ($installer)"
[[ -f "$binary" ]] || die "binary not found in archive ($binary)"
chmod +x "$installer" "$binary"

echo "Running installer..."
exec bash "$installer" "${PASSTHROUGH[@]}" "$binary"
