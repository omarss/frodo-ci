#!/usr/bin/env bash
# Frodo CI installer.
#
#   curl -fsSL https://raw.githubusercontent.com/omarss/frodo-ci/main/install.sh | bash
#
# Downloads the latest release binary for the current OS/arch, falling back to
# `go install` from source when a prebuilt binary is unavailable.
set -euo pipefail

REPO="omarss/frodo-ci"
BINARY="frodo-ci"
VERSION="${FRODO_CI_VERSION:-latest}"

# Pick an install directory that is writable and on PATH.
install_dir() {
  for d in "/usr/local/bin" "$HOME/.local/bin" "$HOME/bin"; do
    if [ -d "$d" ] && [ -w "$d" ]; then echo "$d"; return; fi
  done
  mkdir -p "$HOME/.local/bin"
  echo "$HOME/.local/bin"
}

os() { uname -s | tr '[:upper:]' '[:lower:]'; }
arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) echo "unsupported arch: $(uname -m)" >&2; return 1 ;;
  esac
}

main() {
  local dir os arch url tmp
  dir="$(install_dir)"
  os="$(os)"; arch="$(arch)"

  if [ "$VERSION" = "latest" ]; then
    url="https://github.com/${REPO}/releases/latest/download/${BINARY}_${os}_${arch}"
  else
    url="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${os}_${arch}"
  fi

  tmp="$(mktemp)"
  echo ">> Downloading ${BINARY} (${os}/${arch}) from ${url}"
  if curl -fsSL "$url" -o "$tmp" 2>/dev/null; then
    chmod +x "$tmp"
    mv "$tmp" "${dir}/${BINARY}"
    echo ">> Installed ${BINARY} to ${dir}/${BINARY}"
  else
    rm -f "$tmp"
    echo ">> No prebuilt binary available; falling back to 'go install'."
    if ! command -v go >/dev/null 2>&1; then
      echo "error: Go is not installed and no prebuilt binary was found." >&2
      exit 1
    fi
    GOBIN="$dir" go install "github.com/${REPO}/cmd/${BINARY}@${VERSION}"
    echo ">> Installed ${BINARY} to ${dir}/${BINARY} via go install"
  fi

  case ":$PATH:" in
    *":${dir}:"*) ;;
    *) echo ">> Note: add ${dir} to your PATH." ;;
  esac
  "${dir}/${BINARY}" --version || true
}

main "$@"
