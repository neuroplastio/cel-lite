package cellite

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Program is a compiled expression. It is immutable and safe for
// concurrent use.
type Program struct {
	env  *Env
	expr *Expr
	fn   evalFn
}

// evalFn evaluates a node. Its result is a value, an error (errKind) or
// unknown (unknownKind).
type evalFn func(a Activation) Value

// Compile parses, checks and plans an expression. Its errors are
// *Error, with Line and Col set.
func (e *Env) Compile(src string) (*Program, error) {
	x, err := Parse(src)
	if err != nil {
		return nil, err
	}
	y, err := e.Check(x)
	if err != nil {
		ce := err.(*Error)
		return nil, newError(validUTF8(src), ce.Pos, ce.Kind, ce.Name, ce.Msg)
	}
	return e.Program(y)
}

// Program plans a checked expression, the tree Check returned in this
// Env.
func (e *Env) Program(x *Expr) (*Program, error) {
	if deepNode(x, 1) != nil {
		return nil, &Error{Kind: SyntaxError, Pos: x.Pos, Msg: "expression is nested deeper than " + strconv.Itoa(MaxDepth)}
	}
	c := &compiler{env: e, size: treeSize(x)}
	fn := c.compile(x)
	if c.err != nil {
		return nil, c.err
	}
	return &Program{env: e, expr: x, fn: fn}, nil
}

// Expr is the checked expression.
func (p *Program) Expr() *Expr { return p.expr }

// Type is the expression's type.
func (p *Program) Type() Type { return p.expr.Type }

// Eval runs the program on the activation's variables. It gives the
// value, or an *EvalError, or ErrUnknown when the value depends on a
// variable the activation calls unknown.
func (p *Program) Eval(a Activation) (Value, error) {
	v := p.fn(a)
	switch v.k {
	case errKind:
		return Value{}, errorOf(v)
	case unknownKind:
		return Value{}, ErrUnknown
	}
	return v, nil
}

// Test runs a program whose type is bool: whether it holds (it is true),
// and whether it is unknown. An error is neither; so is a value that
// isn't a bool. It doesn't allocate.
func (p *Program) Test(a Activation) (holds, unknown bool) {
	v := p.fn(a)
	return v.k == BoolKind && v.n != 0, v.k == unknownKind
}

// compiler plans a checked tree into closures.
type compiler struct {
	env *Env
	err error
	// fns holds the nodes' operand closures, in a chunk the size of the
	// tree, made when a node first has operands.
	fns  []evalFn
	size int
}

func (c *compiler) compile(x *Expr) evalFn {
	probe()
	if c.err != nil {
		return nil
	}
	if (x.Op == OpSelect || x.Op == OpIndex) && root(x).Op == OpCond {
		at := c.attr(x)
		if c.err != nil {
			return nil
		}
		return attrValue(at)
	}
	args := c.operands(len(x.Args))
	for i, a := range x.Args {
		if args[i] = c.compile(a); c.err != nil {
			return nil
		}
	}
	if constant(x) {
		return fold(c.build(x, args))
	}
	return c.build(x, args)
}

// operands is room for k operands' closures.
//
//go:noinline
func (c *compiler) operands(k int) []evalFn {
	if k == 0 {
		return nil
	}
	if len(c.fns)+k > cap(c.fns) {
		c.fns = make([]evalFn, 0, max(c.size, k))
	}
	i := len(c.fns)
	c.fns = c.fns[:i+k]
	return c.fns[i : i+k : i+k]
}

// root is the operand a chain of selections and indexes starts from. A
// chain from a variable is cel-go's absolute attribute and one from a
// computed value a relative one (see attrFn), known when the chain is
// planned; one from a conditional is planned as an attrFn, which says at
// run time.
func root(x *Expr) *Expr {
	for x.Op == OpSelect || x.Op == OpIndex {
		x = x.Args[0]
	}
	return x
}

// constant reports whether x is a list or map literal of literals alone,
// whose value is the same on every evaluation.
func constant(x *Expr) bool {
	if x.Op != OpList && x.Op != OpMap || len(x.Args) == 0 {
		return false
	}
	for _, a := range x.Args {
		if a.Op != OpLit {
			return false
		}
	}
	return true
}

// fold evaluates a constant once, when it is planned: lists and maps
// are never changed once built.
//
//go:noinline
func fold(fn evalFn) evalFn {
	v := fn(nil)
	return func(Activation) Value { return v }
}

// The planner recurses through compile and attr alone, which keep their
// frames small, and builds each node's closure in a function apart, not
// inlined, after its operands': a TinyGo goroutine's stack is 64 KB, and
// a function's frame has room for all its cases' temporaries.

// attrValue is an attribute's value alone.
//
//go:noinline
func attrValue(at attrFn) evalFn {
	return func(a Activation) Value {
		v, _ := at(a)
		return v
	}
}

// build makes x's closure from its operands'.
//
//go:noinline
func (c *compiler) build(x *Expr, args []evalFn) evalFn {
	switch x.Op {
	case OpLit:
		v := literal(x.Value)
		return func(Activation) Value {
			probe()
			return v
		}
	case OpVar:
		i, name := x.Ref, x.Name
		return func(a Activation) Value {
			probe()
			v, p := a.Var(i)
			if p != Present {
				return varState(p, name)
			}
			return v
		}
	case OpSelect:
		return c.selectFn(x, args[0])
	case OpIndex:
		return indexFn(args[0], args[1], root(x).Op == OpVar)
	case OpCond:
		return condFn(args[0], args[1], args[2])
	case OpHas:
		op, name := args[0], x.Name
		return func(a Activation) Value {
			o := op(a)
			if o.k >= errKind {
				return o
			}
			// As cel-go's presence test: a map holds the key or not, a
			// list can't be asked, and any other value holds nothing.
			switch o.k {
			case MapKind:
				_, ok := mapGet(o.o.(Map), StringValue(name))
				return BoolValue(ok)
			case ListKind:
				return errValue(eMessage, "unsupported index type 'string' in list")
			case StructKind:
				return errValue(eMessage, "has() on a struct's field is not supported")
			}
			return BoolValue(false)
		}
	case OpCall:
		return c.callFn(x, args)
	case OpAnd:
		return logic(args[0], args[1], false)
	case OpOr:
		return logic(args[0], args[1], true)
	case OpNot:
		return notFn(x, args[0])
	case OpNeg:
		op := args[0]
		return func(a Activation) Value {
			v := op(a)
			switch v.k {
			case IntKind:
				if int64(v.n) == math.MinInt64 {
					return errValue(eIntOverflow, "")
				}
				return IntValue(-int64(v.n))
			case DoubleKind:
				return DoubleValue(-math.Float64frombits(v.n))
			case errKind, unknownKind:
				return v
			}
			return errValue(eNoOverload, "-_")
		}
	case OpEq, OpNe:
		return equality(x, args, x.Op == OpNe)
	case OpLt:
		return binary(x, args, func(l, r Value) Value { return relate(l, r, "_<_", func(c int) bool { return c < 0 }) })
	case OpLe:
		return binary(x, args, func(l, r Value) Value { return relate(l, r, "_<=_", func(c int) bool { return c <= 0 }) })
	case OpGt:
		return binary(x, args, func(l, r Value) Value { return relate(l, r, "_>_", func(c int) bool { return c > 0 }) })
	case OpGe:
		return binary(x, args, func(l, r Value) Value { return relate(l, r, "_>=_", func(c int) bool { return c >= 0 }) })
	case OpIn:
		return binary(x, args, in)
	case OpAdd:
		return binary(x, args, add)
	case OpSub:
		return binary(x, args, sub)
	case OpMul:
		return binary(x, args, mul)
	case OpDiv:
		return binary(x, args, div)
	case OpMod:
		return binary(x, args, mod)
	case OpList:
		return func(a Activation) Value {
			out := make(valueList, len(args))
			var unk Value
			for i, f := range args {
				v := f(a)
				if v.k == errKind {
					return v
				}
				if v.k == unknownKind && unk.k != unknownKind {
					unk = v
				}
				out[i] = v
			}
			if unk.k == unknownKind {
				return unk
			}
			return Value{k: ListKind, o: out}
		}
	case OpMap:
		// As cel-go: the first error in source order, else the first
		// unknown. A key the CEL spec doesn't allow (a dyn that turned
		// out a double or a list) is an error after both; cel-go builds
		// such a map, where a NaN key is never found and two of them are
		// two entries.
		return func(a Activation) Value {
			m := newValueMap(len(args) / 2)
			var unk, bad Value
			for i := 0; i < len(args); i += 2 {
				k := args[i](a)
				if k.k == errKind {
					return k
				}
				v := args[i+1](a)
				if v.k == errKind {
					return v
				}
				for _, x := range [2]Value{k, v} {
					if x.k == unknownKind && unk.k != unknownKind {
						unk = x
					}
				}
				if unk.k == unknownKind || bad.k == errKind {
					continue
				}
				switch k.k {
				case BoolKind, IntKind, UintKind, StringKind:
					m.put(k, v)
				default:
					bad = errValue(eMessage, "invalid map key type: "+kindNames[k.k])
				}
			}
			switch {
			case unk.k == unknownKind:
				return unk
			case bad.k == errKind:
				return bad
			}
			return Value{k: MapKind, o: m}
		}
	case OpIdent:
		c.err = &Error{Kind: Undeclared, Pos: x.Pos, Name: x.Name, Msg: "the expression is not checked: '" + x.Name + "' is unresolved"}
		return nil
	}
	c.err = &Error{Kind: SyntaxError, Pos: x.Pos, Msg: "not an expression node"}
	return nil
}

// literal is a literal's value as a Value.
func literal(v any) Value {
	switch v := v.(type) {
	case []byte:
		return Value{k: BytesKind, s: string(v)}
	}
	return ValueOf(v)
}

// A Value is 48 bytes, more than gc keeps in registers: each Value a
// call returns is copied through the stack, and the copy stalls (its
// loads span the stores that spilled it). So the closures on the hot
// paths read a variable themselves and build their result rather than
// pass on one they were given, each a layer fewer.

// varState is the value of a variable that isn't present: unknown, or
// an error when it is absent.
func varState(p Presence, name string) Value {
	if p == Unknown {
		return Value{k: unknownKind, s: name}
	}
	return errValue(eNoAttr, name)
}

// notFn is !x, reading a variable itself rather than through its
// closure: `!pane.zoomed` is a keymap's commonest context.
//
//go:noinline
func notFn(x *Expr, op evalFn) evalFn {
	if v := x.Args[0]; v.Op == OpVar {
		i, name := v.Ref, v.Name
		return func(a Activation) Value {
			probe()
			v, p := a.Var(i)
			switch {
			case p != Present:
				return varState(p, name)
			case v.k != BoolKind:
				return errValue(eNoOverload, "!_")
			}
			return BoolValue(v.n == 0)
		}
	}
	return func(a Activation) Value { return not(op(a)) }
}

func not(v Value) Value {
	switch {
	case v.k >= errKind:
		return v
	case v.k != BoolKind:
		return errValue(eNoOverload, "!_")
	}
	return BoolValue(v.n == 0)
}

// condFn is cond ? t : f, as a value.
//
//go:noinline
func condFn(cond, t, f evalFn) evalFn {
	return func(a Activation) Value {
		v := cond(a)
		switch {
		case v.k >= errKind:
			return v
		case v.k != BoolKind:
			return errValue(eNoOverload, "_?_:_")
		case v.n != 0:
			return t(a)
		}
		return f(a)
	}
}

// binary evaluates a strict operator, as cel-go's evalBinary: an error on
// the left is returned without evaluating the right; then an error on
// the right; then either side unknown; then the operator. An operand
// that is a constant value is held, not evaluated.
//
//go:noinline
func binary(x *Expr, args []evalFn, op func(l, r Value) Value) evalFn {
	l, r := args[0], args[1]
	if rv, ok := constValue(x.Args[1], r); ok {
		return func(a Activation) Value {
			lv := l(a)
			if lv.k >= errKind {
				return lv
			}
			return op(lv, rv)
		}
	}
	if lv, ok := constValue(x.Args[0], l); ok {
		return func(a Activation) Value {
			rv := r(a)
			if rv.k >= errKind {
				return rv
			}
			return op(lv, rv)
		}
	}
	return func(a Activation) Value {
		lv := l(a)
		if lv.k == errKind {
			return lv
		}
		rv := r(a)
		switch {
		case rv.k == errKind:
			return rv
		case lv.k == unknownKind:
			return lv
		case rv.k == unknownKind:
			return rv
		}
		return op(lv, rv)
	}
}

// equality is == (or, ne, !=): binary's evaluation, with equal called
// in the closure rather than through an operator's.
//
//go:noinline
func equality(x *Expr, args []evalFn, ne bool) evalFn {
	l, r := args[0], args[1]
	if rv, ok := constValue(x.Args[1], r); ok {
		return func(a Activation) Value {
			lv := l(a)
			if lv.k >= errKind {
				return lv
			}
			return BoolValue(equal(lv, rv) != ne)
		}
	}
	if lv, ok := constValue(x.Args[0], l); ok {
		return func(a Activation) Value {
			rv := r(a)
			if rv.k >= errKind {
				return rv
			}
			return BoolValue(equal(lv, rv) != ne)
		}
	}
	return func(a Activation) Value {
		lv := l(a)
		if lv.k == errKind {
			return lv
		}
		rv := r(a)
		switch {
		case rv.k == errKind:
			return rv
		case lv.k == unknownKind:
			return lv
		case rv.k == unknownKind:
			return rv
		}
		return BoolValue(equal(lv, rv) != ne)
	}
}

// constValue is x's value, planned as fn, when it is the same on every
// evaluation, a literal or a list or map of literals, and isn't an error.
func constValue(x *Expr, fn evalFn) (Value, bool) {
	if x.Op != OpLit && !constant(x) {
		return Value{}, false
	}
	v := fn(nil)
	return v, v.k < errKind
}

// logic is && (or false) or ||: either side's false (true) decides,
// whatever the other gives; else unknown wins over an error, and an error
// (or a value that isn't a bool) over the other side's true (false).
func logic(l, r evalFn, or bool) evalFn {
	return func(a Activation) Value {
		lv := l(a)
		if lv.k == BoolKind && (lv.n != 0) == or {
			return BoolValue(or)
		}
		rv := r(a)
		if rv.k == BoolKind && (rv.n != 0) == or {
			return BoolValue(or)
		}
		switch {
		case lv.k == unknownKind:
			return lv
		case rv.k == unknownKind:
			return rv
		case lv.k == errKind:
			return lv
		case rv.k == errKind:
			return rv
		case lv.k != BoolKind || rv.k != BoolKind:
			if or {
				return errValue(eNoOverload, "_||_")
			}
			return errValue(eNoOverload, "_&&_")
		}
		return BoolValue(!or)
	}
}

// attrFn evaluates a node that is a variable, a selection, an index or a
// conditional, and says whether its value was read through a variable:
// cel-go's absolute attribute, `v.f[i]`, against a relative one,
// `f(x)[i]`. They differ in what an index does with an unknown: indexing
// a variable that is unknown evaluates the index first, so an error there
// wins; indexing a computed value that is unknown doesn't evaluate it. A
// conditional is whichever branch it takes.
type attrFn func(a Activation) (Value, bool)

// attr plans x as an attrFn.
func (c *compiler) attr(x *Expr) attrFn {
	probe()
	if c.err != nil {
		return nil
	}
	switch x.Op {
	case OpVar:
		return absolute(c.compile(x))
	case OpSelect:
		return c.selectAttr(x, c.attr(x.Args[0]))
	case OpIndex:
		return indexAttr(c.attr(x.Args[0]), c.compile(x.Args[1]))
	case OpCond:
		return condAttr(c.compile(x.Args[0]), c.attr(x.Args[1]), c.attr(x.Args[2]))
	}
	return relative(c.compile(x))
}

// absolute is a variable as an attribute.
//
//go:noinline
func absolute(v evalFn) attrFn {
	return func(a Activation) (Value, bool) { return v(a), true }
}

// relative is a computed value as an attribute.
//
//go:noinline
func relative(v evalFn) attrFn {
	return func(a Activation) (Value, bool) { return v(a), false }
}

// indexAttr is op[ix].
//
//go:noinline
func indexAttr(op attrFn, ix evalFn) attrFn {
	if op == nil || ix == nil {
		return nil
	}
	return func(a Activation) (Value, bool) {
		o, abs := op(a)
		return qualify(a, o, abs, ix), abs
	}
}

// indexFn is op[ix], op read through a variable or not (abs).
//
//go:noinline
func indexFn(op, ix evalFn, abs bool) evalFn {
	return func(a Activation) Value { return qualify(a, op(a), abs, ix) }
}

// qualify is o[ix]: ix isn't evaluated when o is an error, or an unknown
// not read through a variable.
func qualify(a Activation, o Value, abs bool, ix evalFn) Value {
	if o.k == errKind || o.k == unknownKind && !abs {
		return o
	}
	i := ix(a)
	switch {
	case i.k == errKind:
		return i
	case o.k == unknownKind:
		// cel-go makes the index a qualifier first, even of a
		// variable that is unknown.
		switch i.k {
		case StringKind, IntKind, UintKind, BoolKind, DoubleKind, unknownKind:
			return o
		}
		return errValue(eMessage, "invalid qualifier type: "+kindNames[i.k])
	case i.k == unknownKind:
		return i
	}
	return index(o, i)
}

// condAttr is cond ? t : f: whichever branch it takes.
//
//go:noinline
func condAttr(cond evalFn, t, f attrFn) attrFn {
	if cond == nil || t == nil || f == nil {
		return nil
	}
	return func(a Activation) (Value, bool) {
		v := cond(a)
		switch {
		case v.k >= errKind:
			return v, false
		case v.k != BoolKind:
			return errValue(eNoOverload, "_?_:_"), false
		case v.n != 0:
			return t(a)
		}
		return f(a)
	}
}

// selection is a planned selection, .name: a struct's field by index
// when the operand is typed as that struct, else by name at run time.
type selection struct {
	env   *Env
	name  string
	key   Value
	sname string
	i     int
}

func (c *compiler) selection(x *Expr) *selection {
	s := &selection{env: c.env, name: x.Name, key: StringValue(x.Name)}
	if t := x.Args[0].Type; t.Kind == StructKind {
		s.sname, s.i = t.Name, x.Ref
	}
	return s
}

// of is o.name.
func (s *selection) of(o Value) Value {
	if o.k == StructKind && o.s == s.sname && s.sname != "" {
		return o.o.(Object).Field(s.i)
	}
	if o.k >= errKind {
		return o
	}
	return selectValue(s.env, o, s.name, s.key)
}

// selectFn is op.name, reading a variable itself rather than through
// its closure: `row.name` is a template's commonest binding.
//
//go:noinline
func (c *compiler) selectFn(x *Expr, op evalFn) evalFn {
	s := c.selection(x)
	if v := x.Args[0]; v.Op == OpVar {
		i, name := v.Ref, v.Name
		return func(a Activation) Value {
			probe()
			o, p := a.Var(i)
			switch {
			case p != Present:
				return varState(p, name)
			case o.k == StructKind && o.s == s.sname && s.sname != "":
				return o.o.(Object).Field(s.i)
			}
			return s.of(o)
		}
	}
	return func(a Activation) Value { return s.of(op(a)) }
}

// selectAttr is selectFn as an attribute.
//
//go:noinline
func (c *compiler) selectAttr(x *Expr, op attrFn) attrFn {
	if c.err != nil || op == nil {
		return nil
	}
	s := c.selection(x)
	return func(a Activation) (Value, bool) {
		o, abs := op(a)
		return s.of(o), abs
	}
}

// selectValue is o.name for an o whose type is known only at run time.
//
//go:noinline
func selectValue(env *Env, o Value, name string, key Value) Value {
	switch o.k {
	case MapKind:
		if v, ok := mapGet(o.o.(Map), key); ok {
			return v
		}
		return errValue(eNoKey, strconv.Quote(name))
	case StructKind:
		if s := env.structs[o.s]; s != nil {
			if i, ok := s.fields[name]; ok {
				return o.o.(Object).Field(i)
			}
		}
		return errValue(eNoField, name)
	}
	return errValue(eMessage, "type '"+kindNames[o.k]+"' does not support field selection")
}

// index is c[i]: a list's element at an int (or a uint or a double that
// is a whole number), or a map's value at a key.
//
//go:noinline
func index(c, i Value) Value {
	switch c.k {
	case ListKind:
		l := c.o.(List)
		var n int64
		switch i.k {
		case IntKind:
			n = int64(i.n)
		case UintKind:
			if i.n > math.MaxInt64 {
				return errValue(eMessage, "unsupported index value")
			}
			n = int64(i.n)
		case DoubleKind:
			var ok bool
			if n, ok = doubleToIntLossless(math.Float64frombits(i.n)); !ok {
				return errValue(eMessage, "unsupported index value")
			}
		default:
			return errValue(eMessage, "unsupported index type '"+kindNames[i.k]+"' in list")
		}
		if size := int64(l.Len()); n < 0 || n >= size {
			return errValue(eMessage, "index '"+strconv.FormatInt(n, 10)+"' out of range in list size '"+strconv.FormatInt(size, 10)+"'")
		}
		return l.At(int(n))
	case MapKind:
		switch i.k {
		case BoolKind, IntKind, UintKind, DoubleKind, StringKind:
		default:
			return errValue(eMessage, "unsupported key type: "+kindNames[i.k])
		}
		if v, ok := mapGet(c.o.(Map), i); ok {
			return v
		}
		return errValue(eNoKey, formatKey(i))
	}
	return errValue(eNoOverload, "_[_]")
}

// formatKey writes a map key for an error.
//
//go:noinline
func formatKey(k Value) string {
	switch k.k {
	case StringKind:
		return strconv.Quote(k.s)
	case IntKind:
		return strconv.FormatInt(int64(k.n), 10)
	case UintKind:
		return strconv.FormatUint(k.n, 10) + "u"
	case DoubleKind:
		return strconv.FormatFloat(math.Float64frombits(k.n), 'g', -1, 64)
	case BoolKind:
		return strconv.FormatBool(k.n != 0)
	}
	return "<" + kindNames[k.k] + ">"
}

// equal is CEL's ==: numbers compare across int, uint and double, NaN
// equals nothing, lists and maps compare their elements, and values of
// different kinds are not equal.
//
//go:noinline
func equal(l, r Value) bool {
	switch l.k {
	case NullKind:
		return r.k == NullKind
	case BoolKind:
		return r.k == BoolKind && l.n == r.n
	case StringKind:
		return r.k == StringKind && l.s == r.s
	case BytesKind:
		return r.k == BytesKind && l.s == r.s
	case IntKind, UintKind, DoubleKind:
		switch r.k {
		case IntKind, UintKind, DoubleKind:
			c, ok := compareNum(l, r)
			return ok && c == 0
		}
		return false
	case ListKind:
		if r.k != ListKind {
			return false
		}
		a, b := l.o.(List), r.o.(List)
		n := a.Len()
		if b.Len() != n {
			return false
		}
		for i := 0; i < n; i++ {
			if !equal(a.At(i), b.At(i)) {
				return false
			}
		}
		return true
	case MapKind:
		if r.k != MapKind {
			return false
		}
		a, b := l.o.(Map), r.o.(Map)
		if a.Len() != b.Len() {
			return false
		}
		eq := true
		a.Range(func(k, v Value) bool {
			w, ok := mapGet(b, k)
			eq = ok && equal(v, w)
			return eq
		})
		return eq
	}
	return false
}

// compareNum compares two numbers exactly, across kinds. It is not ok
// when either is NaN.
func compareNum(l, r Value) (int, bool) {
	switch l.k {
	case IntKind:
		i := int64(l.n)
		switch r.k {
		case IntKind:
			return cmp3(i < int64(r.n), i > int64(r.n)), true
		case UintKind:
			return compareIntUint(i, r.n), true
		case DoubleKind:
			d := math.Float64frombits(r.n)
			if math.IsNaN(d) {
				return 0, false
			}
			return -compareDoubleInt(d, i), true
		}
	case UintKind:
		u := l.n
		switch r.k {
		case UintKind:
			return cmp3(u < r.n, u > r.n), true
		case IntKind:
			return -compareIntUint(int64(r.n), u), true
		case DoubleKind:
			d := math.Float64frombits(r.n)
			if math.IsNaN(d) {
				return 0, false
			}
			return -compareDoubleUint(d, u), true
		}
	case DoubleKind:
		d := math.Float64frombits(l.n)
		if math.IsNaN(d) {
			return 0, false
		}
		switch r.k {
		case DoubleKind:
			e := math.Float64frombits(r.n)
			if math.IsNaN(e) {
				return 0, false
			}
			return cmp3(d < e, d > e), true
		case IntKind:
			return compareDoubleInt(d, int64(r.n)), true
		case UintKind:
			return compareDoubleUint(d, r.n), true
		}
	}
	return 0, false
}

func cmp3(less, more bool) int {
	switch {
	case less:
		return -1
	case more:
		return 1
	}
	return 0
}

// compareDoubleInt and the others are cel-go's, so that the corner cases
// agree.
func compareDoubleInt(d float64, i int64) int {
	if d < math.MinInt64 {
		return -1
	}
	if d > math.MaxInt64 {
		return 1
	}
	f := float64(i)
	return cmp3(d < f, d > f)
}

func compareDoubleUint(d float64, u uint64) int {
	if d < 0 {
		return -1
	}
	if d > math.MaxUint64 {
		return 1
	}
	f := float64(u)
	return cmp3(d < f, d > f)
}

func compareIntUint(i int64, u uint64) int {
	if i < 0 || u > math.MaxInt64 {
		return -1
	}
	c := i - int64(u)
	return cmp3(c < 0, c > 0)
}

// relate is <, <=, > or >= (fn) on two values: false when either is NaN,
// as in cel-go, and an error when they don't compare.
//
//go:noinline
func relate(l, r Value, fn string, test func(int) bool) Value {
	if isNaN(l) || isNaN(r) {
		return BoolValue(false)
	}
	var c int
	switch l.k {
	case IntKind, UintKind, DoubleKind:
		var ok bool
		if c, ok = compareNum(l, r); !ok {
			return errValue(eNoOverload, fn)
		}
	case BoolKind:
		if r.k != BoolKind {
			return errValue(eNoOverload, fn)
		}
		c = cmp3(l.n < r.n, l.n > r.n)
	case StringKind:
		if r.k != StringKind {
			return errValue(eNoOverload, fn)
		}
		c = strings.Compare(l.s, r.s)
	case BytesKind:
		if r.k != BytesKind {
			return errValue(eNoOverload, fn)
		}
		c = strings.Compare(l.s, r.s)
	default:
		return errValue(eNoOverload, fn)
	}
	return BoolValue(test(c))
}

func isNaN(v Value) bool { return v.k == DoubleKind && math.IsNaN(math.Float64frombits(v.n)) }

// in is l in r: an element of a list equal to l, or a key of a map.
//
//go:noinline
func in(l, r Value) Value {
	switch r.k {
	case ListKind:
		ls := r.o.(List)
		for i, n := 0, ls.Len(); i < n; i++ {
			if equal(l, ls.At(i)) {
				return BoolValue(true)
			}
		}
		return BoolValue(false)
	case MapKind:
		_, ok := mapGet(r.o.(Map), l)
		return BoolValue(ok)
	}
	return errValue(eNoOverload, "@in")
}

// The arithmetic, as cel-go's: both sides of one kind, ints and uints
// checked for overflow, and division and modulus by zero errors.
func add(l, r Value) Value {
	if l.k != r.k {
		return errValue(eNoOverload, "_+_")
	}
	switch l.k {
	case IntKind:
		x, y := int64(l.n), int64(r.n)
		if y > 0 && x > math.MaxInt64-y || y < 0 && x < math.MinInt64-y {
			return errValue(eIntOverflow, "")
		}
		return IntValue(x + y)
	case UintKind:
		if r.n > 0 && l.n > math.MaxUint64-r.n {
			return errValue(eUintOverflow, "")
		}
		return UintValue(l.n + r.n)
	case DoubleKind:
		return DoubleValue(math.Float64frombits(l.n) + math.Float64frombits(r.n))
	case StringKind:
		return StringValue(l.s + r.s)
	case BytesKind:
		return Value{k: BytesKind, s: l.s + r.s}
	case ListKind:
		a, b := l.o.(List), r.o.(List)
		out := make(valueList, 0, a.Len()+b.Len())
		for i, n := 0, a.Len(); i < n; i++ {
			out = append(out, a.At(i))
		}
		for i, n := 0, b.Len(); i < n; i++ {
			out = append(out, b.At(i))
		}
		return Value{k: ListKind, o: out}
	}
	return errValue(eNoOverload, "_+_")
}

func sub(l, r Value) Value {
	if l.k != r.k {
		return errValue(eNoOverload, "_-_")
	}
	switch l.k {
	case IntKind:
		x, y := int64(l.n), int64(r.n)
		if y < 0 && x > math.MaxInt64+y || y > 0 && x < math.MinInt64+y {
			return errValue(eIntOverflow, "")
		}
		return IntValue(x - y)
	case UintKind:
		if r.n > l.n {
			return errValue(eUintOverflow, "")
		}
		return UintValue(l.n - r.n)
	case DoubleKind:
		return DoubleValue(math.Float64frombits(l.n) - math.Float64frombits(r.n))
	}
	return errValue(eNoOverload, "_-_")
}

func mul(l, r Value) Value {
	if l.k != r.k {
		return errValue(eNoOverload, "_*_")
	}
	switch l.k {
	case IntKind:
		x, y := int64(l.n), int64(r.n)
		if x == -1 && y == math.MinInt64 || y == -1 && x == math.MinInt64 ||
			x > 0 && y > 0 && x > math.MaxInt64/y || x > 0 && y < 0 && y < math.MinInt64/x ||
			x < 0 && y > 0 && x < math.MinInt64/y || x < 0 && y < 0 && y < math.MaxInt64/x {
			return errValue(eIntOverflow, "")
		}
		return IntValue(x * y)
	case UintKind:
		if r.n != 0 && l.n > math.MaxUint64/r.n {
			return errValue(eUintOverflow, "")
		}
		return UintValue(l.n * r.n)
	case DoubleKind:
		return DoubleValue(math.Float64frombits(l.n) * math.Float64frombits(r.n))
	}
	return errValue(eNoOverload, "_*_")
}

func div(l, r Value) Value {
	if l.k != r.k {
		return errValue(eNoOverload, "_/_")
	}
	switch l.k {
	case IntKind:
		x, y := int64(l.n), int64(r.n)
		if y == 0 {
			return errValue(eDivZero, "")
		}
		if x == math.MinInt64 && y == -1 {
			return errValue(eIntOverflow, "")
		}
		return IntValue(x / y)
	case UintKind:
		if r.n == 0 {
			return errValue(eDivZero, "")
		}
		return UintValue(l.n / r.n)
	case DoubleKind:
		return DoubleValue(math.Float64frombits(l.n) / math.Float64frombits(r.n))
	}
	return errValue(eNoOverload, "_/_")
}

func mod(l, r Value) Value {
	if l.k != r.k {
		return errValue(eNoOverload, "_%_")
	}
	switch l.k {
	case IntKind:
		x, y := int64(l.n), int64(r.n)
		if y == 0 {
			return errValue(eModZero, "")
		}
		if x == math.MinInt64 && y == -1 {
			return errValue(eIntOverflow, "")
		}
		return IntValue(x % y)
	case UintKind:
		if r.n == 0 {
			return errValue(eModZero, "")
		}
		return UintValue(l.n % r.n)
	}
	return errValue(eNoOverload, "_%_")
}

// callFn plans a call: its arguments are strict, as in cel-go's
// evalVarArgs: the first error is returned, else the first unknown.
func (c *compiler) callFn(x *Expr, args []evalFn) evalFn {
	if _, ok := builtins[x.Name]; ok {
		return standardCall(standardFn(x.Name), args)
	}
	var fn func(vs []Value) Value
	{
		f := c.env.funcs[x.Name]
		var only *Overload
		for i := range f.Overloads {
			if f.Overloads[i].ID == x.Overload {
				only = &f.Overloads[i]
			}
		}
		member := x.Member
		fn = func(vs []Value) Value {
			// One overload fits when checked: cel-go still checks the
			// values, as a dyn may hold anything.
			o := only
			switch {
			case o == nil:
				if o = pickOverload(f, member, vs); o == nil {
					return errValue(eNoOverload, f.Name)
				}
			case !fitsAll(o, vs):
				return errValue(eNoOverload, f.Name)
			}
			v, err := o.Fn(vs)
			if err != nil {
				return Value{k: errKind, n: uint64(eHelper), o: err}
			}
			return v
		}
	}
	switch len(args) {
	case 1:
		a0 := args[0]
		return func(a Activation) Value {
			v := a0(a)
			if v.k >= errKind {
				return v
			}
			var vs [1]Value
			vs[0] = v
			return fn(vs[:])
		}
	}
	return func(a Activation) Value {
		vs := make([]Value, len(args))
		var unk Value
		for i, f := range args {
			v := f(a)
			if v.k == errKind {
				return v
			}
			if v.k == unknownKind && unk.k != unknownKind {
				unk = v
			}
			vs[i] = v
		}
		if unk.k == unknownKind {
			return unk
		}
		return fn(vs)
	}
}

// pickOverload is the first of a helper's overloads that takes values of
// these kinds, for a call whose argument types fit more than one.
func pickOverload(f *Func, member bool, vs []Value) *Overload {
	for i := range f.Overloads {
		o := &f.Overloads[i]
		if o.Member != member || len(o.Args) != len(vs) {
			continue
		}
		ok := true
		for j, t := range o.Args {
			ok = ok && valueFits(vs[j], t)
		}
		if ok {
			return o
		}
	}
	return nil
}

// fitsAll reports whether each value fits its parameter of o.
func fitsAll(o *Overload, vs []Value) bool {
	for j, t := range o.Args {
		if !valueFits(vs[j], t) {
			return false
		}
	}
	return true
}

// valueFits reports whether a value can stand where type t is expected,
// by its kind.
func valueFits(v Value, t Type) bool {
	switch t.Kind {
	case DynKind:
		return true
	case NullKind:
		return v.k == NullKind
	case StructKind:
		return v.k == StructKind && v.s == t.Name || v.k == NullKind
	}
	return v.k == t.Kind
}

// standardCall calls a standard function, which takes one argument or
// two, with no slice between them: strict, as any call.
//
//go:noinline
func standardCall(fn func(a, b Value) Value, args []evalFn) evalFn {
	a0 := args[0]
	if len(args) == 1 {
		return func(a Activation) Value {
			v := a0(a)
			if v.k >= errKind {
				return v
			}
			return fn(v, Value{})
		}
	}
	a1 := args[1]
	return func(a Activation) Value {
		v := a0(a)
		if v.k == errKind {
			return v
		}
		w := a1(a)
		switch {
		case w.k == errKind:
			return w
		case v.k == unknownKind:
			return v
		case w.k == unknownKind:
			return w
		}
		return fn(v, w)
	}
}

// standardFn is a standard function, dispatched on its arguments' kinds.
func standardFn(name string) func(a, b Value) Value {
	switch name {
	case "size":
		return func(v, _ Value) Value {
			switch v.k {
			case StringKind:
				return IntValue(int64(utf8.RuneCountInString(v.s)))
			case BytesKind:
				return IntValue(int64(len(v.s)))
			case ListKind:
				return IntValue(int64(v.o.(List).Len()))
			case MapKind:
				return IntValue(int64(v.o.(Map).Len()))
			}
			return errValue(eNoOverload, "size")
		}
	case "contains", "startsWith", "endsWith":
		test := strings.Contains
		switch name {
		case "startsWith":
			test = strings.HasPrefix
		case "endsWith":
			test = strings.HasSuffix
		}
		return func(s, sub Value) Value {
			if s.k != StringKind || sub.k != StringKind {
				return errValue(eNoOverload, name)
			}
			return BoolValue(test(s.s, sub.s))
		}
	case "int":
		return func(v, _ Value) Value { return toInt(v) }
	case "uint":
		return func(v, _ Value) Value { return toUint(v) }
	case "double":
		return func(v, _ Value) Value { return toDouble(v) }
	case "string":
		return func(v, _ Value) Value { return toString(v) }
	case "bytes":
		return func(v, _ Value) Value {
			switch v.k {
			case BytesKind:
				return v
			case StringKind:
				return Value{k: BytesKind, s: v.s}
			}
			return convError(v, "bytes")
		}
	case "bool":
		return func(v, _ Value) Value {
			switch v.k {
			case BoolKind:
				return v
			case StringKind:
				if b, err := strconv.ParseBool(v.s); err == nil {
					return BoolValue(b)
				}
			}
			return convError(v, "bool")
		}
	case "dyn":
		return func(v, _ Value) Value { return v }
	}
	return func(_, _ Value) Value { return errValue(eNoOverload, name) }
}

func convError(v Value, to string) Value {
	return errValue(eMessage, "type conversion error from '"+kindNames[v.k]+"' to '"+to+"'")
}

func toInt(v Value) Value {
	switch v.k {
	case IntKind:
		return v
	case UintKind:
		if v.n > math.MaxInt64 {
			return errValue(eIntOverflow, "")
		}
		return IntValue(int64(v.n))
	case DoubleKind:
		f := math.Float64frombits(v.n)
		if math.IsInf(f, 0) || math.IsNaN(f) || f <= math.MinInt64 || f >= math.MaxInt64 {
			return errValue(eIntOverflow, "")
		}
		return IntValue(int64(f))
	case StringKind:
		if n, err := strconv.ParseInt(v.s, 10, 64); err == nil {
			return IntValue(n)
		}
	}
	return convError(v, "int")
}

func toUint(v Value) Value {
	switch v.k {
	case UintKind:
		return v
	case IntKind:
		if int64(v.n) < 0 {
			return errValue(eUintOverflow, "")
		}
		return UintValue(v.n)
	case DoubleKind:
		f := math.Float64frombits(v.n)
		if math.IsInf(f, 0) || math.IsNaN(f) || f < 0 || f >= twoTo64 {
			return errValue(eUintOverflow, "")
		}
		return UintValue(uint64(f))
	case StringKind:
		if n, err := strconv.ParseUint(v.s, 10, 64); err == nil {
			return UintValue(n)
		}
	}
	return convError(v, "uint")
}

func toDouble(v Value) Value {
	switch v.k {
	case DoubleKind:
		return v
	case IntKind:
		return DoubleValue(float64(int64(v.n)))
	case UintKind:
		return DoubleValue(float64(v.n))
	case StringKind:
		if f, err := strconv.ParseFloat(v.s, 64); err == nil {
			return DoubleValue(f)
		}
	}
	return convError(v, "double")
}

// errCode is an evaluation error's kind; its message is made only when
// Eval returns it, so a context that doesn't hold costs no allocation.
type errCode uint8

const (
	eNoAttr errCode = iota + 1
	eNoKey
	eNoField
	eNoOverload
	eDivZero
	eModZero
	eIntOverflow
	eUintOverflow
	eBadValue
	eMessage
	eHelper
)

func errValue(c errCode, detail string) Value { return Value{k: errKind, n: uint64(c), s: detail} }

// errorOf is an error Value as the error Eval returns.
func errorOf(v Value) error {
	var msg string
	switch errCode(v.n) {
	case eNoAttr:
		msg = "no such attribute: " + v.s
	case eNoKey:
		msg = "no such key: " + v.s
	case eNoField:
		msg = "no such field: " + v.s
	case eNoOverload:
		msg = "no such overload: " + v.s
	case eDivZero:
		msg = "division by zero"
	case eModZero:
		msg = "modulus by zero"
	case eIntOverflow:
		msg = "integer overflow"
	case eUintOverflow:
		msg = "unsigned integer overflow"
	case eBadValue:
		msg = "unsupported value: " + v.s
	case eHelper:
		err := v.o.(error)
		return &EvalError{Msg: err.Error(), Err: err}
	default:
		msg = v.s
	}
	return &EvalError{Msg: msg}
}
