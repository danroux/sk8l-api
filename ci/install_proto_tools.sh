#!/usr/bin/env bash
set -euo pipefail

# Install pinned protoc and Go protobuf codegen plugins.
# Override versions or install paths via environment variables or Makefile args.
PROTOC_VERSION="${PROTOC_VERSION:-36.2}"
PROTOC_GEN_GO_VERSION="${PROTOC_GEN_GO_VERSION:-v1.36.12}"
PROTOC_GEN_GO_GRPC_VERSION="${PROTOC_GEN_GO_GRPC_VERSION:-v1.6.2}"
PROTOC_GEN_CONNECT_GO_VERSION="${PROTOC_GEN_CONNECT_GO_VERSION:-v1.21.1}"

INSTALL_BASE="${PROTOC_INSTALL_DIR:-$(go env GOPATH)}"
BIN_DIR="${GOBIN:-$INSTALL_BASE/bin}"

OS="$(uname -s)"
ARCH="$(uname -m)"
case "${OS}/${ARCH}" in
	Linux/x86_64) PROTOC_PLATFORM="linux-x86_64" ;;
	Linux/aarch64 | Linux/arm64) PROTOC_PLATFORM="linux-aarch_64" ;;
	Darwin/x86_64) PROTOC_PLATFORM="osx-x86_64" ;;
	Darwin/arm64) PROTOC_PLATFORM="osx-aarch_64" ;;
	*)
		echo "Unsupported platform for protoc download: ${OS}/${ARCH}" >&2
		exit 1
		;;
esac

TMP_DIR="$(mktemp -d)"
cleanup() {
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT

ARCHIVE="$TMP_DIR/protoc-${PROTOC_VERSION}-${PROTOC_PLATFORM}.zip"
URL="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-${PROTOC_PLATFORM}.zip"

echo "Downloading protoc v${PROTOC_VERSION} (${PROTOC_PLATFORM})..."
curl -fsSL "$URL" -o "$ARCHIVE"

mkdir -p "$INSTALL_BASE" "$BIN_DIR"

if command -v unzip >/dev/null 2>&1; then
	unzip -oq "$ARCHIVE" 'bin/*' 'include/*' -d "$INSTALL_BASE"
else
	python3 - "$ARCHIVE" "$INSTALL_BASE" <<'PY'
import sys
import zipfile

archive, dest = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(archive) as zf:
    for info in zf.infolist():
        if info.filename.startswith(("bin/", "include/")):
            zf.extract(info, dest)
PY
fi
chmod +x "$INSTALL_BASE/bin/protoc"

echo "Installing Go protobuf plugins into ${BIN_DIR}..."
export GOBIN="$BIN_DIR"
go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
go install "connectrpc.com/connect/cmd/protoc-gen-connect-go@${PROTOC_GEN_CONNECT_GO_VERSION}"

echo "Installed: $("$INSTALL_BASE/bin/protoc" --version) at $INSTALL_BASE/bin/protoc"
