#!/usr/bin/env bash
# fuzz.sh runs every fuzz target in the module for FUZZTIME each (10s
# unless set), one after another, since go test fuzzes one target at a
# time. A failing input is saved under the package's testdata/fuzz.
set -euo pipefail
cd "$(dirname "$0")/../.."
fuzztime=${FUZZTIME:-10s}
grep -rno --include='*_test.go' '^func Fuzz[A-Za-z0-9_]*' . | sort | while IFS=: read -r file _ decl; do
	pkg=$(dirname "$file")
	name=${decl#func }
	echo "== $pkg $name"
	go test "$pkg" -run='^$' -fuzz="^$name\$" -fuzztime="$fuzztime"
done
