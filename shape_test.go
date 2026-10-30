package cellite

import (
	"math/rand/v2"
	"testing"
)

// TestTrees hands Check and Program trees a program might build: random
// ones, well formed or not, and checked ones with a reference changed.
// Neither panics, and Eval doesn't on what they take.
func TestTrees(t *testing.T) {
	env := testEnv(t)
	r := rand.New(rand.NewPCG(7, 8))
	// Not the helper repeat, which panics itself on a negative count.
	names := []string{"item", "items", "byName", "st", "pane", "pane.id", "n", "title", "count", "tags",
		"next", "size", "contains", "int", "upper", "label", "fail", "nope", ""}
	vals := []any{nil, true, int64(3), uint64(2), 1.5, "a", []byte("b"), struct{}{}}
	var gen func(d int) *Expr
	gen = func(d int) *Expr {
		if d == 0 || r.IntN(4) == 0 {
			switch r.IntN(8) {
			case 0:
				return nil
			case 1, 2:
				return &Expr{Op: OpLit, Value: vals[r.IntN(len(vals))]}
			case 3:
				return &Expr{Op: OpVar, Name: names[r.IntN(len(names))], Ref: r.IntN(8) - 2}
			}
			return &Expr{Op: OpIdent, Name: names[r.IntN(len(names))]}
		}
		x := &Expr{Op: Op(r.IntN(int(OpMap) + 2)), Name: names[r.IntN(len(names))], Member: r.IntN(3) == 0,
			Ref: r.IntN(8) - 2}
		n := r.IntN(5)
		if a := int(arity[min(int(x.Op), len(arity)-1)]); a >= 0 && r.IntN(4) != 0 {
			n = a
		}
		for range n {
			x.Args = append(x.Args, gen(d-1))
		}
		return x
	}
	act := testActivation(env)
	refused, planned := 0, 0
	for range 20000 {
		x := gen(4)
		y, err := env.Check(x)
		if err != nil {
			refused++
			continue
		}
		for range 4 {
			mutate(r, y, names)
			if p, err := env.Program(y); err == nil {
				planned++
				p.Eval(act)
			}
		}
	}
	if _, err := env.Check(nil); err == nil {
		t.Error("Check(nil) gave no error")
	}
	if _, err := env.Program(nil); err == nil {
		t.Error("Program(nil) gave no error")
	}
	t.Logf("%d refused by Check, %d planned after a change", refused, planned)
}

// mutate changes a node of a checked tree: its Ref, its Overload, its
// Name or its Type.
func mutate(r *rand.Rand, x *Expr, names []string) {
	for len(x.Args) > 0 && r.IntN(3) != 0 {
		x = x.Args[r.IntN(len(x.Args))]
	}
	switch r.IntN(4) {
	case 0:
		x.Ref = r.IntN(12) - 2
	case 1:
		x.Overload = []string{"", "nope", "upper_string", "size_string"}[r.IntN(4)]
	case 2:
		x.Name = names[r.IntN(len(names))]
	default:
		x.Type = []Type{Dyn, Int, StructOf("Item"), StructOf("Nope"), ListOf(StructOf("Item"))}[r.IntN(5)]
	}
}
