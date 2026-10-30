package oracle

import (
	"flag"
	"math/rand/v2"
	"testing"
)

// corpus are expressions picked for their corners: the parser's (signs,
// runs of ! and -, literals, escapes, trailing commas, reserved words),
// the checker's (overloads, dyn, joins, empty literals, null), and the
// evaluator's (overflow, NaN, absent and unknown variables, errors and
// unknowns through && and ||).
var corpus = []string{
	// Literals.
	`1`, `-1`, `- 1`, `--1`, `---1`, `-9223372036854775808`, `--9223372036854775808`, `9223372036854775808`,
	`0x7fffffffffffffff`, `-0x8000000000000000`, `0xffffffffffffffffu`, `18446744073709551616u`, `1u`, `-1u`,
	`1.5`, `-1.5`, `.5`, `1e3`, `1E-3`, `1e400`, `1.e3`, `0x`, `1.5u`,
	`'a'`, `"a"`, `'''a\nb'''`, `"""x"""`, `r'\n'`, `R"\x"`, `b'\xff'`, `b'é'`, `'\xff'`, `'\377'`, `b'\377'`,
	`'é'`, `'\U0001F600'`, `'\ud800'`, `b'é'`, `'\a\b\f\n\r\t\v\\\'\"\?\` + "`" + `'`, `'\q'`,
	`"unterminated`, `'a` + "\n" + `b'`, `'''a` + "\n" + `b'''`, `true`, `false`, `null`,
	// Unary runs.
	`!true`, `!!true`, `!!!true`, `!!1`, `--'a'`, `-'a'`, `!-1`, `!--1`, `-!true`, `-(-1)`, `- -1`,
	`-9223372036854775807 - 1`, `-(-9223372036854775808)`, `-i`, `-(i)`, `-d`, `-u`,
	// Lists and maps.
	`[]`, `[,]`, `[1,]`, `[1, 2][1]`, `[1, 'a']`, `[1, 'a'][1]`, `{}`, `{,}`, `{'a': 1,}`, `{1: 'a', 'b': 2}`,
	`{'a': 1, 'a': 2}`, `{'a': 1, 'a': 2}['a']`, `{1: 'a', 1u: 'b'}`, `{1: 'a'}[1u]`, `{1: 'a'}[1.0]`,
	`{1u: 'a'}[1]`, `{true: 1}[true]`, `{[1]: 2}`, `{1.5: 'a'}`, `[[], [1]]`, `[[1], []]`, `[[1], ['a']]`,
	`[] + [1]`, `[1] + []`, `[1] + ['a']`, `[1] + [dyn('a')]`, `{} == {}`, `[] == []`,
	`[1, 2] == [1, 2]`, `[1] == [1.0]`, `{'a': 1} == {'a': 1}`, `[dyn(1)] == [1u]`, `{'a': dyn(1)} == {'a': 1.0}`,
	`[null]`, `[null, 1]`, `{'a': null}['a'] == null`,
	// Comparisons and equality.
	`1 == 1`, `1 == 1u`, `1 == 1.0`, `dyn(1) == 1u`, `dyn(1) == 1.0`, `dyn(1) == '1'`, `1 < 1.5`, `1u < 1`,
	`-1 < 1u`, `1.5 > 1u`, `'a' < 'b'`, `b'a' < b'b'`, `false < true`, `true < 1`, `dyn(true) < 1`,
	`double('NaN') == double('NaN')`, `double('NaN') < 1.0`, `dyn(double('NaN')) < 'a'`, `1 == null`,
	`null == null`, `dyn(null) == 1`, `9223372036854775807 == 9223372036854775807.0`,
	`dyn(9223372036854775807) == 9223372036854775808.0`, `dyn(18446744073709551615u) == 18446744073709551615.0`,
	`1 < 2 < 3`, `1 < 2 == true`, `(1 < 2) == true`,
	// Arithmetic.
	`9223372036854775807 + 1`, `-9223372036854775808 - 1`, `-9223372036854775808 * -1`, `-9223372036854775808 / -1`,
	`-9223372036854775808 % -1`, `1 / 0`, `1 % 0`, `1u / 0u`, `1u - 2u`, `18446744073709551615u + 1u`,
	`1.0 / 0.0`, `-1.0 / 0.0`, `0.0 / 0.0 == 0.0 / 0.0`, `5 % -3`, `-5 % 3`, `-5 / 3`, `1 + 1u`, `1 + 1.0`,
	`'a' + 'b'`, `b'a' + b'b'`, `'a' + 1`, `dyn(1) + 1u`, `dyn(1) + 1`, `dyn('a') + 'b'`, `dyn([1]) + [2]`,
	// Functions.
	`size('héllo')`, `'héllo'.size()`, `size(b'\xff\xfe')`, `size([1, 2])`, `size({'a': 1})`, `size(1)`, `size(dyn(1))`,
	`'abc'.contains('b')`, `'abc'.startsWith('ab')`, `'abc'.endsWith('bc')`, `contains('a', 'b')`,
	`int(1.9)`, `int(-1.9)`, `int(1e19)`, `int(9223372036854775807.0)`, `int(-9223372036854775808.0)`, `int('12')`,
	`int(' 12')`, `int('0x10')`, `int(18446744073709551615u)`, `int(true)`, `uint(-1)`, `uint(1.5)`, `uint(-0.5)`,
	`uint('7')`, `double('1e3')`, `double('inf')`, `double('-Inf')`, `double('0x1p-2')`, `double('1_0')`,
	`double(18446744073709551615u)`, `string(1.0)`, `string(1e21)`, `string(1e-7)`, `string(-0.0)`, `string(123456789.0)`,
	`string(b'\xff')`, `string(b'abc')`, `string(true)`, `string(18446744073709551615u)`, `string([1])`,
	`bytes('é')`, `bool('true')`, `bool('TRUE')`, `bool('t')`, `bool('1')`, `bool('yes')`, `dyn(1)`,
	`matches('a', 'a')`, `'a'.matches('a')`, `type(1)`, `int`, `timestamp('2020-01-01T00:00:00Z')`,
	`duration('1s')`, `has(m.a)`, `has(m)`, `has(1.a)`, `has(dy.a)`, `has(n.tags.a)`, `has(m.a, m.b)`,
	`[1].all(x, x > 0)`, `[1].exists(x, x > 0)`, `[1].map(x, x)`, `[1].map(x, true, x)`, `[1].filter(x, x > 0)`,
	`[1].exists_one(x, x > 0)`, `[1].all(x)`, `size`, `f(1)`, `1.f()`,
	// Ternary.
	`true ? 1 : 2`, `true ? 1 : 'a'`, `true ? 1 : dyn('a')`, `true ? dyn(1) : 2`, `true ? null : 1`,
	`true ? [] : [1]`, `true ? [1] : []`, `1 ? 2 : 3`, `dyn(1) ? 2 : 3`, `true ? 1 : 2 ? 3 : 4`,
	`true ? true ? 1 : 2 : 3`, `false ? 1 / 0 : 2`,
	// Variables, qualified names, absent and unknown values.
	`n`, `n.id`, `n.id == 'x'`, `n && n.id == 'x'`, `n.tags['a']`, `n.tags.a`, `'a' in n.tags`, `n.x`, `n.id.x`,
	`.n`, `.n.id`, `x`, `m.a + 1`, `md.a`, `md.a.b`, `dy.a`, `dy[0]`, `dy['a']`, `l[0]`, `l[-1]`, `l[1u]`,
	`l[1.0]`, `l[dyn(1.5)]`, `l['a']`, `l[dyn(1u)]`, `l[dyn(0.0)]`, `ms.k`, `ms['k']`, `m[1]`,
	`b && c`, `b || c`, `!b`, `b && 1 / 0 == 1`, `1 / 0 == 1 && b`, `false && 1 / 0 == 1`, `1 / 0 == 1 || true`,
	`dyn(1) && true`, `dyn(1) && false`, `dyn(1) || true`, `1 / 0 == 1 && dyn(1)`,
	`i + j`, `i in l`, `s in ls`, `s in m`, `y == b''`, `d > 0.0`, `u > 0u`,
	// Values read with [] from a dyn or an empty literal: cel-go types
	// them with a parameter, which the first overload binds.
	`dy[s]`, `[dy[s], 'x']`, `-dy[s]`, `dy[s] + dy[s]`, `dy[s] + dy`, `dy[s] - dy`, `b ? dy[s] : 1`, `b ? dy[s] : dy`,
	`-dy[0]`, `-dy[1u]`, `-(b ? dy[s] : dy)`, `[[], [dy[s]]]`, `[dy[s]] + [1]`, `-dy[s][0]`, `-dy[s].a`, `-dy[dy[s]]`,
	`-{}[s]`, `-[][0]`, `-{}.a`, `{dy[s]: 1}`, `b ? [] : [dy[s]]`, `-(b ? dy[s] : dy[s])`, `[dy[s]][0] + 1`,
	`dy[s] in l`, `-dy[true]`, `[dy[s], dy][0] + 1.5`, `-[dy[s], dy][0]`, `[1, dy[s]]`, `{'a': dy[s], 'b': 1}`,
	`-dy[s] + 1`, `dy[s] + 1`, `{}[s] == 1`, `int(dy[s])`, `string(dy[s])`, `int(dyn(dy[s]))`, `string(dyn(dy[s]))`,
	`size(dy[s])`, `dy[s].size()`, `dy[s].contains('a')`, `dy[s] < 2`, `dy[s] == 'x'`, `-dyn(dy[s])`,
	`[dyn(dy[s]), 'x']`, `md[s]`, `-md[s]`, `-md.a`, `dy[null]`, `{}[null]`, `dy[b'a']`, `dy[dyn(null)]`, `l[null]`,
	// Reserved words and syntax.
	`if`, `n.if`, `in`, `null.x`, `true.x`, `x.true`, `a b`, `(`, `)`, `1 +`, `1 = 1`, `1 & 1`, `[1`, `{1: }`,
	`a.b{}`, `Foo{a: 1}`, `x.?y`, `x[?0]`, `[?1]`, `{?'a': 1}`, "`a`", "x.`a`", `f(1,)`, `f(,)`, `#`,
	`// comment` + "\n" + `1`, `1 // trailing`, "\t1\r\n+\f1", `1.f`, `x.5`, `0x1.5`,
}

func TestOracleCorpus(t *testing.T) {
	w := newWorld(t, genDecls)
	g := &gen{r: rand.New(rand.NewPCG(1, 2))}
	bs := []binding{g.binding(), g.binding(), g.binding(), g.binding(), {vals: map[string]any{}}}
	for _, src := range corpus {
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("%s\n\t%s", src, diff)
		}
	}
}

var generated = flag.Int("generated", 3000, "how many generated expressions TestOracleGenerated compares")

// TestOracleGenerated compares a fixed set of generated expressions, each
// on four random bindings.
func TestOracleGenerated(t *testing.T) {
	w := newWorld(t, genDecls)
	failures := 0
	for n := range *generated {
		g := &gen{r: rand.New(rand.NewPCG(uint64(n), 7))}
		src := g.expr(gBool, 4)
		if n%3 == 1 {
			src = g.expr(allTypes[g.r.IntN(len(allTypes))], 4)
		}
		bs := []binding{g.binding(), g.binding(), g.binding(), g.binding()}
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("seed %d: %s\n\t%s", n, src, diff)
			if failures++; failures > 20 {
				t.Fatal("too many differences")
			}
		}
	}
}

// FuzzGenerated compares an expression generated from the fuzzer's bytes.
func FuzzGenerated(f *testing.F) {
	for n := range 20 {
		f.Add([]byte(seedString(n)))
	}
	w := newWorld(f, genDecls)
	f.Fuzz(func(t *testing.T, data []byte) {
		g := seedGen(data)
		src := g.expr(allTypes[g.r.IntN(len(allTypes))], 5)
		bs := []binding{g.binding(), g.binding(), g.binding()}
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("%s\n\t%s", src, diff)
		}
	})
}

// FuzzSource compares whatever source the fuzzer writes, from the corpus.
func FuzzSource(f *testing.F) {
	for _, src := range corpus {
		f.Add(src)
	}
	w := newWorld(f, genDecls)
	g := &gen{r: rand.New(rand.NewPCG(3, 4))}
	bs := []binding{g.binding(), g.binding(), {vals: map[string]any{}}}
	f.Fuzz(func(t *testing.T, src string) {
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("%q\n\t%s", src, diff)
		}
	})
}
