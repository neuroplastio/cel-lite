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
//	act := env.Activation(map[string]any{"pane": true, "pane.id": "x"})
//	holds, unknown := prg.Test(act)
//
// Compile is Parse, then Check, then Program. Each step is public: Parse
// gives the syntax tree (an *Expr), Check resolves its names and types
// every node, and Program plans the checked tree for evaluation.
//
// The guides (getting started, the language, embedding, limits and
// TinyGo, performance) are in the repository's docs directory:
// https://github.com/neuroplastio/cel-lite/tree/main/docs.
//
// # Meaning
//
// An expression means what CEL's language definition says it means, and
// what it means in cel-go (v0.32), which the oracle module's tests
// compare it with. Or it is refused when it is compiled, with an *Error of
// kind Unsupported that names what is missing: timestamps and durations,
// regular expressions, macros (all, exists, map, filter), type values,
// message construction and optional syntax.
//
// # Where cel-lite and cel-go differ
//
// Where cel-go strays from the language definition, cel-lite follows the
// definition, or refuses by name:
//
//   - cel-go types a value read with [] from a dyn (`x['k']`, or an
//     element of an empty literal) with a type parameter that its first
//     use binds: `-x['k']` is a double there, `[x['k'], 'a']` a
//     list(string), `-x['k'] + 1` is refused, and `int(x['k'])` takes
//     only an int when it runs. In the definition the value is dyn. Check
//     refuses such a use (Unsupported, "inferred type"); x.k or
//     dyn(x['k']) keeps the value dyn.
//   - A map's keys are bool, int, uint or string (the definition's
//     Values). A map literal whose dyn key turns out a double, a list or a
//     map is an error when it runs; cel-go builds the map, where a NaN key
//     is never found and two NaN keys are two entries.
//   - A variable may take a type's name (`list`, `int`), and the name
//     then reads the variable. When the variable is absent, cel-go reads
//     the type instead, a value the expression's checked type doesn't
//     allow: `list` declared a bool gives the type list. cel-lite reports
//     the variable missing, as for any other name.
//   - cel-go reads a struct's field by its name with [] when a dyn hides
//     the struct, `dyn(item)['title']`. In the definition [] reads a list
//     or a map; cel-lite gives an error (no such overload).
//   - `x in m` with x bytes, as untyped data can give a variable, is
//     false on any map: bytes are never a key. cel-go fails on a map
//     literal, which can't hash them, and says false on a Go map.
//   - cel-lite's limits are lower than cel-go's (Limits).
//
// # Values
//
// A Value is a CEL value. Lists, maps and structs reach the evaluator
// through the List, Map and Object interfaces, so a program can hand over
// its own data without copying it; ValueOf wraps Go's own slices and maps.
// A struct the program declares (Decls.Structs) is read by field index,
// which Check resolves, through Object. cel-lite doesn't compare structs:
// Check refuses == on them, and where a dyn hides them the evaluation
// does, unless the answer doesn't depend on it. A struct against null
// compares none, and the value decides: a struct is never null.
//
// # Limits
//
// Nothing here panics on any input, since recover doesn't work under
// TinyGo's wasm. An expression is at most MaxSize bytes, its tree at most
// MaxDepth (50) deep, and its brackets, calls and conditionals nest at
// most 16 deep. Within those limits the parser, the checker, the planner
// and the evaluator each need at most 13.2 KB of stack under TinyGo 0.42
// on wasm (`make stack` measures it, against a budget of 16 KB), which
// leaves most of a TinyGo goroutine's fixed 64 KB to the program around
// them; the gate runs every test on a 32 KB stack. Integers are int64 and
// uint64 everywhere, and overflow is an error.
package cellite
