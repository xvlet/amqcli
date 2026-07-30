#!/bin/sh
set -eu

BIN="amqcli"
REPO="xvlet/amqcli"
INSTALL_DIR="${AMQCLI_INSTALL_DIR:-$HOME/.local/bin}"

main() {
    echo ""
    echo "                                _ _ "
    echo "   __ _ _ __ ___   __ _  ___| (_) "
    echo "  / _\` | '_ \` _ \ / _\` |/ __| | | "
    echo " | (_| | | | | | | (_| | (__| | | "
    echo "  \__,_|_| |_| |_|\__, |\___|_|_| "
    echo "                  |___/           amqcli installer 💫"
    echo "                                  github.com/xvlet/amqcli"
    echo ""

    # detect platform
    OS="$(uname -s)"
    case "$OS" in
        Linux)  os="linux" ;;
        Darwin) os="macos" ;;
        AIX)    os="aix" ;;
        *)      err "unsupported OS: $OS" ;;
    esac

    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)   arch="amd64" ;;
        aarch64|arm64)  arch="arm64" ;;
        ppc64*)         arch="ppc64" ;;
        *)              err "unsupported architecture: $ARCH" ;;
    esac

    log "detected ${os}/${arch}"

    # check dependencies
    need curl
    need grep
    need sed
    need tar

    TARGET="${os}_${arch}"
    log "fetching latest release manifest..."
    MANIFEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
    MANIFEST="$(curl -fsSL --retry 3 --connect-timeout 10 --max-time 20 "$MANIFEST_URL")" \
        || err "can't reach GitHub API. Please try again later."
    
    # Extract version
    VERSION="$(echo "$MANIFEST" | grep '"tag_name":' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/')"
    
    # Extract asset URL for the target platform (.tar.gz is assumed for releases)
    URL="$(echo "$MANIFEST" | grep '"browser_download_url":' | grep -i "$TARGET" | grep '\.tar\.gz' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/')"

    if [ -z "$URL" ]; then
        err "release manifest does not include a binary for ${TARGET}"
    fi

    log "downloading ${VERSION}..."
    TMP="$(mktemp -d)"
    trap 'rm -rf "$TMP"' EXIT

    if ! curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 "$URL" -o "${TMP}/${BIN}.tar.gz"; then
        err "download failed from ${URL}"
    fi

    log "extracting..."
    tar -xzf "${TMP}/${BIN}.tar.gz" -C "$TMP" || err "extraction failed"

    # install
    mkdir -p "$INSTALL_DIR"
    
    # Sometimes binary is inside a folder, find it
    BIN_PATH="$(find "$TMP" -type f -name "$BIN" -print -quit)"
    if [ -z "$BIN_PATH" ]; then
        err "could not find ${BIN} binary in the extracted archive"
    fi

    mv "$BIN_PATH" "${INSTALL_DIR}/${BIN}"
    chmod +x "${INSTALL_DIR}/${BIN}"

    # Install config file if it doesn't exist
    if [ ! -f "$HOME/.amqcli.yml" ] && [ -f "$TMP/config.yml" ]; then
        cp "$TMP/config.yml" "$HOME/.amqcli.yml"
        log "installed default config to $HOME/.amqcli.yml"
    fi

    log "installed ${BIN} to ${INSTALL_DIR}/${BIN}"

    # check PATH
    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            echo ""
            warn "${INSTALL_DIR} is not in your PATH"
            echo "  add it to your shell config:"
            echo ""
            echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
            echo ""
            ;;
    esac

    # verify
    if command -v "$BIN" >/dev/null 2>&1; then
        echo ""
        log "ready. run 'amqcli' to get started."
    fi

    echo ""
}

log()  { printf '  \033[32m>\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1"; }
err()  { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }

need() {
    if ! command -v "$1" >/dev/null 2>&1; then
        err "requires '$1' — install it first"
    fi
}

main "$@"
