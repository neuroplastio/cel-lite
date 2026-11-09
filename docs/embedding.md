---
group: Guides
order: 10
nav: Embedding
description: environments, types, values, activations, helpers, programs, errors, and the tree for generating code
---

# Embedding

This page is the tour of cel-lite's API for a program that embeds it.
The [package reference](https://pkg.go.dev/neuroplast.io/cel-lite)
has every identifier, with runnable examples.

## The environment

An `Env` holds what your program declares to its expressions: variables,
the structs their values may be, and helpers. Make one with `NewEnv`, and
compile as many expressions against it as you like. An Env, like a
Program, is safe for concurrent use.

```go
env, err := cellite.NewEnv(cellite.Decls{
	Vars:    []cellite.Var{ /* what an expression reads */ },
	Structs: []cellite.Struct{ /* the struct types they may hold */ },
	Funcs:   []cellite.Func{ /* the helpers it may call */ },
})
```

NewEnv checks the declarations. These are errors: a name that isn't an
identifier (a variable's may be qualified, `pane.id`), a reserved word, a
name declared twice, a type that names a struct not declared, a helper
named as a standard function or a type, an overload ID used twice, and a
map key type other than bool, int, uint or string.

### Types

| Type                | Go                                     |
| ------------------- | -------------------------------------- |
| bool, int, uint, …  | `cellite.Bool`, `cellite.Int`, `cellite.Uint`, `cellite.Double`, `cellite.String`, `cellite.Bytes` |
| any value           | `cellite.Dyn`                          |
| a list              | `cellite.ListOf(cellite.String)`       |
| a map               | `cellite.MapOf(cellite.String, cellite.Int)` |
| a declared struct   | `cellite.StructOf("File")`             |

`cellite.FormatType` writes a type as CEL does (`map(string, int)`), and
`cellite.TypeEqual` compares two. The zero `Type` is null's, which no
variable or field may have: say `cellite.Dyn` for a value of any type.

## Values

A `cellite.Value` is a CEL value: what an activation gives, what a helper
takes and returns, and what `Eval` gives. It is small and passed by value;
making one from a number or a string, or reading one, doesn't allocate.

- Make one with `IntValue`, `UintValue`, `DoubleValue`, `StringValue`,
  `BytesValue`, `BoolValue`, `NullValue`, `ListValue`, `MapValue` and
  `ObjectValue`.
- Read one with `Kind`, then `Int`, `Uint`, `Double`, `String`, `Bytes`,
  `Bool`, `List`, `Map` or `Object`. `Interface` gives it back as a Go
  value, and `cellite.Text` is CEL's `string()` of it.
- `cellite.ValueOf` reads a Go value as CEL does, without copying it:
  nil, bools, strings, `[]byte`, Go's integers (signed as int, unsigned as
  uint) and floats, a `Value`, a `List` or a `Map`; slices of `any`,
  `string`, `int64`, `int`, `float64`, `bool` or `Value`; and maps from
  string to `any`, `string`, `int64`, `int`, `bool`, `float64` or
  `Value`. That covers what `encoding/json` decodes into an `any`.

### Your own data

Your program's lists, maps and structs go in as they are, through three
interfaces, so nothing is copied into Values first:

```go
type List interface {
	Len() int
	At(i int) Value
}

type Map interface {
	Len() int
	Get(key Value) (Value, bool)
	Range(f func(k, v Value) bool)
}

type Object interface {
	Field(i int) Value
}
```

A struct is declared with its fields, and an expression reads a field by
its index in that declaration, which Check resolves from the field's name.
Reading a field is then a switch, with no names and no reflection:

```go
env, err := cellite.NewEnv(cellite.Decls{
	Structs: []cellite.Struct{{Name: "File", Fields: []cellite.Field{
		{Name: "name", Type: cellite.String},
		{Name: "size", Type: cellite.Int},
	}}},
	Vars: []cellite.Var{{Name: "file", Type: cellite.StructOf("File")}},
})

type file struct {
	name string
	size int64
}

// Field is the field at its index in the File declaration.
func (f *file) Field(i int) cellite.Value {
	switch i {
	case 0:
		return cellite.StringValue(f.name)
	case 1:
		return cellite.IntValue(f.size)
	}
	return cellite.NullValue()
}
```

The value is `cellite.ObjectValue("File", f)`, and `file.size > 1024`
reads it.

A field whose type is a struct may have no value, like a nil pointer.
Return an Object of the struct's zero value to read it as cel-go reads a
native type's nil pointer, and protobuf an unset message: `a.next.size`
is then `0`, and `a.next == null` is `false`. Return
`cellite.NullValue()` instead, and `a.next == null` is `true`, which
makes it a presence test, while selecting from it is an error:
`a.next != null && a.next.size > 0` reads safely.

cel-lite doesn't compare structs. Check refuses `==` and `in` on them,
and so does the evaluation where a dyn hid them from Check, unless the
answer doesn't depend on it. A struct against `null` compares none: the
value decides.

## Activations

An activation gives one evaluation its variables' values: for each
variable, by its index in the order declared, a value and its presence.

```go
type Activation interface {
	Var(i int) (Value, Presence)
}
```

`Present` is a value. `Absent` makes reading the variable an error, as
reading a missing attribute is in CEL. `Unknown` makes what depends on it
unknown.

`env.Activation(vals, unknown...)` makes one from a map of values by name,
read by `ValueOf`, with the variables named in `unknown` unknown. It
allocates and copies, so make one when the values change, not for every
evaluation. A program that evaluates often can implement `Activation`
over its own state instead, with no maps and no copying:

```go
// state is read by the variables mode and pane.zoomed, declared in
// that order.
type state struct {
	mode   string
	zoomed bool
}

func (s *state) Var(i int) (cellite.Value, cellite.Presence) {
	switch i {
	case 0:
		return cellite.StringValue(s.mode), cellite.Present
	case 1:
		return cellite.BoolValue(s.zoomed), cellite.Present
	}
	return cellite.Value{}, cellite.Absent
}
```

`env.VarIndex(name)` and `env.Vars()` give the indexes, for a program
that declares its variables from data.

## Helpers

A helper is a Go function an expression can call. It has a name and one
or more overloads:

```go
cellite.Func{Name: "clamp", Overloads: []cellite.Overload{{
	ID:     "clamp_int_int_int",
	Args:   []cellite.Type{cellite.Int, cellite.Int, cellite.Int},
	Result: cellite.Int,
	Fn: func(a []cellite.Value) (cellite.Value, error) {
		x, lo, hi := a[0].Int(), a[1].Int(), a[2].Int()
		return cellite.IntValue(min(max(x, lo), hi)), nil
	},
}}}
```

- A call is matched to an overload when the expression compiles: one
  that fits no overload is a type error. Where more than one fits, as with
  dyn arguments, the values decide when it runs.
- `Member: true` makes it a method: `x.clamp(0, 9)`, its receiver the
  first argument.
- `ID` names the overload in the checked tree (`Expr.Overload`), for a
  program that generates code from it. It is unique in the Env.
- `Fn` gets values only. An argument that is an error or unknown makes
  the call an error or unknown without calling `Fn`. An error `Fn` returns
  is the expression's, as an `*EvalError` whose `Err` it is, so
  `errors.Is` and `errors.As` find it.
- `Fn` must not panic: under TinyGo's WebAssembly nothing can recover.

A helper can't take the name of a standard function, a type or a reserved
word. `cellite.StandardFunctions()`, `cellite.TypeNames()` and
`cellite.ReservedWords()` list them, for a program that declares names it
doesn't choose, such as a table's keys.

## Programs

`env.Compile(src)` is three steps: `Parse`, then `env.Check`, then
`env.Program`. It gives a `*Program`, which is immutable and safe for
concurrent use.

- `prg.Eval(act)` gives the value, or an error: an `*EvalError`, or
  `cellite.ErrUnknown` when the value depends on an unknown variable.
- `prg.Test(act)` is for a bool expression: whether it holds, and whether
  it is unknown. An error is neither, and it doesn't allocate.
- `prg.Type()` is the expression's type, known before it runs: a program
  that needs a bool checks `prg.Type().Kind == cellite.BoolKind`.
- `prg.Expr()` is the checked tree.

## Errors

Compile's errors are `*cellite.Error`:

| `Kind`        | Why                                             | `Name`             |
| ------------- | ----------------------------------------------- | ------------------ |
| `SyntaxError` | the source isn't CEL, or a hand-built tree is malformed |            |
| `Undeclared`  | a name the Env doesn't declare                  | the name           |
| `TypeError`   | types that don't fit: `'a' + 1`                 |                    |
| `Unsupported` | CEL that cel-lite doesn't implement             | the feature        |

`Pos` is the byte offset in the source, and `Line` and `Col` (from 1, in
characters) are where that is. `Error()` writes them first:
`1:1: undeclared reference to 'tabel'`. Check alone has no source, so its
errors have `Pos` only.

```go
_, err := env.Compile(src)
var e *cellite.Error
if errors.As(err, &e) && e.Kind == cellite.Undeclared {
	// e.Name is the name: suggest the declared ones near it.
}
```

An error as the expression runs is an `*EvalError`: a missing key, a
division by zero, an overflow, or a helper's error (`Err`).

## The tree

Parse, Check and the checked tree are public, for programs that rewrite
an expression before checking it, or generate code from it rather than
evaluate it.

- `cellite.Parse(src)` reads the source into an `*Expr`, unchecked:
  names are `OpIdent` nodes until Check resolves them.
- `env.Check(x)` resolves names and types every node, and returns a new
  tree, leaving `x` as it is. A variable is `OpVar`, with `Ref` its index
  in the Env; a struct's field is `OpSelect`, with `Ref` the field's
  index; a call's `Overload` names what it runs, a helper's `ID` or a
  standard overload as cel-go names it (`size_string`, `string_to_int64`).
- `env.Program(x)` plans a tree Check returned in this Env.

An `Expr` is plain data: `Op`, `Pos`, `Type`, `Value` (a literal's),
`Name`, `Ref`, `Overload`, `Member` and `Args`, the operands in the order
the Op gives. Walk it with a switch on `Op`. A program may change a node
or give it new `Args`; the nodes are made in a few large chunks, so
appending to a node's `Args` copies them.

Check and Program refuse a malformed tree, one a program built or changed
by hand, with an error rather than a panic: a missing operand, operands
that don't fit the Op, or a reference to what the Env doesn't declare.
