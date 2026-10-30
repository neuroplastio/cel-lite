package cellite

import (
	"strings"
	"testing"
)

// worstEnv declares what the worst cases name.
func worstEnv(t testing.TB) *Env {
	t.Helper()
	env, err := NewEnv(Decls{Vars: []Var{{Name: "x", Type: Bool}, {Name: "m", Type: Dyn}}})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func worstActivation(env *Env) Activation {
	return env.Activation(map[string]any{"x": true, "m": map[string]any{"a": "a"}})
}

// families are the shapes that make an expression deep, each built n
// levels deep: nested brackets, calls and conditionals (maxNest) and long
// chains of selections, indexes and operators (MaxDepth).
var families = []struct {
	name  string
	build func(n int) string
}{
	{"parens", func(n int) string { return rep("(", n) + "x" + rep(")", n) }},
	{"lists", func(n int) string { return rep("[", n) + "x" + rep("]", n) }},
	{"maps", func(n int) string { return rep("{'a': ", n) + "x" + rep("}", n) }},
	{"calls", func(n int) string { return rep("dyn(", n) + "x" + rep(")", n) }},
	{"indexes", func(n int) string { return rep("m[", n) + "'a'" + rep("]", n) }},
	{"ternaries", func(n int) string { return rep("x ? x : ", n) + "x" }},
	{"negations", func(n int) string { return rep("-(", n) + "1" + rep(")", n) }},
	{"nots", func(n int) string { return rep("!(", n) + "x" + rep(")", n) }},
	{"ands", func(n int) string { return rep("x && (", n) + "x" + rep(")", n) }},
	{"ins", func(n int) string { return rep("x in [", n) + "x" + rep("]", n) }},
	{"selects", func(n int) string { return "m" + rep(".a", n) }},
	{"has", func(n int) string { return "has(m" + rep(".a", n) + ")" }},
	{"index chain", func(n int) string { return "m" + rep("['a']", n) }},
	{"sums", func(n int) string { return "1" + rep(" + 1", n) }},
	{"equalities", func(n int) string { return "x" + rep(" == x", n) }},
	// Each level climbs every precedence, a parser frame apiece.
	{"climbs", func(n int) string { return rep("x || x && m == m + m * dyn(", n) + "x" + rep(")", n) }},
}

func rep(s string, n int) string { return strings.Repeat(s, n) }

// worstCase is a family's expression at exactly the limits: the deepest
// that Parse takes, one level short of what it refuses.
type worstCase struct {
	name, src string
	n         int
}

// worstCases builds each family at the limits, and fails the test if
// Parse takes one level more.
func worstCases(t testing.TB) []worstCase {
	t.Helper()
	var ws []worstCase
	for _, f := range families {
		n := 0
		for ; n <= 2*MaxDepth; n++ {
			if _, err := Parse(f.build(n + 1)); err != nil {
				break
			}
		}
		if n == 0 || n > 2*MaxDepth {
			t.Fatalf("%s: Parse took %d levels", f.name, n)
		}
		ws = append(ws, worstCase{f.name, f.build(n), n})
	}
	return ws
}

// TestLimits compiles and runs each family at the limits, and feeds
// expressions far past them, which are errors. Nothing may panic or
// overflow a stack: the gate runs it under TinyGo's wasm with a smaller
// stack than the 64 KB TinyGo gives a goroutine.
func TestLimits(t *testing.T) {
	env := worstEnv(t)
	for _, w := range worstCases(t) {
		t.Run(w.name, func(t *testing.T) {
			p, err := env.Compile(w.src)
			if err != nil {
				t.Fatalf("at %d levels: %v", w.n, err)
			}
			p.Eval(worstActivation(env))
		})
	}
	for _, c := range []struct {
		name, src string
		ok        bool
	}{
		{"parens past", rep("(", 5000) + "x" + rep(")", 5000), false},
		{"lists past", rep("[", 5000), false},
		{"calls past", rep("size(", 5000), false},
		{"selects past", "m" + rep(".a", 5000), false},
		{"ternaries past", rep("x ? x : ", 5000) + "x", false},
		{"sums past", rep("1 + ", 5000) + "1", false},
		{"nots", rep("!", 50000) + "x", true},
		{"negations", rep("-", 50001) + "1", true},
		{"ands", rep("x && ", 15000) + "x", true},
		{"ors", rep("x || ", 15000) + "x", true},
		{"too large", rep(" ", MaxSize) + "x", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := env.Compile(c.src)
			if (err == nil) != c.ok {
				t.Fatalf("compile error %v, want ok %v", err, c.ok)
			}
			if err == nil {
				p.Eval(worstActivation(env))
			}
		})
	}
}

// TestCheckDepth hands Check and Program a tree deeper than Parse makes.
func TestCheckDepth(t *testing.T) {
	env := worstEnv(t)
	x := &Expr{Op: OpIdent, Name: "x"}
	for range 500 {
		x = &Expr{Op: OpNot, Args: []*Expr{x}}
	}
	if _, err := env.Check(x); err == nil {
		t.Errorf("Check took a tree 500 deep")
	}
	y := &Expr{Op: OpVar, Name: "x", Type: Bool}
	for range 500 {
		y = &Expr{Op: OpNot, Type: Bool, Args: []*Expr{y}}
	}
	if _, err := env.Program(y); err == nil {
		t.Errorf("Program took a tree 500 deep")
	}
}
