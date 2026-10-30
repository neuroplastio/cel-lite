# cel-lite

A subset of [CEL](https://cel.dev), the Common Expression Language, in Go's
standard library alone. It builds for js/wasm and under TinyGo, and its
parsed and checked tree is public, for programs that generate code from an
expression. `cel-lite` is a working name; the package is `cellite`.

Not published yet: there is no tagged release.

```go
env, err := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
	{Name: "mode", Type: cellite.String},
	{Name: "pane.zoomed", Type: cellite.Bool},
}})
prg, err := env.Compile(`mode == 'normal' && !pane.zoomed`)
holds, unknown := prg.Test(env.Activation(map[string]any{"mode": "normal", "pane.zoomed": false}))
```

## What it runs

- Literals, lists and maps; `!`, `-`, `&&`, `||`, the ternary, `==`, `!=`,
  `<`, `<=`, `>`, `>=`, `in`, `+`, `-`, `*`, `/`, `%`; field selection,
  indexing and `has()`.
- `size`, `contains`, `startsWith`, `endsWith`, and the conversions `int`,
  `uint`, `double`, `string`, `bytes`, `bool` and `dyn`.
- Structs the program declares, read by field index through an `Object`
  interface; lists and maps through `List` and `Map`, so the program's own
  data goes in without copying.
- Helpers: functions the program registers, with typed overloads resolved
  when the expression is checked.
- Unknown variables, for partial evaluation, with CEL's rules for `&&`,
  `||` and errors.

Refused by name when an expression is compiled: timestamps and durations,
regular expressions, the comprehension macros (`all`, `exists`, `map`,
`filter`), type values, message construction and optional syntax.

## Meaning

An expression means what CEL's language definition says. cel-go v0.32 is
the oracle: the tests in `oracle/` compare the two engines on a corpus of
corner cases, on generated expressions with absent and unknown variables,
on real keymap contexts, and under two fuzz targets. Where cel-go strays
from the definition, cel-lite follows the definition or refuses by name;
the package documentation lists each difference.

## Limits

An expression is at most 100,000 bytes, its tree at most 50 deep, and its
brackets, calls and conditionals nest at most 16 deep. Within those limits
each phase (parse, check, plan, eval) needs at most 13.2 KB of stack under
TinyGo's wasm, where a goroutine's stack is a fixed 64 KB. Nothing panics,
on any input.

## Speed

Medians of `make bench` on an AMD Ryzen AI 9 HX 370; `oracle/` runs the
same expressions through cel-go.

| | cel-lite | under TinyGo, wasm | cel-go |
|---|---|---|---|
| eval `mode == 'normal' && !pane.zoomed` | 60 ns | 76 ns | 92 ns |
| eval `row.icon != '' ? row.icon : '-'` | 50 ns | 66 ns | 71 ns |
| eval `rows[n].name == row.name` | 116 ns | 159 ns | 429 ns |
| load: parse, check, plan | 0.2–2.0 µs, 5–22 allocs | | 7–33 µs, 120–400 allocs |

Evaluating allocates nothing unless the expression makes a string or
calls a helper.

## Developing

Go and TinyGo are pinned in `mise.toml`.

- `mise exec -- make check`: gofmt, vet, tests, the standard library alone,
  every test under TinyGo on a 32 KB stack, the stack budget, js/wasm and
  the oracle.
- `make stack`: the stack each phase needs within the limits, under TinyGo.
- `make fuzz`: a minute each of the two fuzz targets against cel-go.
- `make bench`: load (parse, check, plan) and eval, on expressions shaped
  like keymap contexts and template bindings; `oracle/` has cel-go's side.

## License

Apache-2.0.
