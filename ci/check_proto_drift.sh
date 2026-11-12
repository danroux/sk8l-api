#!/usr/bin/env bash
set -euo pipefail

# Verify committed protobuf, gRPC, and Connect stubs under protos/ match
# `make proto` output using the pinned toolchain versions defined in Makefile.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

TOOLS_DIR="$(mktemp -d)"
cleanup() {
	rm -rf "$TOOLS_DIR"
}
trap cleanup EXIT

read_pb_header_version() {
	local file="$1"
	local tool="$2"
	if [[ -f "$file" ]]; then
		sed -nE "s|^//.*[[:space:]]${tool}[[:space:]]+(v[^[:space:]]+).*|\1|p" "$file" | head -n 1
	fi
}

normalize_protoc_version() {
	local ver="$1"
	echo "$ver" | sed -E 's/^v[0-9]+\.([0-9]+\.[0-9]+)$/\1/; s/^v//'
}

PREV_PROTOC_HDR="$(read_pb_header_version "protos/sk8l.pb.go" "protoc")"
PREV_PROTOC_VERSION="$(normalize_protoc_version "$PREV_PROTOC_HDR")"
PREV_GEN_GO_VERSION="$(read_pb_header_version "protos/sk8l.pb.go" "protoc-gen-go")"
PREV_GEN_GO_GRPC_VERSION="$(read_pb_header_version "protos/sk8l_grpc.pb.go" "protoc-gen-go-grpc")"

make proto-tools PROTOC_INSTALL_DIR="$TOOLS_DIR" GOBIN="$TOOLS_DIR/bin"
export PATH="$TOOLS_DIR/bin:$PATH"
hash -r

for tool in protoc protoc-gen-go protoc-gen-go-grpc protoc-gen-connect-go; do
	resolved="$(command -v "$tool" || true)"
	if [[ "$resolved" != "$TOOLS_DIR/bin/$tool" ]]; then
		echo "Expected pinned $tool in $TOOLS_DIR/bin, got: ${resolved:-<missing>}" >&2
		exit 1
	fi
done

CI_PROTOC_VERSION="$(protoc --version | awk '{print $2}')"
CI_GEN_GO_VERSION="$(go list -m -f '{{.Version}}' google.golang.org/protobuf)"
CI_GEN_GO_GRPC_VERSION="$(sed -nE 's/^PROTOC_GEN_GO_GRPC_VERSION[[:space:]]*\?=[[:space:]]*([^[:space:]]+).*/\1/p' Makefile | head -n 1)"
CI_GEN_CONNECT_VERSION="$(go list -m -f '{{.Version}}' connectrpc.com/connect)"

print_drift_versions() {
	echo "" >&2
	echo "Tool versions used by this check:" >&2
	echo "  PROTOC_VERSION=${CI_PROTOC_VERSION}" >&2
	echo "  PROTOC_GEN_GO_VERSION=${CI_GEN_GO_VERSION} (from go.mod)" >&2
	echo "  PROTOC_GEN_GO_GRPC_VERSION=${CI_GEN_GO_GRPC_VERSION}" >&2
	echo "  PROTOC_GEN_CONNECT_GO_VERSION=${CI_GEN_CONNECT_VERSION} (from go.mod)" >&2
	echo "" >&2
	echo "Versions recorded in committed protos/ headers:" >&2
	echo "  protoc:           ${PREV_PROTOC_HDR:-<unknown>} (PROTOC_VERSION=${PREV_PROTOC_VERSION:-<unknown>})" >&2
	echo "  protoc-gen-go:    ${PREV_GEN_GO_VERSION:-<unknown>}" >&2
	echo "  protoc-gen-go-grpc: ${PREV_GEN_GO_GRPC_VERSION:-<unknown>}" >&2

	if [[ -n "$PREV_PROTOC_VERSION" && "$PREV_PROTOC_VERSION" != "$CI_PROTOC_VERSION" ]] || \
	   [[ -n "$PREV_GEN_GO_GRPC_VERSION" && "$PREV_GEN_GO_GRPC_VERSION" != "$CI_GEN_GO_GRPC_VERSION" ]]; then
		echo "" >&2
		echo "If you generated protos/ locally with newer tools, update Makefile and ci/install_proto_tools.sh to:" >&2
		if [[ -n "$PREV_PROTOC_VERSION" && "$PREV_PROTOC_VERSION" != "$CI_PROTOC_VERSION" ]]; then
			echo "  PROTOC_VERSION=${PREV_PROTOC_VERSION}" >&2
		fi
		if [[ -n "$PREV_GEN_GO_GRPC_VERSION" && "$PREV_GEN_GO_GRPC_VERSION" != "$CI_GEN_GO_GRPC_VERSION" ]]; then
			echo "  PROTOC_GEN_GO_GRPC_VERSION=${PREV_GEN_GO_GRPC_VERSION}" >&2
		fi
	fi
}

make proto

if ! git diff --exit-code -- protos/; then
	echo "" >&2
	echo "Proto drift detected: regenerated stubs under protos/ differ from committed files." >&2
	print_drift_versions
	echo "" >&2
	echo "Otherwise, run 'make proto-tools && make proto' and commit the updated files in protos/." >&2
	exit 1
fi

untracked="$(git ls-files --others --exclude-standard -- protos/)"
if [[ -n "$untracked" ]]; then
	echo "Proto drift detected: untracked files generated under protos/:" >&2
	echo "$untracked" >&2
	print_drift_versions
	exit 1
fi

echo "No proto drift detected."
