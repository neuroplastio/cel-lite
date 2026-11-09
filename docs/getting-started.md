---
group: Start
order: 10
nav: Getting started
description: install cel-lite, compile a first expression, and give it variables and a helper
---

# Getting started

cel-lite compiles an expression once and evaluates it as often as you
like. You declare what the expression may read, compile it against those
declarations, and hand each evaluation the values.

## Install

```sh
go get neuroplast.io/cel-lite
```

cel-lite needs Go 1.24 or later, and nothing else: the module requires no
other module. The package is `cellite`.

## A first expression

An expression with no variables compiles in an empty environment:

```go
package main

import (
	"fmt"
	"log"

	"neuroplast.io/cel-lite"
)

func main() {
	env, err := cellite.NewEnv(cellite.Decls{})
	if err != nil {
		log.Fatal(err)
	}
	prg, err := env.Compile(`size('héllo') * 2 + 1`)
	if err != nil {
		log.Fatal(err)
	}
	v, err := prg.Eval(env.Activation(nil))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cellite.FormatType(prg.Type()), v.Int()) // int 11
}
```

Compile parses the expression, checks its types, and plans it for
evaluation. Its error is a `*cellite.Error` that says where and why;
compiling `'héllo' + 1` gives:

```
1:9: found no matching overload for '_+_' applied to '(string, int)'
```

The program's type is known when it compiles (`prg.Type()`), so a program
that wants a bool can refuse anything else before it runs anything.

## Variables

Declare each variable with its name and type. A name may be qualified,
`pane.id`, and then the expression `pane.id` reads that variable:

```go
env, err := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
	{Name: "mode", Type: cellite.String},
	{Name: "pane.zoomed", Type: cellite.Bool},
	{Name: "pane.labels", Type: cellite.MapOf(cellite.String, cellite.String)},
}})
if err != nil {
	log.Fatal(err)
}
prg, err := env.Compile(`mode == 'normal' && 'pinned' in pane.labels`)
if err != nil {
	log.Fatal(err)
}
```

An activation gives an evaluation its variables' values. `env.Activation`
makes one from a map, reading Go values as CEL values:

```go
act := env.Activation(map[string]any{
	"mode":        "normal",
	"pane.zoomed": false,
	"pane.labels": map[string]string{"pinned": "yes"},
})
holds, unknown := prg.Test(act) // true, false
```

`Test` is for a bool expression: whether it holds, and whether its value
is unknown. An error, such as reading a variable the activation doesn't
have, is neither. `Eval` gives the value itself, or the error.

A variable can also be unknown: its value isn't there yet. An expression
that depends on it is unknown, and one that doesn't, isn't:

```go
prg, _ := env.Compile(`mode == 'insert' && pane.zoomed`)
act := env.Activation(map[string]any{"mode": "normal"}, "pane.zoomed")
holds, unknown := prg.Test(act) // false, false: mode decides it
```

## A helper

A helper is a Go function that an expression calls. Declare it with one
or more overloads, each with its argument and result types; a call is
matched to an overload when the expression compiles, so a wrong call is a
compile error, not a surprise later:

```go
initials := func(args []cellite.Value) (cellite.Value, error) {
	var b strings.Builder
	for _, w := range strings.Fields(args[0].String()) {
		r, _ := utf8.DecodeRuneInString(w)
		b.WriteRune(r)
	}
	return cellite.StringValue(b.String()), nil
}
env, err := cellite.NewEnv(cellite.Decls{
	Vars: []cellite.Var{{Name: "user.name", Type: cellite.String}},
	Funcs: []cellite.Func{{Name: "initials", Overloads: []cellite.Overload{{
		ID:     "initials_string",
		Args:   []cellite.Type{cellite.String},
		Result: cellite.String,
		Fn:     initials,
	}}}},
})
if err != nil {
	log.Fatal(err)
}
prg, err := env.Compile(`initials(user.name) == 'AL'`)
```

Set `Member: true` on an overload to call it as a method,
`user.name.initials()`, with the receiver as its first argument. A helper
gets values only: an argument that is an error or unknown makes the call
an error or unknown without calling the helper. An error the helper
returns becomes the expression's.

## Next

- [The language](language.md): everything the subset holds, and what it
  refuses.
- [Embedding](embedding.md): structs, your own lists and maps, faster
  activations, errors, and the tree for generating code.
- [Limits, TinyGo and WebAssembly](tinygo.md): building for small
  targets.
