#!/usr/bin/env bash
# cel-lite's gate (make check): gofmt, vet and test; the standard library
# alone; every test under TinyGo's wasm on half a goroutine's stack, and the
# stack budget measured; Go's js/wasm; and the cel-go oracle.
#
# Each TinyGo and wasm run goes under a memory cap and a timeout, where
# systemd-run is there: an overflowed TinyGo stack corrupts memory silently,
# and a runaway must die alone, not take the machine with it.
set -euo pipefail
cd "$(dirname "$0")/.."
GO=${GO:-go}

# A new scope starts from the whole machine, so it takes the caller's CPUs
# and, if the caller runs idle, the idle weight: a gate confined to some
# cores keeps its TinyGo builds there.
capped() {
	if command -v systemd-run >/dev/null && systemd-run --user --scope -q -p MemoryMax=64M -- true 2>/dev/null; then
		local props=(-p MemoryMax=2G -p MemorySwapMax=0)
		local cpus cg
		cpus=$(awk '/^Cpus_allowed_list:/ {print $2}' /proc/self/status)
		[ -n "$cpus" ] && props+=(-p "AllowedCPUs=$cpus")
		cg=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup)
		[ "$(cat "$cg/cpu.idle" 2>/dev/null)" = 1 ] && props+=(-p CPUWeight=idle)
		systemd-run --user --scope -q "${props[@]}" -- timeout 600 "$@"
	else
		timeout 600 "$@"
	fi
}

echo "== gofmt"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "gofmt: not formatted:"
	echo "$unformatted"
	exit 1
fi

echo "== vet"
"$GO" vet .
"$GO" vet -tags cellitestack .

echo "== test"
"$GO" test .

# The module needs nothing but the standard library.
echo "== dependencies"
deps=$("$GO" list -m all)
if [ "$deps" != "github.com/neuroplastio/cel-lite" ]; then
	echo "dependencies: the module requires more than the standard library:"
	echo "$deps"
	exit 1
fi

# TinyGo runs a goroutine on a fixed 64 KB stack. Every test runs on 32 KB,
# and TestStackUse measures what each phase needs within the limits against
# its budget (stack_test.go).
echo "== tinygo"
if ! command -v tinygo >/dev/null; then
	echo "tinygo: not on PATH; run the gate through mise: mise exec -- make check"
	exit 1
fi
capped tinygo test -p 2 -target wasm -stack-size=32KB .
capped tinygo test -p 2 -tags cellitestack -target wasm -stack-size=1MB -run TestStackUse .

echo "== js/wasm"
GOOS=js GOARCH=wasm capped "$GO" test -exec "$("$GO" env GOROOT)/lib/wasm/go_js_wasm_exec" .

# cel-go lives in the oracle's own module, never in ours.
echo "== oracle"
"$GO" -C oracle vet .
"$GO" -C oracle test .

echo "check: ok"
