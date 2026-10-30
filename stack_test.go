//go:build cellitestack && tinygo

package cellite

import (
	"runtime"
	"testing"
)

// stackBudget is the most stack the parser, the checker, the planner or
// the evaluator may use within the limits, under TinyGo: a quarter of a
// goroutine's 64 KB, which leaves the rest to the program around them.
const stackBudget = 16 << 10

// TestStackUse measures the stack the parser, the checker, the planner
// and the evaluator need for each worst case, under TinyGo, whose
// goroutine stacks are fixed (`make stack`), and fails past stackBudget.
// The probes see the stack down to their own frames only, so the gate
// also runs every test on a 32 KB stack.
func TestStackUse(t *testing.T) {
	if runtime.Compiler != "tinygo" {
		t.Skip("Go moves its stacks; this measures TinyGo's")
	}
	env := worstEnv(t)
	most := 0
	for _, w := range worstCases(t) {
		base := stackHere()
		used := func() int { return int(base - stackLow) }

		stackLow = 0
		x, err := Parse(w.src)
		if err != nil {
			t.Fatal(err)
		}
		parse := used()
		stackLow = 0
		y, err := env.Check(x)
		if err != nil {
			t.Fatal(err)
		}
		check := used()
		stackLow = 0
		p, err := env.Program(y)
		if err != nil {
			t.Fatal(err)
		}
		plan := used()
		stackLow = 0
		p.Eval(worstActivation(env))
		eval := used()

		t.Logf("%-12s %3d levels: parse %6d  check %6d  plan %6d  eval %6d bytes", w.name, w.n, parse, check, plan, eval)
		most = max(most, parse, check, plan, eval)
	}
	t.Logf("most: %d bytes, of a budget of %d", most, stackBudget)
	if most > stackBudget {
		t.Errorf("the worst case within the limits uses %d bytes of stack, past the budget of %d", most, stackBudget)
	}
}
