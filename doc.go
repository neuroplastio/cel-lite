// Package cellite runs a subset of CEL, the Common Expression Language,
// with Go's standard library alone, so it builds for js/wasm and under
// TinyGo. Its parsed and checked tree is public, for programs that turn
// an expression into Go of their own.
//
// A program declares what an expression may name, compiles it once and
// evaluates the Program as often as it likes:
//
//	env, err := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
//		{Name: "pane", Type: cellite.Bool},
//		{Name: "pane.id", Type: cellite.String},
//	}})
//	prg, err := env.Compile(`pane && pane.id == 'x'`)
//	holds, unknown := prg.Test(env.Activation(map[string]any{"pane": true, "pane.id": "x"}))
//
// Compile is Parse, then Check, then Program. Each step is public: Parse
// gives the syntax tree (an *Expr), Check resolves its names and types
// every node, and Program plans the checked tree for evaluation.
//
// # Meaning
//
// An expression means what it means in cel-go (v0.32), which the oracle
// module's tests compare it with, or it is refused when it is compiled,
// with an *Error of kind Unsupported that names what is missing:
// timestamps and durations, regular expressions, macros (all, exists,
// map, filter), type values, message construction and optional syntax.
// cel-go types a value read with [] from a dyn (`x['k']`) with a type
// parameter that its first use binds, so that `-x['k']` is a double and
// `int(x['k'])` takes only an int; Check refuses such a use by name, and
// x.k or dyn(x['k']) keeps the value dyn.
//
// A map literal whose key turns out, when evaluated, to be anything but a
// bool, an int, a uint or a string is an error, as the CEL spec has it;
// cel-go builds the map.
//
// # Values
//
// A Value is a CEL value. Lists, maps and structs reach the evaluator
// through the List, Map and Object interfaces, so a program can hand over
// its own data without copying it; ValueOf wraps Go's own slices and maps.
// A struct the program declares (Decls.Structs) is read by field index,
// which Check resolves, through Object.
//
// # Limits
//
// Nothing here panics on any input, since recover doesn't work under
// TinyGo's wasm: an expression nests at most MaxDepth deep and is at most
// MaxSize bytes, which keeps recursion within TinyGo's 64 KB stacks.
// Integers are int64 and uint64 everywhere, and overflow is an error.
package cellite
