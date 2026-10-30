package oracle

import (
	"reflect"
	"strconv"
	"testing"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/ext"
)

// The cellite package's benchmarks (bench_test.go there), through cel-go:
// the same expressions, declared as a cel-go program would declare them,
// so the two engines' numbers stand side by side.

var keymapExprs = []struct{ name, src string }{
	{"bool", `search`},
	{"and-not", `mode == 'normal' && !pane.zoomed`},
	{"and-chain", `dock && !field && !overlay`},
	{"string-eq", `viewer.kind == 'secret'`},
	{"in-map", `'k' in pane.labels`},
}

var templateExprs = []struct{ name, src string }{
	{"field", `row.name`},
	{"field-or", `row.icon != '' ? row.icon : '-'`},
	{"bool-text", `row.unread ? '●' : '○'`},
	{"size-and", `loaded && size(rows) == 0`},
	{"concat", `'l' + string(n)`},
	{"helper", `age(row.age)`},
	{"helper-in", `control(row) in ['number', 'text']`},
	{"index-field", `rows[n].name == row.name`},
}

func celKeymap(tb testing.TB) (*cel.Env, map[string]any) {
	tb.Helper()
	env, err := cel.NewEnv(
		cel.Variable("search", cel.BoolType), cel.Variable("mode", cel.StringType),
		cel.Variable("pane.zoomed", cel.BoolType), cel.Variable("dock", cel.BoolType),
		cel.Variable("field", cel.BoolType), cel.Variable("overlay", cel.BoolType),
		cel.Variable("viewer.kind", cel.StringType),
		cel.Variable("pane.labels", cel.MapType(cel.StringType, cel.StringType)),
	)
	if err != nil {
		tb.Fatal(err)
	}
	return env, map[string]any{
		"search": false, "mode": "normal", "pane.zoomed": false, "dock": true, "field": false,
		"overlay": false, "viewer.kind": "secret", "pane.labels": map[string]string{"k": "v"},
	}
}

// Row is the template's struct, as cel-go reads native Go structs.
type Row struct {
	Name    string `cel:"name"`
	Icon    string `cel:"icon"`
	Unread  bool   `cel:"unread"`
	Age     int64  `cel:"age"`
	Control string `cel:"control"`
}

func celTemplate(tb testing.TB) (*cel.Env, map[string]any) {
	tb.Helper()
	rowType := cel.ObjectType("oracle.Row")
	env, err := cel.NewEnv(
		ext.NativeTypes(reflect.TypeFor[Row](), ext.ParseStructTags(true)),
		cel.Variable("row", rowType), cel.Variable("rows", cel.ListType(rowType)),
		cel.Variable("loaded", cel.BoolType), cel.Variable("n", cel.IntType),
		cel.Function("age", cel.Overload("age_int", []*cel.Type{cel.IntType}, cel.StringType,
			cel.UnaryBinding(func(v ref.Val) ref.Val { return types.String(strconv.FormatInt(int64(v.(types.Int)), 10) + "s") }))),
		cel.Function("control", cel.Overload("control_row", []*cel.Type{rowType}, cel.StringType,
			cel.UnaryBinding(func(v ref.Val) ref.Val { return types.String(v.Value().(Row).Control) }))),
	)
	if err != nil {
		tb.Fatal(err)
	}
	r := Row{Name: "build", Unread: true, Age: 42, Control: "text"}
	return env, map[string]any{"row": r, "rows": []Row{r, {Name: "other"}}, "loaded": true, "n": int64(0)}
}

func BenchmarkCelGoLoad(b *testing.B) {
	run := func(b *testing.B, env *cel.Env, src string) {
		for b.Loop() {
			ast, iss := env.Compile(src)
			if iss.Err() != nil {
				b.Fatal(iss.Err())
			}
			if _, err := env.Program(ast); err != nil {
				b.Fatal(err)
			}
		}
	}
	kenv, _ := celKeymap(b)
	for _, e := range keymapExprs {
		b.Run("keymap/"+e.name, func(b *testing.B) { run(b, kenv, e.src) })
	}
	tenv, _ := celTemplate(b)
	for _, e := range templateExprs {
		b.Run("template/"+e.name, func(b *testing.B) { run(b, tenv, e.src) })
	}
}

func BenchmarkCelGoEval(b *testing.B) {
	run := func(b *testing.B, env *cel.Env, vals map[string]any, src string) {
		ast, iss := env.Compile(src)
		if iss.Err() != nil {
			b.Fatal(iss.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			b.Fatal(err)
		}
		// A prepared activation, as a program that evaluates often keeps.
		act, err := cel.NewActivation(vals)
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := prg.Eval(act); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			prg.Eval(act)
		}
	}
	kenv, kvals := celKeymap(b)
	for _, e := range keymapExprs {
		b.Run("keymap/"+e.name, func(b *testing.B) { run(b, kenv, kvals, e.src) })
	}
	tenv, tvals := celTemplate(b)
	for _, e := range templateExprs {
		b.Run("template/"+e.name, func(b *testing.B) { run(b, tenv, tvals, e.src) })
	}
}
