package oracle

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"

	"github.com/neuroplastio/cel-lite"
)

// decl is a variable declared to both engines.
type decl struct {
	name string
	typ  cellite.Type
}

// world is a set of declarations, read by both engines.
type world struct {
	decls []decl
	lite  *cellite.Env
	cel   *cel.Env
}

func newWorld(t testing.TB, decls []decl) *world {
	t.Helper()
	w := &world{decls: decls}
	var vars []cellite.Var
	var opts []cel.EnvOption
	for _, d := range decls {
		vars = append(vars, cellite.Var{Name: d.name, Type: d.typ})
		opts = append(opts, cel.Variable(d.name, celType(d.typ)))
	}
	var err error
	if w.lite, err = cellite.NewEnv(cellite.Decls{Vars: vars}); err != nil {
		t.Fatal(err)
	}
	if w.cel, err = cel.NewEnv(opts...); err != nil {
		t.Fatal(err)
	}
	return w
}

// celType is a cellite type as cel-go declares it.
func celType(t cellite.Type) *cel.Type {
	switch t.Kind {
	case cellite.BoolKind:
		return cel.BoolType
	case cellite.IntKind:
		return cel.IntType
	case cellite.UintKind:
		return cel.UintType
	case cellite.DoubleKind:
		return cel.DoubleType
	case cellite.StringKind:
		return cel.StringType
	case cellite.BytesKind:
		return cel.BytesType
	case cellite.NullKind:
		return cel.NullType
	case cellite.ListKind:
		return cel.ListType(celType(*t.Elem))
	case cellite.MapKind:
		return cel.MapType(celType(*t.Key), celType(*t.Elem))
	}
	return cel.DynType
}

// binding is one activation: the variables' values as Go has them (both
// engines read the same), the ones left out absent, and those named
// unknown.
type binding struct {
	vals    map[string]any
	unknown []string
}

// outcome is what an engine made of an expression on a binding.
type outcome struct {
	// compileErr is why it was refused; refused marks cellite's
	// Unsupported, and limit its depth limits.
	compileErr string
	refused    bool
	limit      bool
	typ        string
	// kind is "value", "error" or "unknown"; val the value, canonical.
	kind string
	val  any
	msg  string
}

func (o outcome) String() string {
	switch {
	case o.compileErr != "":
		return "refused: " + o.compileErr
	case o.kind == "value":
		return fmt.Sprintf("%s %v", o.typ, o.val)
	}
	return o.typ + " " + o.kind + " " + o.msg
}

// liteCompile and celCompile compile src; the outcome holds the error or
// the type.
func (w *world) liteCompile(src string) (*cellite.Program, outcome) {
	p, err := w.lite.Compile(src)
	if err != nil {
		e := err.(*cellite.Error)
		return nil, outcome{compileErr: err.Error(), refused: e.Kind == cellite.Unsupported,
			limit: strings.Contains(e.Msg, "deeper than") || strings.Contains(e.Msg, "larger than")}
	}
	return p, outcome{typ: cellite.FormatType(p.Type())}
}

func (w *world) celCompile(src string) (prg cel.Program, o outcome) {
	defer func() {
		if r := recover(); r != nil {
			o = outcome{compileErr: fmt.Sprintf("cel-go panicked: %v", r)}
		}
	}()
	ast, iss := w.cel.Compile(src)
	if iss.Err() != nil {
		return nil, outcome{compileErr: iss.Err().Error()}
	}
	prg, err := w.cel.Program(ast, cel.EvalOptions(cel.OptPartialEval))
	if err != nil {
		return nil, outcome{compileErr: err.Error()}
	}
	return prg, outcome{typ: ast.OutputType().String()}
}

func (w *world) liteEval(p *cellite.Program, b binding) outcome {
	v, err := p.Eval(w.lite.Activation(b.vals, b.unknown...))
	switch {
	case err == cellite.ErrUnknown:
		return outcome{kind: "unknown"}
	case err != nil:
		return outcome{kind: "error", msg: err.Error()}
	}
	return outcome{kind: "value", val: canonLite(v)}
}

func (w *world) celEval(prg cel.Program, b binding) (o outcome) {
	defer func() {
		if r := recover(); r != nil {
			o = outcome{kind: "panic", msg: fmt.Sprint(r)}
		}
	}()
	var pats []*cel.AttributePatternType
	for _, u := range b.unknown {
		pats = append(pats, cel.AttributePattern(u))
	}
	vars, err := cel.PartialVars(b.vals, pats...)
	if err != nil {
		return outcome{kind: "panic", msg: err.Error()}
	}
	out, _, err := prg.Eval(vars)
	switch {
	case out != nil && types.IsUnknown(out):
		return outcome{kind: "unknown"}
	case err != nil:
		return outcome{kind: "error", msg: err.Error()}
	case types.IsError(out):
		return outcome{kind: "error", msg: fmt.Sprint(out)}
	}
	return outcome{kind: "value", val: canonCel(out)}
}

// compare runs src in both engines on each binding and reports how they
// differ, or "" when they agree. A cellite refusal by name agrees with
// anything, and so does one of its depth limits. So does its run-time
// "invalid map key type": a map literal whose dyn key turns out a double,
// a list or a map. The CEL spec allows no such key; cel-go builds the
// map, where no lookup finds a NaN key, or a list.
func (w *world) compare(src string, bs []binding) string {
	lp, lo := w.liteCompile(src)
	cp, co := w.celCompile(src)
	if strings.HasPrefix(co.compileErr, "cel-go panicked") {
		return ""
	}
	switch {
	case lo.refused || lo.limit:
		return ""
	case lo.compileErr != "" && co.compileErr == "":
		return fmt.Sprintf("cellite refuses what cel-go compiles (%s): %s", co.typ, lo.compileErr)
	case lo.compileErr == "" && co.compileErr != "":
		return fmt.Sprintf("cellite compiles (%s) what cel-go refuses: %s", lo.typ, co.compileErr)
	case lo.compileErr != "":
		return ""
	case lo.typ != co.typ:
		return fmt.Sprintf("types differ: cellite %s, cel-go %s", lo.typ, co.typ)
	}
	for _, b := range bs {
		l, c := w.liteEval(lp, b), w.celEval(cp, b)
		if c.kind == "panic" || l.kind == "error" && strings.HasPrefix(l.msg, "invalid map key type") {
			continue
		}
		if l.kind != c.kind || l.kind == "value" && !reflect.DeepEqual(l.val, c.val) {
			return fmt.Sprintf("on %s: cellite %s, cel-go %s", formatBinding(b), l, c)
		}
	}
	return ""
}

func formatBinding(b binding) string {
	var parts []string
	for k, v := range b.vals {
		parts = append(parts, fmt.Sprintf("%s=%#v", k, v))
	}
	sort.Strings(parts)
	if len(b.unknown) > 0 {
		parts = append(parts, "unknown="+strings.Join(b.unknown, ","))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// canonLite and canonCel write a value so that equal values of the two
// engines are reflect.DeepEqual: scalars as tagged strings, lists as
// []any, maps as map[string]any keyed by their keys' tagged strings.
func canonLite(v cellite.Value) any {
	switch v.Kind() {
	case cellite.NullKind:
		return "null"
	case cellite.BoolKind:
		return "bool:" + strconv.FormatBool(v.Bool())
	case cellite.IntKind:
		return "int:" + strconv.FormatInt(v.Int(), 10)
	case cellite.UintKind:
		return "uint:" + strconv.FormatUint(v.Uint(), 10)
	case cellite.DoubleKind:
		return canonDouble(v.Double())
	case cellite.StringKind:
		return "string:" + v.String()
	case cellite.BytesKind:
		return "bytes:" + string(v.Bytes())
	case cellite.ListKind:
		l := v.List()
		out := make([]any, l.Len())
		for i := range out {
			out[i] = canonLite(l.At(i))
		}
		return out
	case cellite.MapKind:
		out := map[string]any{}
		v.Map().Range(func(k, v cellite.Value) bool {
			out[fmt.Sprint(canonLite(k))] = canonLite(v)
			return true
		})
		return out
	}
	return "struct:" + v.StructName()
}

func canonDouble(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "double:-0"
	}
	return "double:" + strconv.FormatFloat(f, 'g', -1, 64)
}

func canonCel(v ref.Val) any {
	switch v := v.(type) {
	case types.Null:
		return "null"
	case types.Bool:
		return "bool:" + strconv.FormatBool(bool(v))
	case types.Int:
		return "int:" + strconv.FormatInt(int64(v), 10)
	case types.Uint:
		return "uint:" + strconv.FormatUint(uint64(v), 10)
	case types.Double:
		return canonDouble(float64(v))
	case types.String:
		return "string:" + string(v)
	case types.Bytes:
		return "bytes:" + string(v)
	case traits.Mapper:
		out := map[string]any{}
		it := v.Iterator()
		for it.HasNext() == types.True {
			k := it.Next()
			out[fmt.Sprint(canonCel(k))] = canonCel(v.Get(k))
		}
		return out
	case traits.Lister:
		n := int(v.Size().(types.Int))
		out := make([]any, n)
		for i := range out {
			out[i] = canonCel(v.Get(types.Int(i)))
		}
		return out
	}
	return fmt.Sprintf("other:%T", v)
}
