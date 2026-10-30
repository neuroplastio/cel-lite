package cellite

import (
	"errors"
	"sort"
	"strings"
	"testing"
)

// item is a struct the program declares, read through Object by field
// index: title, count, tags, next.
type item struct {
	title string
	count int64
	tags  []string
	next  *item
}

func (it *item) Field(i int) Value {
	switch i {
	case 0:
		return StringValue(it.title)
	case 1:
		return IntValue(it.count)
	case 2:
		return ValueOf(it.tags)
	case 3:
		if it.next == nil {
			return NullValue()
		}
		return ObjectValue("Item", it.next)
	}
	return NullValue()
}

// items is a list of items, read through List.
type items []*item

func (l items) Len() int       { return len(l) }
func (l items) At(i int) Value { return ObjectValue("Item", l[i]) }

var errHelper = errors.New("helper failed")

func testEnv(t testing.TB) *Env {
	t.Helper()
	env, err := NewEnv(Decls{
		Structs: []Struct{{Name: "Item", Fields: []Field{
			{Name: "title", Type: String}, {Name: "count", Type: Int},
			{Name: "tags", Type: ListOf(String)}, {Name: "next", Type: StructOf("Item")},
		}}},
		Vars: []Var{
			{Name: "item", Type: StructOf("Item")}, {Name: "items", Type: ListOf(StructOf("Item"))},
			{Name: "byName", Type: MapOf(String, StructOf("Item"))}, {Name: "st", Type: Dyn},
			{Name: "pane", Type: Bool}, {Name: "pane.id", Type: String}, {Name: "n", Type: Int},
		},
		Funcs: []Func{
			{Name: "upper", Overloads: []Overload{{ID: "upper_string", Args: []Type{String}, Result: String,
				Fn: func(a []Value) (Value, error) { return StringValue(strings.ToUpper(a[0].String())), nil }}}},
			{Name: "repeat", Overloads: []Overload{{ID: "string_repeat_int", Member: true, Args: []Type{String, Int}, Result: String,
				Fn: func(a []Value) (Value, error) {
					return StringValue(strings.Repeat(a[0].String(), int(a[1].Int()))), nil
				}}}},
			{Name: "label", Overloads: []Overload{
				{ID: "label_int", Args: []Type{Int}, Result: String,
					Fn: func(a []Value) (Value, error) { return StringValue("int"), nil }},
				{ID: "label_string", Args: []Type{String}, Result: String,
					Fn: func(a []Value) (Value, error) { return StringValue("string"), nil }},
			}},
			{Name: "fail", Overloads: []Overload{{ID: "fail", Args: []Type{}, Result: Bool,
				Fn: func([]Value) (Value, error) { return Value{}, errHelper }}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func testActivation(env *Env, unknown ...string) Activation {
	b := &item{title: "b", count: 5}
	a := &item{title: "a", count: 2, tags: []string{"x", "y"}, next: b}
	return env.Activation(map[string]any{
		"item":   ObjectValue("Item", a),
		"items":  ListValue(items{a, b}),
		"byName": MapValue(valueStrMap{"a": ObjectValue("Item", a)}),
		"st":     ObjectValue("Item", a),
		"pane":   true, "pane.id": "x", "n": 1,
	}, unknown...)
}

func TestEval(t *testing.T) {
	env := testEnv(t)
	act := testActivation(env)
	for _, c := range []struct {
		src, typ, want string
	}{
		{`item.title`, "string", "a"},
		{`item.count + 1`, "int", "3"},
		{`item.tags[1]`, "string", "y"},
		{`'x' in item.tags`, "bool", "true"},
		{`items[1].title`, "string", "b"},
		{`byName['a'].count`, "int", "2"},
		{`byName.a.count`, "int", "2"},
		{`item.next.title`, "string", "b"},
		{`size(items)`, "int", "2"},
		{`item.count > 1 ? 'many' : 'one'`, "string", "many"},
		{`st.title`, "dyn", "a"},
		{`st.count * 2`, "int", "4"},
		{`upper(item.title)`, "string", "A"},
		{`item.title.repeat(3)`, "string", "aaa"},
		{`label(n)`, "string", "int"},
		{`label(dyn(pane.id))`, "string", "string"},
		{`pane && pane.id == 'x'`, "bool", "true"},
		{`[1, 2, 3][n]`, "int", "2"},
		{`{'a': 1, 'a': 2}['a']`, "int", "2"},
		{`size('héllo')`, "int", "5"},
		{`string(1.5e10)`, "string", "1.5e+10"},
		{`string(b'abc')`, "string", "abc"},
		{`1 < dyn(1.5)`, "bool", "true"},
		{`dyn(1) == 1u`, "bool", "true"},
		{`[[], [1]]`, "list(list(int))", "[[] [1]]"},
		{`{'k': [1]}`, "map(string, list(int))", "{k: [1]}"},
	} {
		p, err := env.Compile(c.src)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if got := FormatType(p.Type()); got != c.typ {
			t.Errorf("%s: type %s, want %s", c.src, got, c.typ)
		}
		v, err := p.Eval(act)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if got := show(v); got != c.want {
			t.Errorf("%s = %s, want %s", c.src, got, c.want)
		}
	}
}

// show writes a value for a test's eyes: lists and maps too, which Text
// leaves out.
func show(v Value) string {
	switch v.Kind() {
	case ListKind:
		l := v.List()
		parts := make([]string, l.Len())
		for i := range parts {
			parts[i] = show(l.At(i))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case MapKind:
		var parts []string
		v.Map().Range(func(k, v Value) bool {
			parts = append(parts, show(k)+": "+show(v))
			return true
		})
		return "{" + strings.Join(parts, ", ") + "}"
	case StructKind:
		return v.StructName() + "{}"
	}
	s, err := Text(v)
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

func TestCompileErrors(t *testing.T) {
	env := testEnv(t)
	for _, c := range []struct {
		src  string
		kind ErrorKind
		msg  string
	}{
		{`1 +`, SyntaxError, "1:4: "},
		{"pane &&\n  ", SyntaxError, "2:3: "},
		{`nope`, Undeclared, "undeclared reference to 'nope'"},
		{`item.nope`, TypeError, "undefined field 'nope'"},
		{`item.count + 'a'`, TypeError, "no matching overload for '_+_' applied to '(int, string)'"},
		{`upper(1)`, TypeError, "no matching overload for 'upper'"},
		{`1 < 1.5`, TypeError, "no matching overload for '_<_'"},
		{`timestamp('2020-01-01T00:00:00Z')`, Unsupported, "timestamps"},
		{`'a'.matches('a')`, Unsupported, "regular expressions"},
		{`[1].all(x, x > 0)`, Unsupported, ""},
		{`item == item`, Unsupported, "comparing structs"},
		{`-st['k']`, Unsupported, "inferred"},
		{`int(st['k'])`, Unsupported, "inferred"},
		{`x.?y`, Unsupported, ""},
	} {
		_, err := env.Compile(c.src)
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: %v, want an *Error", c.src, err)
			continue
		}
		if e.Kind != c.kind || !strings.Contains(e.Error(), c.msg) {
			t.Errorf("%s: %v (kind %d), want kind %d with %q", c.src, e, e.Kind, c.kind, c.msg)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	env := testEnv(t)
	act := testActivation(env)
	for _, c := range []struct{ src, msg string }{
		{`n / 0 == 1`, "division by zero"},
		{`n % 0 == 1`, "modulus by zero"},
		{`9223372036854775807 + n`, "overflow"},
		{`items[5].title`, "out of range"},
		{`byName['z'].count`, "no such key"},
		{`upper(st)`, "no such overload"},
		{`label(st)`, "no such overload"},
		{`dyn(item.count) + 'a'`, "no such overload"},
	} {
		p, err := env.Compile(c.src)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		v, err := p.Eval(act)
		var e *EvalError
		if !errors.As(err, &e) || !strings.Contains(e.Error(), c.msg) {
			t.Errorf("%s = %s, %v; want an *EvalError with %q", c.src, show(v), err, c.msg)
		}
	}
	p, err := env.Compile(`fail()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Eval(act); !errors.Is(err, errHelper) {
		t.Errorf("fail() gave %v, want the helper's error", err)
	}
}

func TestUnknown(t *testing.T) {
	env := testEnv(t)
	for _, c := range []struct {
		src             string
		unknown         []string
		holds, isUnknwn bool
	}{
		{`pane && pane.id == 'x'`, nil, true, false},
		{`pane && pane.id == 'x'`, []string{"pane.id"}, false, true},
		{`!pane && pane.id == 'x'`, []string{"pane.id"}, false, false},
		{`pane || pane.id == 'x'`, []string{"pane.id"}, true, false},
		{`n / 0 == 1 && pane.id == 'x'`, []string{"pane.id"}, false, true},
	} {
		p, err := env.Compile(c.src)
		if err != nil {
			t.Fatal(err)
		}
		holds, unknown := p.Test(testActivation(env, c.unknown...))
		if holds != c.holds || unknown != c.isUnknwn {
			t.Errorf("%s with %v unknown: holds %v, unknown %v; want %v, %v", c.src, c.unknown, holds, unknown, c.holds, c.isUnknwn)
		}
	}
	p, _ := env.Compile(`pane.id`)
	if _, err := p.Eval(testActivation(env, "pane.id")); !errors.Is(err, ErrUnknown) {
		t.Errorf("an unknown pane.id gave %v, want ErrUnknown", err)
	}
}

func TestCheckedTree(t *testing.T) {
	env := testEnv(t)
	x, err := Parse(`items[0].count + n`)
	if err != nil {
		t.Fatal(err)
	}
	y, err := env.Check(x)
	if err != nil {
		t.Fatal(err)
	}
	if x.Args[0].Op != OpSelect || x.Args[0].Type.Kind != NullKind || x.Args[1].Op != OpIdent {
		t.Errorf("Check changed the parsed tree")
	}
	sel := y.Args[0]
	if y.Op != OpAdd || sel.Op != OpSelect || sel.Ref != 1 || sel.Type.Kind != IntKind {
		t.Errorf("items[0].count: op %d, ref %d, type %s; want a select of field 1, an int", sel.Op, sel.Ref, FormatType(sel.Type))
	}
	if v := y.Args[1]; v.Op != OpVar || v.Name != "n" {
		t.Errorf("n is op %d %q, want a variable", v.Op, v.Name)
	} else if i, _ := env.VarIndex("n"); v.Ref != i {
		t.Errorf("n's Ref is %d, want %d", v.Ref, i)
	}
	q, err := env.Check(must(Parse(`pane.id`)))
	if err != nil || q.Op != OpVar || q.Name != "pane.id" {
		t.Errorf("pane.id checks as %+v, %v; want the variable pane.id", q, err)
	}
	c, err := env.Check(must(Parse(`upper(item.title)`)))
	if err != nil || c.Overload != "upper_string" {
		t.Errorf("upper(item.title): %v, overload %q", err, c.Overload)
	}
}

func must(x *Expr, err error) *Expr {
	if err != nil {
		panic(err)
	}
	return x
}

func TestText(t *testing.T) {
	for _, c := range []struct {
		v    Value
		want string
	}{
		{IntValue(-3), "-3"}, {UintValue(3), "3"}, {DoubleValue(1.5), "1.5"}, {DoubleValue(1e21), "1e+21"},
		{BoolValue(true), "true"}, {StringValue("é"), "é"}, {BytesValue([]byte("ab")), "ab"},
	} {
		if got, err := Text(c.v); err != nil || got != c.want {
			t.Errorf("Text(%v) = %q, %v; want %q", c.v.Interface(), got, err, c.want)
		}
	}
	// CEL's string() takes neither.
	if _, err := Text(BytesValue([]byte{0xff})); err == nil {
		t.Errorf("Text of invalid UTF-8 bytes gave no error")
	}
	if _, err := Text(NullValue()); err == nil {
		t.Errorf("Text of null gave no error")
	}
}

func TestNewEnvErrors(t *testing.T) {
	for _, d := range []Decls{
		{Vars: []Var{{Name: "x"}}},
		{Vars: []Var{{Name: "x", Type: Int}, {Name: "x", Type: Int}}},
		{Vars: []Var{{Name: "1x", Type: Int}}},
		{Vars: []Var{{Name: "x", Type: StructOf("Nope")}}},
		{Vars: []Var{{Name: "x", Type: MapOf(Double, Int)}}},
		{Structs: []Struct{{Name: "S", Fields: []Field{{Name: "a", Type: Int}, {Name: "a", Type: Int}}}}},
		{Funcs: []Func{{Name: "size", Overloads: []Overload{{ID: "s", Args: []Type{Int}, Result: Int}}}}},
	} {
		if _, err := NewEnv(d); err == nil {
			t.Errorf("NewEnv(%+v) gave no error", d)
		}
	}
}

// TestNames checks that the names CEL keeps are the ones NewEnv refuses,
// and that a variable may take a type's name and win over the type.
func TestNames(t *testing.T) {
	helper := func(name string) Decls {
		return Decls{Funcs: []Func{{Name: name, Overloads: []Overload{{ID: "h", Args: []Type{Int}, Result: Int,
			Fn: func([]Value) (Value, error) { return IntValue(0), nil }}}}}}
	}
	for _, names := range [][]string{ReservedWords(), TypeNames(), StandardFunctions()} {
		if len(names) == 0 || !sort.StringsAreSorted(names) {
			t.Errorf("%q: empty or not sorted", names)
		}
		for _, name := range names {
			if _, err := NewEnv(helper(name)); err == nil {
				t.Errorf("a helper named %q is accepted", name)
			}
		}
	}
	for _, name := range ReservedWords() {
		if _, err := NewEnv(Decls{Vars: []Var{{Name: name, Type: Int}}}); err == nil {
			t.Errorf("a variable named %q is accepted", name)
		}
	}
	for _, name := range TypeNames() {
		env, err := NewEnv(Decls{Vars: []Var{{Name: name, Type: Int}}})
		if err != nil {
			t.Errorf("a variable named %q: %v", name, err)
			continue
		}
		p, err := env.Compile(name + " + 1")
		if err != nil {
			t.Errorf("%s + 1: %v", name, err)
			continue
		}
		if v, err := p.Eval(env.Activation(map[string]any{name: 2})); err != nil || v.Int() != 3 {
			t.Errorf("%s + 1 = %v, %v; want 3", name, v.Interface(), err)
		}
	}
	ReservedWords()[0] = "x"
	if ReservedWords()[0] == "x" {
		t.Error("ReservedWords shares its slice")
	}
}
