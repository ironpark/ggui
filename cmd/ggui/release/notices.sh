#!/bin/sh
# notices.sh copies the notices of the rcodesign library the Linux and
# Windows binaries link (zapp's pkg/signing/rcodesign/lib module: its
# LICENSE, NOTICE and the Rust dependencies' licenses) to
# cmd/ggui/release/notices, from where GoReleaser puts them in those
# archives. apple-codesign is MPL-2.0, whose notice says where its source
# is. Run from the repository root, as GoReleaser's before hook does.
set -eu
out=cmd/ggui/release/notices
lib=$(cd cmd/ggui && GOWORK=off go list -m -f '{{.Dir}}' github.com/ironpark/zapp/pkg/signing/rcodesign/lib)
if [ -z "$lib" ] || [ ! -f "$lib/NOTICE" ]; then
	GOWORK=off go -C cmd/ggui mod download github.com/ironpark/zapp/pkg/signing/rcodesign/lib
	lib=$(cd cmd/ggui && GOWORK=off go list -m -f '{{.Dir}}' github.com/ironpark/zapp/pkg/signing/rcodesign/lib)
fi
rm -rf "$out"
mkdir -p "$out"
cp "$lib/LICENSE" "$lib/NOTICE" "$lib/licenses/linux.html" "$lib/licenses/windows.html" "$out"
chmod 644 "$out"/*
