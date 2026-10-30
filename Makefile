# cel-lite. Go and TinyGo are pinned in mise.toml: `mise exec -- make check`.
GO ?= go
export GO

.PHONY: check stack fuzz bench

# The gate: gofmt, vet, test, the standard library alone, TinyGo and js/wasm,
# the stack budget and the cel-go oracle.
check:
	./scripts/check.sh

# The stack each phase needs within the limits, under TinyGo.
stack:
	tinygo test -p 2 -tags cellitestack -target wasm -stack-size=1MB -run TestStackUse -v .

# Compare with cel-go on fuzzed expressions and sources, one worker each.
fuzz:
	$(GO) -C oracle test -run '^$$' -fuzz '^FuzzGenerated$$' -fuzztime 60s -fuzzminimizetime 5s -parallel 1 .
	$(GO) -C oracle test -run '^$$' -fuzz '^FuzzSource$$' -fuzztime 60s -fuzzminimizetime 5s -parallel 1 .

# Time, bytes and allocations per operation.
bench:
	$(GO) test -run '^$$' -bench . -benchmem .
