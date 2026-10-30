package cellite

import (
	"strconv"
	"testing"
)

// The benchmarks' expressions have the shapes of cel-lite's two users,
// with names of their own: a keymap's contexts, short bool checks made on
// every key, and a template's bindings, made on every redraw. The oracle
// module runs the same ones through cel-go (oracle/bench_test.go).

// keymapExprs are contexts a keymap binds keys in.
var keymapExprs = []struct{ name, src string }{
	{"bool", `search`},
	{"and-not", `mode == 'normal' && !pane.zoomed`},
	{"and-chain", `dock && !field && !overlay`},
	{"string-eq", `viewer.kind == 'secret'`},
	{"in-map", `'k' in pane.labels`},
}

// templateExprs are a template's bindings: text, conditions and
// attributes.
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

func keymapEnv(tb testing.TB) (*Env, Activation) {
	tb.Helper()
	env, err := NewEnv(Decls{Vars: []Var{
		{Name: "search", Type: Bool}, {Name: "mode", Type: String}, {Name: "pane.zoomed", Type: Bool},
		{Name: "dock", Type: Bool}, {Name: "field", Type: Bool}, {Name: "overlay", Type: Bool},
		{Name: "viewer.kind", Type: String}, {Name: "pane.labels", Type: MapOf(String, String)},
	}})
	if err != nil {
		tb.Fatal(err)
	}
	return env, vars{
		BoolValue(false), StringValue("normal"), BoolValue(false), BoolValue(true), BoolValue(false),
		BoolValue(false), StringValue("secret"), ValueOf(map[string]string{"k": "v"}),
	}
}

// row is a struct the template's program declares; its Object is what a
// generator would write.
type row struct {
	name, icon string
	unread     bool
	age        int64
	control    string
}

func (r *row) Field(i int) Value {
	switch i {
	case 0:
		return StringValue(r.name)
	case 1:
		return StringValue(r.icon)
	case 2:
		return BoolValue(r.unread)
	case 3:
		return IntValue(r.age)
	case 4:
		return StringValue(r.control)
	}
	return NullValue()
}

type rows []*row

func (l rows) Len() int       { return len(l) }
func (l rows) At(i int) Value { return ObjectValue("Row", l[i]) }

func templateEnv(tb testing.TB) (*Env, Activation) {
	tb.Helper()
	env, err := NewEnv(Decls{
		Structs: []Struct{{Name: "Row", Fields: []Field{
			{Name: "name", Type: String}, {Name: "icon", Type: String}, {Name: "unread", Type: Bool},
			{Name: "age", Type: Int}, {Name: "control", Type: String},
		}}},
		Vars: []Var{
			{Name: "row", Type: StructOf("Row")}, {Name: "rows", Type: ListOf(StructOf("Row"))},
			{Name: "loaded", Type: Bool}, {Name: "n", Type: Int},
		},
		Funcs: []Func{
			{Name: "age", Overloads: []Overload{{ID: "age_int", Args: []Type{Int}, Result: String,
				Fn: func(a []Value) (Value, error) { return StringValue(strconv.FormatInt(a[0].Int(), 10) + "s"), nil }}}},
			{Name: "control", Overloads: []Overload{{ID: "control_row", Args: []Type{StructOf("Row")}, Result: String,
				Fn: func(a []Value) (Value, error) { return StringValue(a[0].Object().(*row).control), nil }}}},
		},
	})
	if err != nil {
		tb.Fatal(err)
	}
	r := &row{name: "build", icon: "", unread: true, age: 42, control: "text"}
	return env, vars{ObjectValue("Row", r), ListValue(rows{r, {name: "other"}}), BoolValue(true), IntValue(0)}
}

// vars is an Activation over a slice, by variable index.
type vars []Value

func (v vars) Var(i int) (Value, Presence) { return v[i], Present }

func BenchmarkLoad(b *testing.B) {
	run := func(b *testing.B, env *Env, src string) {
		// b.N, not b.Loop: TinyGo's testing doesn't have Loop.
		for range b.N {
			if _, err := env.Compile(src); err != nil {
				b.Fatal(err)
			}
		}
	}
	kenv, _ := keymapEnv(b)
	for _, e := range keymapExprs {
		b.Run("keymap/"+e.name, func(b *testing.B) { run(b, kenv, e.src) })
	}
	tenv, _ := templateEnv(b)
	for _, e := range templateExprs {
		b.Run("template/"+e.name, func(b *testing.B) { run(b, tenv, e.src) })
	}
}

func BenchmarkEval(b *testing.B) {
	run := func(b *testing.B, env *Env, act Activation, src string) {
		p, err := env.Compile(src)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := p.Eval(act); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			p.Eval(act)
		}
	}
	kenv, kact := keymapEnv(b)
	for _, e := range keymapExprs {
		b.Run("keymap/"+e.name, func(b *testing.B) { run(b, kenv, kact, e.src) })
	}
	tenv, tact := templateEnv(b)
	for _, e := range templateExprs {
		b.Run("template/"+e.name, func(b *testing.B) { run(b, tenv, tact, e.src) })
	}
}
