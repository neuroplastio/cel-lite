---
group: Guides
order: 20
nav: Limits, TinyGo and WebAssembly
description: the limits on an expression, the stack each phase needs, and building for TinyGo and WebAssembly
---

# Limits, TinyGo and WebAssembly

cel-lite is written for small targets as much as for servers: it builds
and runs under TinyGo and under Go's WebAssembly ports, with the standard
library alone. Its limits come from that.

## Limits

| Limit                                            | Value          |
| ------------------------------------------------ | -------------- |
| An expression's source (`cellite.MaxSize`)       | 100,000 bytes  |
| Its tree's depth (`cellite.MaxDepth`)            | 50             |
| How deeply brackets, calls and `? :` nest        | 16             |

An expression past a limit is refused when it compiles, with a
`SyntaxError` that says which. Chains of `&&` and `||` are built balanced,
as cel-go builds them, so a chain of hundreds of conditions stays shallow;
a chain of `+` or of field selections is as deep as it is long.

cel-go's limits are higher: 100,000 code points, and a parse depth of 250.
Real expressions come nowhere near either set: the deepest context in a
keymap, or binding in a template, is a few levels deep.

## The stack

Each phase recurses once per level of the tree, and under TinyGo a
goroutine's stack is fixed: 64 KB by default on the `wasm` and `wasip1`
targets. TinyGo checks a goroutine's stack only when it pauses, so an
overflow doesn't fail cleanly, it corrupts memory. The limits above are
set so that each phase fits a quarter of that stack, and the gate measures
it:

| Phase | Most stack at the limits | Its worst case                      |
| ----- | ------------------------ | ----------------------------------- |
| Parse | 13,248 bytes             | operators of rising precedence      |
| Check | 11,248 bytes             | a chain of 49 selections or indexes |
| Plan  | 8,224 bytes              | a sum of 49 terms                   |
| Eval  | 11,184 bytes             | a chain of 49 equalities            |

These are TinyGo 0.42's numbers on `wasm`, measured by `make stack`
against a budget of 16 KB (16,384 bytes), which `TestStackUse` enforces.
The gate also runs every test on a 32 KB stack. A program that compiles
or evaluates expressions in a goroutine of its own keeps the rest of its
64 KB for itself.

Go's own compiler grows stacks as they need it, so none of this matters
there. TinyGo's `wasm-unknown` target gives a goroutine only 4 KB by
default: pass `-stack-size=32KB` or more.

## Nothing panics

TinyGo's WebAssembly can't recover from a panic, so cel-lite doesn't
panic, on any source or any tree a program builds by hand: what it can't
take is an error. Three fuzz targets hold it to that: one hands Check and
Program trees built from random bytes, and two compare cel-lite with
cel-go on generated expressions and on random source text. A helper you
declare must not panic either.

cel-lite starts no goroutines, reads no clock and no files, and uses no
reflection. It imports `errors`, `math`, `sort`, `strconv`, `strings`
and `unicode/utf8`.

## Building

```sh
# TinyGo, for a browser or Node (with TinyGo's wasm_exec.js)
tinygo build -target wasm -o app.wasm .

# TinyGo, for a WASI runtime
tinygo build -target wasip1 -o app.wasm .

# Go, for a browser or Node (with Go's wasm_exec.js)
GOOS=js GOARCH=wasm go build -o app.wasm .

# Go, for a WASI runtime
GOOS=wasip1 GOARCH=wasm go build -o app.wasm .
```

The gate builds and tests cel-lite under TinyGo 0.42's `wasm` target and
Go's `js/wasm` on every change. The `wasip1` builds have been checked by
hand: built by both compilers and run under Node's WASI.

A small program that compiles and tests one expression is 193 KB under
TinyGo (`-target wasm -no-debug`), and 3.1 MB under Go's js/wasm (most of
it Go's runtime: a program that prints hello is 2.6 MB there). The same
program with cel-go is 20.7 MB under Go's js/wasm; under TinyGo it builds,
at 3.3 MB, but stops as it starts, on reflection TinyGo doesn't implement.

Under TinyGo, evaluating takes 24–168 ns on the benchmarks' expressions,
against 14–116 ns under Go: see [Performance](performance.md).
