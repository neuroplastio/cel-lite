package cellite

import "testing"

func TestSmoke(t *testing.T) {
	env, err := NewEnv(Decls{Vars: []Var{
		{Name: "focus", Type: String}, {Name: "pane", Type: Bool}, {Name: "pane.id", Type: String},
		{Name: "pane.annotations", Type: MapOf(String, String)}, {Name: "n", Type: Int},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		`pane && pane.id == 'x'`, `'a.b' in pane.annotations`, `pane.annotations["a.b"] == "y"`,
		`n + 1 > 2 ? 'big' : 'small'`, `[1, 2, 3][n]`, `size("héllo")`, `--'a'`, `!!n`, `-9223372036854775808`,
		`{'a': 1, 'a': 2}['a']`, `1 < dyn(1.5)`, `dyn(1) == 1u`, `string(1.5e10)`, `[1] + []`, `[[], [1]]`,
	} {
		p, err := env.Compile(src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		v, err := p.Eval(env.Activation(map[string]any{"pane": true, "pane.id": "x", "n": 1,
			"pane.annotations": map[string]string{"a.b": "y"}}))
		t.Logf("%s : %s = %v (%v)", src, FormatType(p.Type()), v.Interface(), err)
	}
}
