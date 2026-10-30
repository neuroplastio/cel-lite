package oracle

import (
	"strings"
	"testing"

	"github.com/neuroplastio/cel-lite"
)

// Untyped data can give a variable a value of another kind than declared:
// a string decoded as an int, a list where a map was meant. CEL keeps the
// overload Check chose, so such a value is an error where an operator or
// a function meets it, and cellite must say what cel-go says.

var mistypedDecls = []decl{
	{"s", cellite.String}, {"i", cellite.Int}, {"u", cellite.Uint}, {"d", cellite.Double},
	{"b", cellite.Bool}, {"y", cellite.Bytes}, {"l", cellite.ListOf(cellite.Int)},
	{"m", cellite.MapOf(cellite.String, cellite.Int)},
}

// mistypedValues are a value of each kind, as untyped data gives them.
var mistypedValues = []any{
	"1", int64(1), uint64(1), 1.0, true, []byte("1"), []any{int64(1)}, map[string]any{"a": int64(1)}, nil,
}

var mistypedCorpus = []string{
	`int(s)`, `uint(s)`, `double(s)`, `string(s)`, `bytes(s)`, `bool(s)`, `size(s)`, `s.size()`,
	`s.contains('1')`, `s.startsWith('1')`, `s.endsWith('1')`, `s + 'a'`, `s == '1'`, `s != '1'`,
	`s < '2'`, `s in ['1']`, `s in {'1': 1}`, `{'1': 1}[s]`, `s in m`, `m[s]`, `dyn(s) == 1`,
	`int(i)`, `uint(i)`, `double(i)`, `string(i)`, `i + 1`, `-i`, `i * 2`, `i / 1`, `i % 2`,
	`i == 1`, `i < 2`, `i in [1]`, `[1, 2][i]`, `{1: 'a'}[i]`, `i in {1: 'a'}`, `dyn(i) == '1'`,
	`int(u)`, `double(u)`, `string(u)`, `u + 1u`, `u == 1u`, `u < 2u`, `[1, 2][u]`, `u in {1u: 'a'}`,
	`int(d)`, `uint(d)`, `string(d)`, `d + 1.0`, `-d`, `d == 1.0`, `d < 2.0`, `d in [1.0]`,
	`!b`, `b && true`, `b || false`, `b ? 1 : 2`, `string(b)`, `bool(b)`, `b == true`, `b in {true: 1}`,
	`string(y)`, `bytes(y)`, `size(y)`, `y + b'a'`, `y == b'1'`, `y < b'2'`, `y in [b'1']`,
	`size(l)`, `l.size()`, `l[0]`, `1 in l`, `l + [2]`, `l == [1]`,
	`size(m)`, `m['a']`, `'a' in m`, `m.a`, `has(m.a)`, `m == {'a': 1}`,
}

// mistypedExcluded is why a case differs from cel-go by design, as doc.go
// lists it, or "".
func mistypedExcluded(src string, v any) string {
	if _, ok := v.([]byte); ok && strings.Contains(src, " in {") {
		return "in on a map with a bytes key: false, where cel-go's map literal fails to hash it"
	}
	return ""
}

// TestOracleMistyped compares the corpus on bindings where every variable
// has its declared kind but one, which has each kind in turn.
func TestOracleMistyped(t *testing.T) {
	w := newWorld(t, mistypedDecls)
	typed := map[string]any{
		"s": "1", "i": int64(1), "u": uint64(1), "d": 1.0, "b": true, "y": []byte("1"),
		"l": []any{int64(1)}, "m": map[string]any{"a": int64(1)},
	}
	for _, src := range mistypedCorpus {
		if diff := w.compare(src, []binding{{vals: typed}}); diff != "" {
			t.Errorf("%s\n\t%s", src, diff)
		}
		for _, d := range mistypedDecls {
			for _, v := range mistypedValues {
				if mistypedExcluded(src, v) != "" {
					continue
				}
				b := binding{vals: map[string]any{}}
				for k, x := range typed {
					b.vals[k] = x
				}
				b.vals[d.name] = v
				if diff := w.compare(src, []binding{b}); diff != "" {
					t.Errorf("%s\n\t%s", src, diff)
				}
			}
		}
	}
}
