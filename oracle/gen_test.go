package oracle

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/neuroplastio/cel-lite"
)

// The world the generated expressions run in: a variable of each type,
// and a node with qualified attributes, as a keymap's path declares them.
var genDecls = []decl{
	{"b", cellite.Bool}, {"c", cellite.Bool},
	{"i", cellite.Int}, {"j", cellite.Int},
	{"u", cellite.Uint}, {"d", cellite.Double},
	{"s", cellite.String}, {"t", cellite.String},
	{"y", cellite.Bytes},
	{"l", cellite.ListOf(cellite.Int)}, {"ls", cellite.ListOf(cellite.String)},
	{"m", cellite.MapOf(cellite.String, cellite.Int)},
	{"ms", cellite.MapOf(cellite.String, cellite.String)},
	{"md", cellite.MapOf(cellite.String, cellite.Dyn)},
	{"dy", cellite.Dyn},
	{"n", cellite.Bool}, {"n.id", cellite.String}, {"n.tags", cellite.MapOf(cellite.String, cellite.String)},
}

// gtype is a type the generator makes expressions of.
type gtype string

const (
	gBool   gtype = "bool"
	gInt    gtype = "int"
	gUint   gtype = "uint"
	gDouble gtype = "double"
	gString gtype = "string"
	gBytes  gtype = "bytes"
	gNull   gtype = "null"
	gDyn    gtype = "dyn"
	gListI  gtype = "list(int)"
	gListS  gtype = "list(string)"
	gMapSI  gtype = "map(string, int)"
	gMapSS  gtype = "map(string, string)"
	gMapSD  gtype = "map(string, dyn)"
	gMapIS  gtype = "map(int, string)"
)

var allTypes = []gtype{gBool, gInt, gUint, gDouble, gString, gBytes, gNull, gDyn, gListI, gListS, gMapSI, gMapSS, gMapSD, gMapIS}

// gen makes random expressions, mostly well typed, sometimes not.
type gen struct{ r *rand.Rand }

func (g *gen) pick(xs ...string) string { return xs[g.r.IntN(len(xs))] }
func (g *gen) chance(p float64) bool    { return g.r.Float64() < p }

// vars are the variables of each type.
var vars = map[gtype][]string{
	gBool: {"b", "c", "n"}, gInt: {"i", "j"}, gUint: {"u"}, gDouble: {"d"},
	gString: {"s", "t", "n.id"}, gBytes: {"y"}, gDyn: {"dy"},
	gListI: {"l"}, gListS: {"ls"}, gMapSI: {"m"}, gMapSS: {"ms", "n.tags"}, gMapSD: {"md"},
}

func (g *gen) expr(t gtype, d int) string {
	if g.chance(0.03) {
		t = allTypes[g.r.IntN(len(allTypes))]
	}
	if d <= 0 || g.chance(0.25) {
		return g.leaf(t)
	}
	d--
	cond := func() string { return g.expr(gBool, d) + " ? " + g.expr(t, d) + " : " + g.expr(t, d) }
	paren := func(s string) string { return "(" + s + ")" }
	switch t {
	case gBool:
		switch g.r.IntN(15) {
		case 0:
			return "!" + paren(g.expr(gBool, d))
		case 1:
			return paren(g.expr(gBool, d)) + " && " + paren(g.expr(gBool, d))
		case 2:
			return paren(g.expr(gBool, d)) + " || " + paren(g.expr(gBool, d))
		case 3, 4:
			a, b := g.comparable()
			return paren(g.expr(a, d)) + " " + g.pick("==", "!=", "<", "<=", ">", ">=") + " " + paren(g.expr(b, d))
		case 5:
			return paren(g.expr(gInt, d)) + " in " + paren(g.expr(gListI, d))
		case 6:
			return paren(g.expr(gString, d)) + " in " + paren(g.expr(g.mapOrList(), d))
		case 7:
			return "has(" + g.pick("m", "ms", "md", "n.tags") + "." + g.key() + ")"
		case 8:
			return paren(g.expr(gString, d)) + "." + g.pick("contains", "startsWith", "endsWith") + "(" + g.expr(gString, d) + ")"
		case 9:
			return cond()
		case 10:
			return "bool(" + g.expr(gString, d) + ")"
		case 11:
			x := g.pick(string(gListI), string(gMapSS), string(gDyn), string(gNull))
			return paren(g.expr(gtype(x), d)) + " " + g.pick("==", "!=") + " " + paren(g.expr(gtype(x), d))
		case 12:
			return "dyn(" + g.expr(allTypes[g.r.IntN(len(allTypes))], d) + ") == " + paren(g.expr(allTypes[g.r.IntN(len(allTypes))], d))
		case 13:
			return g.pick("md", "dy") + "." + g.key() + " == " + paren(g.expr(gDyn, d))
		default:
			return "!!" + paren(g.expr(gBool, d))
		}
	case gInt, gUint, gDouble:
		ops := []string{"+", "-", "*", "/", "%"}
		if t == gDouble {
			ops = ops[:4]
		}
		switch g.r.IntN(9) {
		case 0, 1:
			return paren(g.expr(t, d)) + " " + g.pick(ops...) + " " + paren(g.expr(t, d))
		case 2:
			if t != gUint {
				return "-" + paren(g.expr(t, d))
			}
		case 3:
			if t == gInt {
				return "size(" + g.expr(gtype(g.pick(string(gString), string(gBytes), string(gListI), string(gMapSS))), d) + ")"
			}
		case 4:
			from := g.pick(string(gInt), string(gUint), string(gDouble), string(gString))
			return string(t) + "(" + g.expr(gtype(from), d) + ")"
		case 5:
			return cond()
		case 6:
			if t == gInt {
				return paren(g.expr(gListI, d)) + "[" + g.expr(gInt, d) + "]"
			}
		case 7:
			if t == gInt {
				return paren(g.expr(gMapSI, d)) + "[" + g.expr(gString, d) + "]"
			}
		case 8:
			if t == gInt {
				return g.pick("m", "md") + "." + g.key()
			}
		}
		return g.leaf(t)
	case gString:
		switch g.r.IntN(7) {
		case 0, 1:
			return paren(g.expr(gString, d)) + " + " + paren(g.expr(gString, d))
		case 2:
			from := g.pick(string(gInt), string(gUint), string(gDouble), string(gBool), string(gBytes), string(gString))
			return "string(" + g.expr(gtype(from), d) + ")"
		case 3:
			return cond()
		case 4:
			return paren(g.expr(gListS, d)) + "[" + g.expr(gInt, d) + "]"
		case 5:
			return paren(g.expr(gMapSS, d)) + "[" + g.expr(gString, d) + "]"
		default:
			return g.pick("ms", "n.tags") + "." + g.key()
		}
	case gBytes:
		switch g.r.IntN(3) {
		case 0:
			return paren(g.expr(gBytes, d)) + " + " + paren(g.expr(gBytes, d))
		case 1:
			return "bytes(" + g.expr(gString, d) + ")"
		}
		return cond()
	case gListI, gListS:
		e := gInt
		if t == gListS {
			e = gString
		}
		switch g.r.IntN(4) {
		case 0:
			return "[" + g.expr(e, d) + ", " + g.expr(e, d) + "]"
		case 1:
			return paren(g.expr(t, d)) + " + " + paren(g.expr(t, d))
		case 2:
			return cond()
		}
		return "[]"
	case gMapSI, gMapSS, gMapSD, gMapIS:
		k, v := gString, gInt
		switch t {
		case gMapSS:
			v = gString
		case gMapSD:
			v = gDyn
		case gMapIS:
			k, v = gInt, gString
		}
		switch g.r.IntN(3) {
		case 0:
			return "{" + g.expr(k, d) + ": " + g.expr(v, d) + ", " + g.expr(k, d) + ": " + g.expr(v, d) + "}"
		case 1:
			return cond()
		}
		return "{}"
	case gDyn:
		switch g.r.IntN(4) {
		case 0:
			return "dyn(" + g.expr(allTypes[g.r.IntN(len(allTypes))], d) + ")"
		case 1:
			return "md[" + g.expr(gString, d) + "]"
		case 2:
			return "dy." + g.key()
		}
		return paren(g.expr(gDyn, d)) + " " + g.pick("+", "-", "*", "/", "%", "[0]") + " " + paren(g.expr(allTypes[g.r.IntN(len(allTypes))], d))
	}
	return g.leaf(t)
}

func (g *gen) mapOrList() gtype {
	return gtype(g.pick(string(gMapSI), string(gMapSS), string(gListS), string(gMapSD)))
}

// comparable is a pair of operand types for a relation, mostly ones that
// compare.
func (g *gen) comparable() (gtype, gtype) {
	nums := []gtype{gInt, gUint, gDouble}
	switch g.r.IntN(5) {
	case 0, 1:
		return nums[g.r.IntN(3)], nums[g.r.IntN(3)]
	case 2:
		return gString, gString
	case 3:
		x := gtype(g.pick(string(gBool), string(gBytes), string(gDyn)))
		return x, x
	}
	return allTypes[g.r.IntN(len(allTypes))], allTypes[g.r.IntN(len(allTypes))]
}

func (g *gen) key() string { return g.pick("a", "b", "k", "id") }

// leaf is a literal or a variable of type t.
func (g *gen) leaf(t gtype) string {
	if vs := vars[t]; len(vs) > 0 && g.chance(0.5) {
		return vs[g.r.IntN(len(vs))]
	}
	switch t {
	case gBool:
		return g.pick("true", "false")
	case gInt:
		return g.pick("0", "1", "-1", "2", "7", "-3", "9223372036854775807", "-9223372036854775808", "0x10", "100")
	case gUint:
		return g.pick("0u", "1u", "2u", "18446744073709551615u", "0x10u", "7u")
	case gDouble:
		return g.pick("0.0", "1.5", "-2.5", "1e308", "1e-300", ".5", "2.0", "-0.0", "3e2")
	case gString:
		return g.pick(`'a'`, `"b"`, `'k'`, `''`, `'id'`, `'héllo'`, `'\x41'`, `"é"`, `r'\n'`, `'''x'''`, `'1'`, `'-5'`, `'true'`, `'1.5'`, `'NaN'`)
	case gBytes:
		return g.pick(`b'a'`, `b''`, `b'\xff'`, `b"\101"`, `B'k'`)
	case gNull:
		return "null"
	case gDyn:
		return "dyn(" + g.leaf(allTypes[g.r.IntN(6)]) + ")"
	case gListI:
		return g.pick("[1, 2]", "[]", "[0]", "[-1, 7, 2]")
	case gListS:
		return g.pick("['a', 'b']", "[]", "['k']")
	case gMapSI:
		return g.pick("{'a': 1}", "{}", "{'k': 2, 'b': -1}")
	case gMapSS:
		return g.pick("{'a': 'x'}", "{}", "{'k': 'b'}")
	case gMapSD:
		return g.pick("{'a': 1, 'b': 'x'}", "{}", "{'k': [1]}")
	case gMapIS:
		return g.pick("{1: 'a'}", "{}", "{0: 'k', 2: 'b'}")
	}
	return "null"
}

// binding is a random activation of genDecls: each variable present with
// a value of its type (dy with a value of any), absent, or unknown.
func (g *gen) binding() binding {
	b := binding{vals: map[string]any{}}
	for _, d := range genDecls {
		switch x := g.r.Float64(); {
		case x < 0.07:
			continue
		case x < 0.15:
			b.unknown = append(b.unknown, d.name)
			continue
		}
		b.vals[d.name] = g.value(d.typ)
	}
	return b
}

func (g *gen) value(t cellite.Type) any {
	switch t.Kind {
	case cellite.BoolKind:
		return g.chance(0.5)
	case cellite.IntKind:
		return []int64{0, 1, -1, 2, 7, math.MaxInt64, math.MinInt64}[g.r.IntN(7)]
	case cellite.UintKind:
		return []uint64{0, 1, 2, 7, math.MaxUint64}[g.r.IntN(5)]
	case cellite.DoubleKind:
		return []float64{0, 1.5, -2.5, math.Inf(1), math.NaN(), 1e308, 2}[g.r.IntN(7)]
	case cellite.StringKind:
		return []string{"", "a", "b", "k", "héllo", "1", "id"}[g.r.IntN(7)]
	case cellite.BytesKind:
		return []byte(g.pick("", "a", "\xff"))
	case cellite.ListKind:
		out := []any{}
		for range g.r.IntN(3) {
			out = append(out, g.value(*t.Elem))
		}
		return out
	case cellite.MapKind:
		out := map[string]any{}
		for range g.r.IntN(3) {
			out[g.key()] = g.value(*t.Elem)
		}
		return out
	}
	// dyn: any type's value.
	return g.value([]cellite.Type{cellite.Bool, cellite.Int, cellite.Uint, cellite.Double, cellite.String,
		cellite.ListOf(cellite.Int), cellite.MapOf(cellite.String, cellite.String)}[g.r.IntN(7)])
}

// seedGen is a generator seeded from fuzz input.
func seedGen(data []byte) *gen {
	var s1, s2 uint64
	for i, c := range data {
		if i%2 == 0 {
			s1 = s1*131 + uint64(c)
		} else {
			s2 = s2*137 + uint64(c)
		}
	}
	return &gen{r: rand.New(rand.NewPCG(s1, s2))}
}

func seedString(n int) string { return strconv.Itoa(n) + strings.Repeat("x", n%7) }
