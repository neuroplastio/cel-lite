# cel-lite: agent guide

cel-lite runs a subset of CEL with Go's standard library alone, so that it
builds for js/wasm and under TinyGo. Myelin Agent (myelin@neuroplast.io)
owns it. Its license is Apache-2.0. The name is a working name: keep it
easy to rename (one module path, one package name, `cellite`).

## What cel-lite is, and is not

- **CEL as the language definition says.** cel-go is the oracle because it
  is usually right, not because it defines the language. Where the two
  differ, cel-lite follows the definition or refuses by name (Unsupported),
  and the difference is listed in doc.go ("Where cel-lite and cel-go
  differ") and excluded from the oracle by name (D2).
- **A public tree.** Parse, Check and the checked `*Expr` are API: a
  program may rewrite the parsed tree and generate Go from the checked one
  (hotty-surf does both).
- **No files, no clock, no goroutines, no reflection on the hot path.**
  Values reach the evaluator through the List, Map and Object interfaces.
- **Its users keep what is theirs.** myelin (keymap contexts) and
  hotty-surf (.surf templates) declare their own variables, structs and
  helpers. Never edit a user's repository; when one needs something, it
  asks here.

## Rules

- **The root module requires nothing.** cel-go lives only in `oracle/`, a
  module of its own, and is linked into no binary. The gate checks it.
- **Nothing panics, on any input.** `recover` doesn't work under TinyGo's
  wasm. FuzzTrees hands Check and Program trees built by hand, and the
  fuzzers in `oracle/` run against cel-go.
- **The limits are measured, not chosen** (D5). Each phase (parse, check,
  plan, eval) stays within 16 KB of stack under TinyGo at the limits:
  `make stack` measures it, TestStackUse fails past the budget, and the
  gate runs every test on a 32 KB stack. Raise a limit only with a new
  measurement. A deep recursion overflows a TinyGo goroutine's fixed
  64 KB silently and corrupts the heap; on 2026-10-30 one such run took
  the machine's memory.
- **Recurse through small functions.** TinyGo gives a frame room for all
  of a function's cases: keep the recursive path (parse's expr/binary/
  unary/member/primary, check, compile, attr) small, and do each node's
  work in a function apart, marked `//go:noinline`.
- **Every heavy run is capped.** Builds, tests, fuzzing and TinyGo/wasm runs
  go under `systemd-run --user --scope -q -p MemoryMax=… -p MemorySwapMax=…`
  with a timeout, plus `-p CPUWeight=idle` outside the feel lock. Send
  output that could run away through `head -c` into a file. check.sh caps
  its own TinyGo and wasm steps.
- **Benchmarks hold the feel lock** (`flock ~/.cache/hotty/sway-feel/lock …`)
  and stay short; other sessions queue on it. No artificial load.
- **Nothing private.** No data from private repositories goes into this
  one: the oracle reads keymap contexts from sibling checkouts at test time
  and skips when they are missing.
- **Publishing is the maintainer's.** No push, no tag and no release
  without the maintainer's go-ahead.

## Where things live

- `README.md` and `docs/`: the user docs, which the website renders from
  `docs/docs.yaml` and each page's front matter (group, order, nav,
  description; the title is the page's heading). `doc.go` and the godoc
  say the same in short, and `example_test.go` runs: keep them in step.
  Code lines in the docs stay within 80 columns.
- `oracle/`: the comparison with cel-go: a corpus, generated expressions,
  two fuzz targets, and cel-go's side of the benchmarks.
- `limit_test.go`, `stack_test.go`, `stackprobe*.go`: the worst cases at
  exactly the limits and the stack they use.
- `vault/knowledge/decisions.md`: the binding decisions, Dn, append-only.
- `vault/tasks/board.md`: open work.
- `vault/journal/YYYY-MM.md`: one bullet per leg.

## Working

1. Read the board's top and the latest journal entries.
2. Take one leg. Run `mise exec -- make check` under the memory cap
   before every commit.
3. Commit on local main. Tell gov which commits would go out.
