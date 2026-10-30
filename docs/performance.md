---
group: Reference
order: 20
nav: Performance
description: the benchmarks against cel-go, how they were measured, and how to run them
---

# Performance

cel-lite compiles an expression in one or two microseconds and evaluates
it in tens of nanoseconds, allocating nothing unless it builds a string
or calls a helper. The benchmarks below run the same expressions through
cel-lite, under Go and under TinyGo, and through cel-go.

## The expressions

The benchmarks use expressions of two shapes that run often: a keymap's
contexts, short bool checks made on every key, and a template's
bindings, made on every redraw.

| Name          | Expression                              |
| ------------- | --------------------------------------- |
| `bool`        | `search`                                |
| `and-not`     | `mode == 'normal' && !pane.zoomed`      |
| `and-chain`   | `dock && !field && !overlay`            |
| `string-eq`   | `viewer.kind == 'secret'`               |
| `in-map`      | `'k' in pane.labels`                    |
| `field`       | `row.name`                              |
| `field-or`    | `row.icon != '' ? row.icon : '-'`       |
| `bool-text`   | `row.unread ? '●' : '○'`                |
| `size-and`    | `loaded && size(rows) == 0`             |
| `concat`      | `'l' + string(n)`                       |
| `helper`      | `age(row.age)`                          |
| `helper-in`   | `control(row) in ['number', 'text']`    |
| `index-field` | `rows[n].name == row.name`              |

`row` is a struct the program declares, `rows` a list of them, and `age`
and `control` are helpers.

## Evaluating

Time for one evaluation, the median of the runs, with the allocations it
makes:

| Expression    | cel-lite, Go | cel-lite, TinyGo wasm | cel-go         |
| ------------- | ------------ | --------------------- | -------------- |
| `bool`        | 14 ns        | 24 ns                 | 45 ns          |
| `and-not`     | 60 ns        | 76 ns                 | 92 ns, 1 alloc |
| `and-chain`   | 75 ns        | 85 ns                 | 110 ns         |
| `string-eq`   | 29 ns        | 47 ns                 | 67 ns, 1 alloc |
| `in-map`      | 46 ns        | 88 ns                 | 121 ns, 3 allocs |
| `field`       | 24 ns        | 35 ns                 | 60 ns, 1 alloc |
| `field-or`    | 50 ns        | 66 ns                 | 71 ns          |
| `bool-text`   | 41 ns        | 52 ns                 | 55 ns          |
| `size-and`    | 70 ns        | 103 ns                | 141 ns, 2 allocs |
| `concat`      | 60 ns, 1 alloc | 111 ns, 1 alloc     | 140 ns, 3 allocs |
| `helper`      | 72 ns, 2 allocs | 144 ns, 2 allocs   | 94 ns, 2 allocs |
| `helper-in`   | 91 ns, 1 alloc | 168 ns, 1 alloc     | 267 ns, 6 allocs |
| `index-field` | 116 ns       | 159 ns                | 429 ns, 7 allocs |

Evaluating allocates only to build a string (`concat`) or to call a
helper, which gets its arguments in a slice. `helper` is the one
expression where cel-go is quicker: its helper builds a string too.

## Compiling

Time to parse, check and plan one expression, with its allocations:

| Expression    | cel-lite             | cel-go                |
| ------------- | -------------------- | --------------------- |
| `bool`        | 0.20 µs, 5 allocs    | 7.2 µs, 123 allocs    |
| `and-not`     | 1.49 µs, 17 allocs   | 22.6 µs, 307 allocs   |
| `and-chain`   | 1.34 µs, 18 allocs   | 19.0 µs, 292 allocs   |
| `string-eq`   | 0.87 µs, 12 allocs   | 15.7 µs, 219 allocs   |
| `in-map`      | 0.98 µs, 12 allocs   | 18.3 µs, 262 allocs   |
| `field`       | 0.43 µs, 10 allocs   | 10.1 µs, 161 allocs   |
| `field-or`    | 2.05 µs, 19 allocs   | 32.2 µs, 392 allocs   |
| `bool-text`   | 1.04 µs, 15 allocs   | 30.8 µs, 403 allocs   |
| `size-and`    | 1.60 µs, 17 allocs   | 26.6 µs, 369 allocs   |
| `concat`      | 1.30 µs, 14 allocs   | 23.8 µs, 348 allocs   |
| `helper`      | 1.05 µs, 14 allocs   | 15.9 µs, 217 allocs   |
| `helper-in`   | 2.05 µs, 22 allocs   | 32.6 µs, 380 allocs   |
| `index-field` | 1.26 µs, 17 allocs   | 30.2 µs, 386 allocs   |

## How it was measured

- An AMD Ryzen AI 9 HX 370 on Linux; Go 1.26.8, TinyGo 0.42.0, cel-go
  v0.32.0.
- cel-lite under Go: `go test -bench . -benchmem -count=10
  -benchtime=100ms`, the median of 10 runs.
- cel-lite under TinyGo: `tinygo test -target wasm -bench Eval -benchmem
  -count=5 -benchtime=100ms`, run in Node, the median of 5.
- cel-go: the same expressions on the same variables, in `oracle/`, the
  median of 5. Its programs have cel-go's default options, and its
  activation is prepared once (`cel.NewActivation`), as a program that
  evaluates often keeps it. Its struct is a Go struct registered as a
  native type.
- Both engines evaluate on a prepared activation: cel-lite's reads
  variables by their index.

## Running them

Go and TinyGo are pinned in the repository's `mise.toml`.

```sh
# cel-lite, under Go
go test -run '^$' -bench . -benchmem .

# cel-lite, under TinyGo's WebAssembly (in Node)
tinygo test -target wasm -run '^$' -bench Eval -benchmem .

# cel-go, on the same expressions
go -C oracle test -run '^$' -bench . -benchmem .
```

`make bench` runs the first. Compare runs with
[benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat), and
keep the machine otherwise idle while they run.

## Why it is quick

- **Planned once.** Compiling turns the checked tree into a tree of Go
  closures, each holding what it needs: a variable's index, a struct
  field's index, a constant operand. Evaluating calls them, with no
  lookups by name and no walk over the tree.
- **Values without allocation.** A `Value` holds a number or a string in
  place, and lists, maps and structs are read through interfaces over the
  program's own data, so nothing is copied in.
- **Few allocations to compile.** The parser and the checker make a
  tree's nodes in a few chunks, sized from the source or the tree, not
  one by one.
- **The rest of CEL isn't there.** No protobuf types, no macros, no
  runtime type registry: a smaller engine has less to do.
