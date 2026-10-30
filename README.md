# cel-lite

[CEL](https://cel.dev) expressions for Go programs, with the standard
library alone: small enough for TinyGo and WebAssembly, and quick enough
to run on every keystroke.

cel-lite runs a subset of the Common Expression Language. An expression
means what CEL's language definition says it means; what cel-lite doesn't
implement, it refuses by name when the expression is compiled, so nothing
quietly means something else. The parsed and checked tree is public, for
programs that would rather generate code from an expression than run it.

## Why

- **CEL without cel-go's weight.** [cel-go](https://github.com/google/cel-go)
  is the reference implementation, and the right choice when you need all
  of CEL. It links protobuf, ANTLR and five more modules into a program,
  though, and one that only asks `mode == 'normal' && !pane.zoomed` pays
  for all of them.
- **No dependencies.** The module imports the standard library and
  nothing else, and its tests check that it stays that way.
- **TinyGo and WebAssembly.** It builds and runs under TinyGo and Go's
  js/wasm. Each phase stays within a measured 13.2 KB of stack, so it fits
  TinyGo's fixed goroutine stacks, and nothing panics on any input.
- **Fast.** Compiling an expression takes one or two microseconds, and
  evaluating it takes tens of nanoseconds, allocating nothing unless it
  builds a string or calls a helper.

## Install

```sh
go get github.com/neuroplastio/cel-lite
```

It needs Go 1.24 or later. The package is `cellite`.

## Example

```go
env, err := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
	{Name: "mode", Type: cellite.String},
	{Name: "pane.zoomed", Type: cellite.Bool},
}})
if err != nil {
	log.Fatal(err)
}
prg, err := env.Compile(`mode == 'normal' && !pane.zoomed`)
if err != nil {
	log.Fatal(err) // a *cellite.Error says where, and why
}
act := env.Activation(map[string]any{"mode": "normal", "pane.zoomed": false})
holds, unknown := prg.Test(act) // true, false
```

An expression can also read your own structs, lists and maps without
copying them, call Go functions you declare, and work with variables
whose values aren't known yet. [Getting started](docs/getting-started.md)
walks through each.

## Compared with cel-go

|                       | cel-lite                          | cel-go v0.32                   |
| --------------------- | --------------------------------- | ------------------------------ |
| Language              | a subset of CEL, the rest refused | all of CEL, and extensions     |
| Dependencies          | none                              | 8 modules, 70 packages         |
| Binary, linux/amd64   | 1.9 MB                            | 10.2 MB                        |
| Go, js/wasm           | 3.1 MB                            | 20.7 MB                        |
| TinyGo, wasm          | 193 KB                            | 3.3 MB, panics as it starts    |
| Compile an expression | 0.2–2 µs, 5–22 allocations        | 7–33 µs, 120–400 allocations   |
| Evaluate it           | 14–116 ns, up to 2 allocations    | 45–430 ns, up to 7 allocations |

The sizes and dependencies are those of one small program, which
compiles and tests one expression, built with `-trimpath -ldflags='-s -w'`
by Go 1.26 and with `-no-debug` by TinyGo 0.42. A program that prints
hello is 1.6 MB, or 2.6 MB in js/wasm, so most of cel-lite's column is
Go's own runtime. Under TinyGo, cel-go builds but stops at start-up on
reflection TinyGo doesn't implement. The times are the benchmarks'
[medians](docs/performance.md); evaluating allocates only to build a
string or to call a helper.

## What's in the subset

- Literals, lists and maps; `!`, `-`, `&&`, `||`, `? :`, `==`, `!=`, `<`,
  `<=`, `>`, `>=`, `in`, `+`, `-`, `*`, `/` and `%`; field selection,
  indexing and `has()`.
- `size`, `contains`, `startsWith` and `endsWith`, and the conversions
  `int`, `uint`, `double`, `string`, `bytes`, `bool` and `dyn`.
- Structs you declare, and helpers: Go functions with typed overloads.
- Unknown variables, with CEL's rules for errors and unknowns through
  `&&` and `||`.

Refused by name: timestamps and durations, regular expressions, the
comprehension macros (`all`, `exists`, `exists_one`, `map`, `filter`),
type values, message construction and optional syntax.
[The language](docs/language.md) has the details, and the few places
where cel-lite follows CEL's definition rather than cel-go.

## Documentation

- [Getting started](docs/getting-started.md): install, a first
  expression, variables and a helper.
- [The language](docs/language.md): what the subset holds, what it
  refuses and how, and where it differs from cel-go.
- [Embedding](docs/embedding.md): environments, types, values, helpers,
  programs, errors, and the tree.
- [Limits, TinyGo and WebAssembly](docs/tinygo.md): the limits, the stack
  each phase needs, and building for TinyGo and js/wasm.
- [Performance](docs/performance.md): the benchmarks against cel-go, how
  they were measured, and how to run them.
- [Package reference](https://pkg.go.dev/github.com/neuroplastio/cel-lite)
  on pkg.go.dev, with runnable examples.

## Status

v0.1.0, the first release. cel-lite follows semantic versioning: until
1.0, a release that breaks the API raises the minor version (0.1 to 0.2),
and a patch release never does. Before 1.0 these may still change: the
tree (`Expr` and its `Op`s) as the subset grows, the wording of error
messages, and the API's edges as programs that embed it ask for things.
An expression that compiles keeps its meaning, unless that meaning is
found to differ from CEL's definition, which is a bug.

## Developing

Go and TinyGo are pinned in `mise.toml`.

- `mise exec -- make check`: gofmt, vet and the tests; the standard
  library alone; every test under TinyGo on a 32 KB stack, and the stack
  budget; js/wasm; and the comparison with cel-go in `oracle/`.
- `make fuzz`: a minute each of the fuzz targets: trees built by hand,
  and generated expressions and source text against cel-go.
- `make bench`: compiling and evaluating; `oracle/` has cel-go's side.
- `make stack`: the stack each phase needs at the limits, under TinyGo.

## License

Apache License 2.0: see [LICENSE](LICENSE).
