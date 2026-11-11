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

make proto

if ! git diff --exit-code -- protos/; then
	echo "" >&2
	echo "Proto drift detected: regenerated stubs under protos/ differ from committed files." >&2
	echo "Run 'make proto-tools && make proto' and commit the updated files in protos/." >&2
	exit 1
fi

untracked="$(git ls-files --others --exclude-standard -- protos/)"
if [[ -n "$untracked" ]]; then
	echo "Proto drift detected: untracked files generated under protos/:" >&2
	echo "$untracked" >&2
	exit 1
fi

echo "No proto drift detected."
