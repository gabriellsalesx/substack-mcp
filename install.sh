#!/bin/sh
# Installs the latest substack-mcp release for Linux or macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.sh | sh
#
# Set INSTALL_DIR to choose where the binary goes (default: ~/.local/bin)
# and VERSION to pin a release (e.g. v0.1.0).
set -eu

repo="gabriellsalesx/substack-mcp"
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "substack-mcp: unsupported OS $(uname -s); on Windows use install.ps1" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "substack-mcp: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${VERSION:-}" ]; then
  base="https://github.com/$repo/releases/download/$VERSION"
else
  base="https://github.com/$repo/releases/latest/download"
fi
archive="substack-mcp_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $archive..."
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

expected="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)"
else
  actual="$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)"
fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "substack-mcp: checksum mismatch for $archive" >&2
  exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp" substack-mcp
mkdir -p "$install_dir"
install -m 0755 "$tmp/substack-mcp" "$install_dir/substack-mcp"

echo "Installed $("$install_dir/substack-mcp" version) to $install_dir/substack-mcp"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to your PATH, e.g.: echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.profile" ;;
esac
echo "Next: substack-mcp login"
