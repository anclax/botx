#!/usr/bin/env sh
set -euo pipefail

REPO="${BOTX_REPO:-cloudcarver/botx}"
VERSION="${BOTX_VERSION:-}"
OUT_DIR="${1:-.}"

fail() {
  echo "error: $1" >&2
  exit 1
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

download_file() {
  url="$1"
  output="$2"
  if has_cmd curl; then
    curl -fsSL "$url" -o "$output"
    return 0
  fi
  if has_cmd wget; then
    wget -qO "$output" "$url"
    return 0
  fi
  fail "curl or wget is required"
}

fetch_latest_version() {
  api="https://api.github.com/repos/$REPO/releases/latest"
  if has_cmd curl; then
    curl -fsSL "$api" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1
    return 0
  fi
  if has_cmd wget; then
    wget -qO- "$api" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1
    return 0
  fi
  fail "curl or wget is required"
}

os_name="$(uname -s)"
arch_name="$(uname -m)"

case "$os_name" in
  Linux*) os="linux" ;;
  Darwin*) os="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) os="windows" ;;
  *) fail "unsupported OS: $os_name" ;;
esac

case "$arch_name" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) fail "unsupported architecture: $arch_name" ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(fetch_latest_version)"
fi

if [ -z "$VERSION" ]; then
  fail "could not determine version"
fi

if [ "$os" = "windows" ]; then
  ext="zip"
  bin="botx.exe"
else
  ext="tar.gz"
  bin="botx"
fi

asset="botx_${VERSION}_${os}_${arch}.${ext}"
url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"

mkdir -p "$OUT_DIR"
tmp_dir="$(mktemp -d)"
archive_path="$tmp_dir/$asset"
unpack_dir="$tmp_dir/unpack"
mkdir -p "$unpack_dir"

download_file "$url" "$archive_path"

if [ "$ext" = "zip" ]; then
  if ! has_cmd unzip; then
    fail "unzip is required for Windows archives"
  fi
  unzip -q "$archive_path" -d "$unpack_dir"
else
  tar -xzf "$archive_path" -C "$unpack_dir"
fi

target="$OUT_DIR/$bin"
mv "$unpack_dir/$bin" "$target"
chmod +x "$target"

echo "Downloaded botx to $target"
