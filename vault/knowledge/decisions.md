# cel-lite decisions

Append-only. Supersede a decision instead of contradicting it.

## D1: A module of its own, the standard library alone (2026-10-30)

cel-lite is its own module, github.com/neuroplastio/cel-lite, package
`cellite`, so that myelin and hotty-surf can both use it and myelin isn't
wired to it yet (the maintainer, through gov). It requires nothing, and it
builds for js/wasm and under TinyGo. cel-go is a test oracle in `oracle/`,
a nested module, linked into no binary.

## D2: The language definition first, cel-go as the oracle (2026-10-30)

An expression means what CEL's language definition says, and what cel-go
v0.32 does wherever the two agree. Where cel-go strays, cel-lite follows
the definition or refuses by name, lists the difference in doc.go, and the
oracle excludes the case by name, citing the definition (gov).

## D3: Type parameters that cel-go binds are refused (2026-10-30)

cel-go types `x['k']` on a dyn, and an element of an empty literal, with a
type parameter that its first use binds: `-x['k']` is a double,
`[x['k'], 'a']` a list(string), and `int(x['k'])` calls only the int
overload when it runs. In the definition the value is dyn. Check refuses
such a use (Unsupported "inferred type"). x.k or dyn(x['k']) keeps it dyn.

## D4: Map keys as the definition has them (2026-10-30)

A map's keys are bool, int, uint or string (langdef.md, Values). A static
key of another type is refused; a dyn key that turns out a double, a list
or a map is an error when the literal is evaluated. cel-go builds such a
map, where a NaN key is never found.

## D5: Limits from the stack they need under TinyGo (2026-10-30)

A TinyGo goroutine's stack is a fixed 64 KB, checked only at a pause, so
an overflow corrupts the heap silently. Each phase (parse, check, plan,
eval) stays within 16 KB at the limits: MaxDepth 50, nesting 16, MaxSize
100,000 bytes. The worst cases are generated at exactly the limits, and
TestStackUse fails past the budget; a limit is raised only with a new
measurement (gov).

## D6: A public, checked tree (2026-10-30)

Parse, Check and Program are separate and public. Check returns a new tree
with every node typed; Ref is a variable's index (OpVar) or a struct
field's index (OpSelect), and Overload names the one overload that fits,
for generators such as hotty-surf's.

## D7: A tree's nodes in chunks (2026-10-30)

Parse and Check make a tree's nodes and its operand lists in a few
chunks, sized by the tokens (Parse) or the tree (Check), not one
allocation a node. A program may change a node or give it new Args;
an Args slice has no room past its end, so appending to it copies. A
node kept keeps its chunk.

## D8: A variable may take a type's name (2026-10-30)

cel-go lets a variable take any of CEL's type names (`list`, `int`,
`type`, `null_type`, …), and the variable wins: with `list` declared,
`list` reads it. cel-lite does the same, since programs declare names
they don't choose (a keymap's node `list`). Reserved words stay refused,
and structs and helpers still can't take a type's name. When the
variable is absent, cel-go reads the type instead, which the checked
type doesn't allow; cel-lite reports the variable missing (D2: doc.go
lists it, the oracle leaves it out by name) (gov).

## D9: Untyped values keep their checked overload; null against a struct (2026-10-30)

Untyped data (a decoded file, a program's `map[string]any`) can give a
variable a value of another kind than declared. A conversion keeps the
overload Check chose, as cel-go does: `int(s)` with s a string given an
int is an error (no such overload), not 1. `x in m` with x bytes stays
false on any map, since bytes are never a key; cel-go fails only on a map
literal, which can't hash them (D2: doc.go lists it, the oracle leaves it
out by name). `x == null` and `x != null` with x a struct compare no
structs, so Check lets them through and the value decides: a struct is
never null, and a NullValue an Object gives for an unset field is. With
cel-go's zero struct for a nil pointer it is false; with NullValue it is
a presence test, which hotty-surf's templates use (gov, hotty-ui).
