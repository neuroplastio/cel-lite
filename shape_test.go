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
	act := testActivation(env)
	r := rand.New(rand.NewPCG(7, 8))
	refused, planned := 0, 0
	for range 20000 {
		x := tree(r, 4)
		if p, err := env.Program(x); err == nil {
			p.Eval(act)
		}
		y, err := env.Check(x)
		if err != nil {
			refused++
			continue
		}
		for range 4 {
			mutate(r, y)
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

// FuzzTrees is TestTrees on a tree the fuzzer's bytes build, one choice
// a byte, deep enough to pass the limits.
func FuzzTrees(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 3, 2, 0, 1, 1, 9, 4})
	f.Add([]byte{1, 21, 1, 1, 2, 1, 5, 2, 3, 0, 1})
	f.Add([]byte{1, 14, 3, 1, 0, 0, 3, 2, 3, 2, 3, 3, 0, 2, 1, 4, 7})
	env := testEnv(f)
	act := testActivation(env)
	f.Fuzz(func(t *testing.T, data []byte) {
		c := choices(data)
		x := tree(&c, MaxDepth+2)
		if p, err := env.Program(x); err == nil {
			p.Eval(act)
		}
		y, err := env.Check(x)
		if err != nil {
			return
		}
		if p, err := env.Program(y); err == nil {
			p.Eval(act)
		}
		for len(c) > 0 {
			mutate(&c, y)
			if p, err := env.Program(y); err == nil {
				p.Eval(act)
			}
		}
	})
}

// chooser picks a number below n: a *rand.Rand, or choices.
type chooser interface{ IntN(n int) int }

// choices are a fuzzer's bytes, one choice each, then zeros.
type choices []byte

func (c *choices) IntN(n int) int {
	if len(*c) == 0 {
		return 0
	}
	k := int((*c)[0]) % n
	*c = (*c)[1:]
	return k
}

// treeNames are the names a built tree takes: the test Env's variables,
// fields and helpers, standard functions, and names it hasn't. Not the
// helper repeat, which panics itself on a negative count.
var treeNames = []string{"item", "items", "byName", "st", "pane", "pane.id", "n", "title", "count", "tags",
	"next", "size", "contains", "int", "upper", "label", "fail", "nope", ""}

// tree builds a tree at most d deep, as a program might, well formed or
// not: any Op, any operands, any Name and Ref.
func tree(c chooser, d int) *Expr {
	if d == 0 || c.IntN(4) == 0 {
		switch c.IntN(8) {
		case 0:
			return nil
		case 1, 2:
			vals := []any{nil, true, int64(3), uint64(2), 1.5, "a", []byte("b"), struct{}{}}
			return &Expr{Op: OpLit, Value: vals[c.IntN(len(vals))]}
		case 3:
			return &Expr{Op: OpVar, Name: treeNames[c.IntN(len(treeNames))], Ref: c.IntN(8) - 2}
		}
		return &Expr{Op: OpIdent, Name: treeNames[c.IntN(len(treeNames))]}
	}
	x := &Expr{Op: Op(c.IntN(int(OpMap) + 2)), Name: treeNames[c.IntN(len(treeNames))], Member: c.IntN(3) == 0,
		Ref: c.IntN(8) - 2}
	n := c.IntN(5)
	if a := int(arity[min(int(x.Op), len(arity)-1)]); a >= 0 && c.IntN(4) != 0 {
		n = a
	}
	for range n {
		x.Args = append(x.Args, tree(c, d-1))
	}
	return x
}

// mutate changes a node of a checked tree: its Ref, its Overload, its
// Name or its Type.
func mutate(c chooser, x *Expr) {
	for len(x.Args) > 0 && c.IntN(3) != 0 {
		x = x.Args[c.IntN(len(x.Args))]
	}
	switch c.IntN(4) {
	case 0:
		x.Ref = c.IntN(12) - 2
	case 1:
		x.Overload = []string{"", "nope", "upper_string", "size_string"}[c.IntN(4)]
	case 2:
		x.Name = treeNames[c.IntN(len(treeNames))]
	default:
		x.Type = []Type{Dyn, Int, StructOf("Item"), StructOf("Nope"), ListOf(StructOf("Item"))}[c.IntN(5)]
	}
}
