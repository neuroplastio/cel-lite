package cellite_test

import (
	"errors"
	"fmt"
	"strings"

	"neuroplast.io/cel-lite"
)

// A program declares the variables an expression may read, compiles the
// expression once, and tests it as often as it likes.
func Example() {
	env, err := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "mode", Type: cellite.String},
		{Name: "pane.zoomed", Type: cellite.Bool},
	}})
	if err != nil {
		panic(err)
	}
	prg, err := env.Compile(`mode == 'normal' && !pane.zoomed`)
	if err != nil {
		panic(err)
	}
	act := env.Activation(map[string]any{
		"mode":        "normal",
		"pane.zoomed": false,
	})
	holds, unknown := prg.Test(act)
	fmt.Println(holds, unknown)
	// Output: true false
}

// Eval gives the expression's value, of the type Check found.
func ExampleEnv_Compile() {
	env, _ := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "title", Type: cellite.String},
		{Name: "count", Type: cellite.Int},
	}})
	prg, err := env.Compile(`count > 0 ? title + ' (' + string(count) + ')' : title`)
	if err != nil {
		panic(err)
	}
	v, err := prg.Eval(env.Activation(map[string]any{"title": "Inbox", "count": 3}))
	if err != nil {
		panic(err)
	}
	fmt.Println(cellite.FormatType(prg.Type()), v.String())
	// Output: string Inbox (3)
}

// A helper is a Go function an expression calls, with typed overloads:
// a call is resolved when the expression is compiled. Member makes it a
// method, its receiver the first argument.
func ExampleNewEnv_helper() {
	upper := func(args []cellite.Value) (cellite.Value, error) {
		return cellite.StringValue(strings.ToUpper(args[0].String())), nil
	}
	env, err := cellite.NewEnv(cellite.Decls{
		Vars: []cellite.Var{{Name: "name", Type: cellite.String}},
		Funcs: []cellite.Func{{Name: "upper", Overloads: []cellite.Overload{{
			ID:     "upper_string",
			Member: true,
			Args:   []cellite.Type{cellite.String},
			Result: cellite.String,
			Fn:     upper,
		}}}},
	})
	if err != nil {
		panic(err)
	}
	prg, _ := env.Compile(`name.upper() == 'ADA'`)
	holds, _ := prg.Test(env.Activation(map[string]any{"name": "Ada"}))
	fmt.Println(holds, prg.Expr().Args[0].Overload)
	// Output: true upper_string
}

// file is a program's own struct, which an expression reads through
// cellite.Object, field by index, without copying it.
type file struct {
	name string
	size int64
}

func (f *file) Field(i int) cellite.Value {
	switch i {
	case 0:
		return cellite.StringValue(f.name)
	case 1:
		return cellite.IntValue(f.size)
	}
	return cellite.NullValue()
}

// files is a slice of them as a cellite.List.
type files []*file

func (fs files) Len() int { return len(fs) }

func (fs files) At(i int) cellite.Value { return cellite.ObjectValue("File", fs[i]) }

// A struct the program declares is read by field index, which Check
// resolves from the field's name; lists and maps of the program's own
// come in through List and Map.
func ExampleNewEnv_structs() {
	env, err := cellite.NewEnv(cellite.Decls{
		Structs: []cellite.Struct{{Name: "File", Fields: []cellite.Field{
			{Name: "name", Type: cellite.String},
			{Name: "size", Type: cellite.Int},
		}}},
		Vars: []cellite.Var{
			{Name: "files", Type: cellite.ListOf(cellite.StructOf("File"))},
		},
	})
	if err != nil {
		panic(err)
	}
	prg, err := env.Compile(`size(files) > 1 && files[1].size > 1024`)
	if err != nil {
		panic(err)
	}
	fs := files{{name: "a.txt", size: 12}, {name: "b.bin", size: 4096}}
	holds, _ := prg.Test(env.Activation(map[string]any{
		"files": cellite.ListValue(fs),
	}))
	fmt.Println(holds)
	// Output: true
}

// A variable the activation calls unknown makes unknown what depends on
// it, and only that: partial evaluation.
func ExampleProgram_Test() {
	env, _ := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "ready", Type: cellite.Bool},
		{Name: "user.admin", Type: cellite.Bool},
	}})
	prg, _ := env.Compile(`ready && user.admin`)
	for _, ready := range []bool{false, true} {
		act := env.Activation(map[string]any{"ready": ready}, "user.admin")
		holds, unknown := prg.Test(act)
		fmt.Println(ready, holds, unknown)
	}
	// Output:
	// false false false
	// true false true
}

// An expression is refused when it is compiled: a syntax error, an
// undeclared name, types that don't fit, or CEL that cel-lite doesn't
// implement, which it names.
func ExampleError() {
	env, _ := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "tags", Type: cellite.ListOf(cellite.String)},
	}})
	for _, src := range []string{
		`tags[0] ==`,
		`tag == 'x'`,
		`tags[0] + 1`,
		`tags.exists(t, t == 'x')`,
	} {
		_, err := env.Compile(src)
		var e *cellite.Error
		if errors.As(err, &e) && e.Kind == cellite.Unsupported {
			fmt.Printf("unsupported %q: %v\n", e.Name, err)
			continue
		}
		fmt.Println(err)
	}
	// Output:
	// 1:11: syntax error: unexpected end of input
	// 1:1: undeclared reference to 'tag'
	// 1:9: found no matching overload for '_+_' applied to '(string, int)'
	// unsupported "macro exists": 1:6: the macro exists() is not supported: comprehensions (all, exists, exists_one, map, filter) are not implemented
}

// Check types every node of the tree Parse reads, and resolves names to
// variables by index: a program can generate code of its own from it.
func ExampleEnv_Check() {
	env, _ := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "pane.zoomed", Type: cellite.Bool},
		{Name: "title", Type: cellite.String},
	}})
	x, err := cellite.Parse(`!pane.zoomed && size(title) > 3`)
	if err != nil {
		panic(err)
	}
	y, err := env.Check(x)
	if err != nil {
		panic(err)
	}
	var walk func(x *cellite.Expr, indent string)
	walk = func(x *cellite.Expr, indent string) {
		fmt.Printf("%s%s %s", indent, opName(x.Op), cellite.FormatType(x.Type))
		switch x.Op {
		case cellite.OpVar:
			fmt.Printf(" %s (variable %d)", x.Name, x.Ref)
		case cellite.OpCall:
			fmt.Printf(" %s (%s)", x.Name, x.Overload)
		case cellite.OpLit:
			fmt.Printf(" %v", x.Value)
		}
		fmt.Println()
		for _, a := range x.Args {
			walk(a, indent+"  ")
		}
	}
	walk(y, "")
	// Output:
	// && bool
	//   ! bool
	//     var bool pane.zoomed (variable 0)
	//   > bool
	//     call int size (size_string)
	//       var string title (variable 1)
	//     literal int 3
}

func opName(op cellite.Op) string {
	switch op {
	case cellite.OpAnd:
		return "&&"
	case cellite.OpNot:
		return "!"
	case cellite.OpGt:
		return ">"
	case cellite.OpVar:
		return "var"
	case cellite.OpCall:
		return "call"
	case cellite.OpLit:
		return "literal"
	}
	return fmt.Sprint("op ", int(op))
}

// ValueOf reads a Go value as CEL does, without copying: here a map from
// a decoded document, read with a dyn variable.
func ExampleValueOf() {
	env, _ := cellite.NewEnv(cellite.Decls{Vars: []cellite.Var{
		{Name: "doc", Type: cellite.Dyn},
	}})
	prg, _ := env.Compile(`doc.kind == 'pod' && 'app' in doc.labels`)
	doc := map[string]any{
		"kind":   "pod",
		"labels": map[string]any{"app": "web"},
	}
	holds, _ := prg.Test(env.Activation(map[string]any{"doc": doc}))
	fmt.Println(holds)
	// Output: true
}

// A program that declares names it doesn't choose, such as a table's
// keys, leaves out the words CEL keeps. A type's name may be a
// variable's.
func ExampleReservedWords() {
	reserved := map[string]bool{}
	for _, w := range cellite.ReservedWords() {
		reserved[w] = true
	}
	var vars []cellite.Var
	for _, key := range []string{"name", "in", "list", "if"} {
		if !reserved[key] {
			vars = append(vars, cellite.Var{Name: key, Type: cellite.Dyn})
		}
	}
	_, err := cellite.NewEnv(cellite.Decls{Vars: vars})
	fmt.Println(len(vars), err)
	// Output: 2 <nil>
}
