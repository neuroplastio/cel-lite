package cellite

import "strings"

// Check resolves a parsed expression's names in the Env and types every
// node, as cel-go's checker does, and returns the checked tree, a new
// one: x is left as it is. An undeclared name, types that don't fit and
// a standard function this package doesn't implement are errors
// (*Error). Check has no source, so the Error's Line and Col are 0;
// Compile fills them in.
func (e *Env) Check(x *Expr) (*Expr, error) {
	c := &checker{env: e}
	y := c.check(x)
	if c.err != nil {
		return nil, c.err
	}
	settleTree(y)
	return y, nil
}

// checker checks one expression; the first error stops it.
type checker struct {
	env *Env
	err *Error
}

func (c *checker) fail(pos int, kind ErrorKind, name, msg string) *Expr {
	if c.err == nil {
		c.err = &Error{Kind: kind, Pos: pos, Name: name, Msg: msg}
	}
	return nil
}

// settleTree makes every free type in the tree dyn.
func settleTree(x *Expr) {
	x.Type = settle(x.Type)
	for _, a := range x.Args {
		settleTree(a)
	}
}

// opNames are the operators as cel-go names them in its errors.
var opNames = map[Op]string{
	OpEq: "_==_", OpNe: "_!=_", OpLt: "_<_", OpLe: "_<=_", OpGt: "_>_", OpGe: "_>=_", OpIn: "@in",
	OpAdd: "_+_", OpSub: "_-_", OpMul: "_*_", OpDiv: "_/_", OpMod: "_%_", OpNeg: "-_", OpNot: "!_",
	OpAnd: "_&&_", OpOr: "_||_", OpCond: "_?_:_", OpIndex: "_[_]",
}

// noOverload is cel-go's error for operands no overload takes.
func (c *checker) noOverload(pos int, fn string, args ...*Expr) *Expr {
	return c.fail(pos, TypeError, "", "found no matching overload for '"+fn+"' applied to '("+typeList(args)+")'")
}

func typeList(args []*Expr) string {
	ts := make([]string, len(args))
	for i, a := range args {
		ts[i] = FormatType(settle(a.Type))
	}
	return strings.Join(ts, ", ")
}

func (c *checker) check(x *Expr) *Expr {
	if c.err != nil || x == nil {
		return nil
	}
	y := &Expr{Op: x.Op, Pos: x.Pos, Value: x.Value, Name: x.Name, Member: x.Member}
	if len(x.Args) > 0 && x.Op != OpSelect && x.Op != OpCall {
		y.Args = make([]*Expr, len(x.Args))
		for i, a := range x.Args {
			if y.Args[i] = c.check(a); c.err != nil {
				return nil
			}
		}
	}
	switch x.Op {
	case OpLit:
		switch x.Value.(type) {
		case nil:
			y.Type = Null
		case bool:
			y.Type = Bool
		case int64:
			y.Type = Int
		case uint64:
			y.Type = Uint
		case float64:
			y.Type = Double
		case string:
			y.Type = String
		case []byte:
			y.Type = Bytes
		default:
			return c.fail(x.Pos, TypeError, "", "a literal's value is nil, a bool, an int64, a uint64, a float64, a string or []byte")
		}
	case OpIdent, OpVar:
		return c.ident(x)
	case OpSelect:
		return c.selectField(x)
	case OpHas:
		t := y.Args[0].Type
		switch {
		case t.Kind == MapKind || isDynLike(t):
		case t.Kind == StructKind:
			return c.fail(x.Pos, Unsupported, "has() on a struct", "has() on a struct's field is not supported")
		default:
			return c.fail(x.Pos, TypeError, "", "type '"+FormatType(settle(t))+"' does not support field selection")
		}
		y.Type = Bool
	case OpIndex:
		if k := y.Args[1]; k.Op == OpLit {
			switch k.Value.(type) {
			case nil, []byte:
				// cel-go plans a literal index as a qualifier, which is
				// never null or bytes.
				return c.fail(k.Pos, TypeError, "", "invalid qualifier type: "+FormatType(k.Type))
			}
		}
		t, i := y.Args[0].Type, y.Args[1].Type
		switch {
		case t.Kind == ListKind && assignable(Int, i):
			y.Type = elemOf(t)
		case t.Kind == MapKind && assignable(keyOf(t), i):
			y.Type = elemOf(t)
		case t.Kind == DynKind && assignable(Int, i):
			// cel-go's index_list and index_map both fit, with different
			// parameters as results: dyn.
			y.Type = Dyn
		case isDynLike(t):
			// Only index_map fits (`x['k']`), or the operand is itself a
			// parameter, which the first overload binds: a parameter.
			y.Type = param
		default:
			return c.noOverload(x.Pos, "_[_]", y.Args...)
		}
		if y.Type.Kind == freeKind {
			y.Type = param
		}
	case OpCall:
		return c.call(x)
	case OpCond:
		cond, t, f := y.Args[0], y.Args[1], y.Args[2]
		if !assignable(Bool, cond.Type) || !assignable(t.Type, f.Type) {
			return c.noOverload(x.Pos, "_?_:_", y.Args...)
		}
		if binds(t.Type, f.Type) {
			return c.inferred(x.Pos)
		}
		y.Type = mostGeneral(f.Type, t.Type)
	case OpAnd, OpOr:
		for _, a := range y.Args {
			if !assignable(Bool, a.Type) {
				return c.fail(a.Pos, TypeError, "", "expected type 'bool' but found '"+FormatType(settle(a.Type))+"'")
			}
		}
		y.Type = Bool
	case OpNot:
		if !assignable(Bool, y.Args[0].Type) {
			return c.noOverload(x.Pos, "!_", y.Args...)
		}
		y.Type = Bool
	case OpNeg:
		switch t := y.Args[0].Type; {
		case t.Kind == IntKind || t.Kind == DoubleKind:
			y.Type = t
		case t.Kind == paramKind:
			return c.inferred(x.Pos)
		case isDynLike(t):
			y.Type = Dyn
		default:
			return c.noOverload(x.Pos, "-_", y.Args...)
		}
	case OpEq, OpNe:
		l, r := y.Args[0].Type, y.Args[1].Type
		if !assignable(l, r) {
			return c.noOverload(x.Pos, opNames[x.Op], y.Args...)
		}
		if hasStruct(l) || hasStruct(r) {
			return c.fail(x.Pos, Unsupported, "== on structs", "comparing structs is not supported")
		}
		y.Type = Bool
	case OpLt, OpLe, OpGt, OpGe:
		if !matchRelation(y.Args[0].Type, y.Args[1].Type) {
			if nullTime(y.Args[0].Type, y.Args[1].Type) {
				return c.timeOverload(x.Pos)
			}
			return c.noOverload(x.Pos, opNames[x.Op], y.Args...)
		}
		y.Type = Bool
	case OpIn:
		l, r := y.Args[0].Type, y.Args[1].Type
		switch {
		case r.Kind == ListKind && assignable(l, elemOf(r)):
		case r.Kind == MapKind && assignable(l, keyOf(r)):
		case isDynLike(r):
		default:
			return c.noOverload(x.Pos, "@in", y.Args...)
		}
		if hasStruct(l) {
			return c.fail(x.Pos, Unsupported, "== on structs", "comparing structs is not supported")
		}
		y.Type = Bool
	case OpAdd, OpSub, OpMul, OpDiv, OpMod:
		l, r := y.Args[0].Type, y.Args[1].Type
		if (l.Kind == paramKind || r.Kind == paramKind) && isDynLike(l) && isDynLike(r) ||
			x.Op == OpAdd && l.Kind == ListKind && r.Kind == ListKind && assignable(l, r) && binds(l, r) {
			return c.inferred(x.Pos)
		}
		t, ok := arithResult(x.Op, l, r)
		if !ok {
			if (x.Op == OpAdd || x.Op == OpSub) && nullTime(y.Args[0].Type, y.Args[1].Type) {
				return c.timeOverload(x.Pos)
			}
			return c.noOverload(x.Pos, opNames[x.Op], y.Args...)
		}
		y.Type = t
	case OpList:
		elem := free
		for i, a := range y.Args {
			if i == 0 {
				elem = a.Type
			} else if elem = c.join(a.Pos, elem, a.Type); c.err != nil {
				return nil
			}
		}
		y.Type = ListOf(elem)
	case OpMap:
		key, val := free, free
		for i := 0; i < len(y.Args); i += 2 {
			k, v := y.Args[i], y.Args[i+1]
			switch k.Type.Kind {
			case BoolKind, IntKind, UintKind, StringKind, DynKind, freeKind, paramKind:
			default:
				return c.fail(k.Pos, Unsupported, "map key "+FormatType(settle(k.Type)),
					"a map's key is a bool, an int, a uint or a string, not '"+FormatType(settle(k.Type))+"'")
			}
			if i == 0 {
				key, val = k.Type, v.Type
			} else if key, val = c.join(k.Pos, key, k.Type), c.join(v.Pos, val, v.Type); c.err != nil {
				return nil
			}
		}
		y.Type = MapOf(key, val)
	default:
		return c.fail(x.Pos, SyntaxError, "", "not an expression node")
	}
	return y
}

// param is the type of a value read with [] from a dyn, or read out of
// an empty literal (paramKind).
var param = Type{Kind: paramKind}

// inferred refuses an expression cel-go types by binding a parameter
// (paramKind) to a concrete type, so that the expression's type, or the
// overload it calls, no longer follows the value: `-x['k']` is a double
// in cel-go, `[x['k'], 'a']` a list(string), and `int(x['k'])` takes
// only an int. Reading the value as a field (`x.k`) or through dyn()
// keeps it dyn.
func (c *checker) inferred(pos int) *Expr {
	return c.fail(pos, Unsupported, "inferred type",
		"the type of a value read with [] from a dyn or an empty literal would be inferred from its use here; read it as x.k or wrap it in dyn()")
}

// hasParam reports whether t is or holds a parameter.
func hasParam(t Type) bool {
	switch t.Kind {
	case paramKind:
		return true
	case ListKind:
		return hasParam(elemOf(t))
	case MapKind:
		return hasParam(keyOf(t)) || hasParam(elemOf(t))
	}
	return false
}

// nullTime reports whether operands that no overload here takes would
// fit one of cel-go's timestamp or duration overloads: null stands for
// either, and dyn for anything. Such an expression is refused by name.
func nullTime(ts ...Type) bool {
	null := false
	for _, t := range ts {
		if t.Kind != NullKind && !isDynLike(t) {
			return false
		}
		null = null || t.Kind == NullKind
	}
	return null
}

func (c *checker) timeOverload(pos int) *Expr {
	return c.fail(pos, Unsupported, "timestamps", "timestamps and durations are not supported (null takes their place here)")
}

// join is the element type of a literal holding values of types a and b,
// as cel-go's joinTypes: the more general one when they fit, else dyn.
// A parameter one of them would bind is refused (inferred).
func (c *checker) join(pos int, a, b Type) Type {
	switch {
	case !assignable(a, b):
		return Dyn
	case binds(a, b):
		c.inferred(pos)
		return Dyn
	}
	return mostGeneral(a, b)
}

// hasStruct reports whether values of t may be or hold structs, which
// can't be compared.
func hasStruct(t Type) bool {
	switch t.Kind {
	case StructKind:
		return true
	case ListKind:
		return hasStruct(elemOf(t))
	case MapKind:
		return hasStruct(elemOf(t))
	}
	return false
}

// ident resolves a name to a variable.
func (c *checker) ident(x *Expr) *Expr {
	if i, ok := c.env.varIdx[x.Name]; ok {
		return &Expr{Op: OpVar, Pos: x.Pos, Name: x.Name, Ref: i, Type: c.env.vars[i].Type}
	}
	if typeNames[x.Name] {
		return c.fail(x.Pos, Unsupported, "type "+x.Name, "type values ('"+x.Name+"') are not supported")
	}
	return c.fail(x.Pos, Undeclared, x.Name, "undeclared reference to '"+x.Name+"'")
}

// qualified is the dotted name a chain of selections on an identifier
// spells: `a.b.c`.
func qualified(x *Expr) (string, bool) {
	switch x.Op {
	case OpIdent:
		return x.Name, true
	case OpSelect:
		if q, ok := qualified(x.Args[0]); ok {
			return q + "." + x.Name, true
		}
	}
	return "", false
}

// selectField checks a selection: a qualified name the Env declares is
// that variable, as in cel-go, else a struct's field or a map's key.
func (c *checker) selectField(x *Expr) *Expr {
	if q, ok := qualified(x); ok {
		if i, ok := c.env.varIdx[q]; ok {
			return &Expr{Op: OpVar, Pos: x.Pos, Name: q, Ref: i, Type: c.env.vars[i].Type}
		}
	}
	op := c.check(x.Args[0])
	if c.err != nil {
		return nil
	}
	y := &Expr{Op: OpSelect, Pos: x.Pos, Name: x.Name, Args: []*Expr{op}}
	switch t := op.Type; {
	case t.Kind == MapKind:
		if y.Type = elemOf(t); y.Type.Kind == freeKind {
			y.Type = param
		}
	case t.Kind == StructKind:
		s := c.env.structs[t.Name]
		i, ok := s.fields[x.Name]
		if !ok {
			return c.fail(x.Pos, TypeError, x.Name, "undefined field '"+x.Name+"'")
		}
		y.Ref, y.Type = i, s.Fields[i].Type
	case isDynLike(t):
		y.Type = Dyn
	default:
		return c.fail(x.Pos, TypeError, "", "type '"+FormatType(settle(t))+"' does not support field selection")
	}
	return y
}

// relations are the operand kinds CEL's <, <=, > and >= take when they
// are checked. cel-go declares overloads across int, uint and double too,
// but its default environment leaves them out, so `1 < 1.5` is refused;
// through dyn the values still compare across kinds, as they do in
// cel-go.
var relations = [][2]Kind{
	{BoolKind, BoolKind}, {IntKind, IntKind}, {UintKind, UintKind}, {DoubleKind, DoubleKind},
	{StringKind, StringKind}, {BytesKind, BytesKind},
}

// fits reports whether a value of type t can be an argument of kind k.
func fits(k Kind, t Type) bool { return isDynLike(t) || t.Kind == k }

func matchRelation(l, r Type) bool {
	for _, s := range relations {
		if fits(s[0], l) && fits(s[1], r) {
			return true
		}
	}
	return false
}

// arithmetic are the operand kinds each arithmetic operator takes, in
// cel-go's order; the result is the operands' kind.
var arithmetic = map[Op][]Kind{
	OpAdd: {BytesKind, DoubleKind, IntKind, ListKind, StringKind, UintKind},
	OpSub: {DoubleKind, IntKind, UintKind},
	OpMul: {DoubleKind, IntKind, UintKind},
	OpDiv: {DoubleKind, IntKind, UintKind},
	OpMod: {IntKind, UintKind},
}

// arithResult is the type of l op r: the first overload's result, or dyn
// when overloads with different results fit, as in cel-go.
func arithResult(op Op, l, r Type) (Type, bool) {
	var out Type
	found := false
	for _, k := range arithmetic[op] {
		if !fits(k, l) || !fits(k, r) {
			continue
		}
		t := Type{Kind: k}
		if k == ListKind {
			// (list(A), list(A)): A takes l's element, then r's.
			switch {
			case l.Kind == ListKind && r.Kind == ListKind:
				if !assignable(elemOf(l), elemOf(r)) {
					continue
				}
				t = ListOf(mostGeneral(elemOf(r), elemOf(l)))
			case l.Kind == ListKind:
				t = l
			case r.Kind == ListKind:
				t = r
			default:
				t = ListOf(free)
			}
		}
		switch {
		case !found:
			out, found = t, true
		case out.Kind != DynKind && !TypeEqual(out, t):
			out = Dyn
		}
	}
	return out, found
}

// builtin is an overload of a standard function: whether it is a method,
// its parameters' kinds (DynKind takes anything), its result, and cel-go's
// name for it.
type builtin struct {
	member bool
	args   []Kind
	result Kind
	id     string
}

var builtins = map[string][]builtin{
	"size": {
		{false, []Kind{StringKind}, IntKind, "size_string"},
		{false, []Kind{BytesKind}, IntKind, "size_bytes"},
		{false, []Kind{ListKind}, IntKind, "size_list"},
		{false, []Kind{MapKind}, IntKind, "size_map"},
		{true, []Kind{StringKind}, IntKind, "string_size"},
		{true, []Kind{BytesKind}, IntKind, "bytes_size"},
		{true, []Kind{ListKind}, IntKind, "list_size"},
		{true, []Kind{MapKind}, IntKind, "map_size"},
	},
	"contains":   {{true, []Kind{StringKind, StringKind}, BoolKind, "contains_string"}},
	"startsWith": {{true, []Kind{StringKind, StringKind}, BoolKind, "starts_with_string"}},
	"endsWith":   {{true, []Kind{StringKind, StringKind}, BoolKind, "ends_with_string"}},
	"int": {
		{false, []Kind{IntKind}, IntKind, "int64_to_int64"},
		{false, []Kind{UintKind}, IntKind, "uint64_to_int64"},
		{false, []Kind{DoubleKind}, IntKind, "double_to_int64"},
		{false, []Kind{StringKind}, IntKind, "string_to_int64"},
	},
	"uint": {
		{false, []Kind{UintKind}, UintKind, "uint64_to_uint64"},
		{false, []Kind{IntKind}, UintKind, "int64_to_uint64"},
		{false, []Kind{DoubleKind}, UintKind, "double_to_uint64"},
		{false, []Kind{StringKind}, UintKind, "string_to_uint64"},
	},
	"double": {
		{false, []Kind{DoubleKind}, DoubleKind, "double_to_double"},
		{false, []Kind{IntKind}, DoubleKind, "int64_to_double"},
		{false, []Kind{UintKind}, DoubleKind, "uint64_to_double"},
		{false, []Kind{StringKind}, DoubleKind, "string_to_double"},
	},
	"string": {
		{false, []Kind{StringKind}, StringKind, "string_to_string"},
		{false, []Kind{BoolKind}, StringKind, "bool_to_string"},
		{false, []Kind{IntKind}, StringKind, "int64_to_string"},
		{false, []Kind{UintKind}, StringKind, "uint64_to_string"},
		{false, []Kind{DoubleKind}, StringKind, "double_to_string"},
		{false, []Kind{BytesKind}, StringKind, "bytes_to_string"},
	},
	"bytes": {
		{false, []Kind{BytesKind}, BytesKind, "bytes_to_bytes"},
		{false, []Kind{StringKind}, BytesKind, "string_to_bytes"},
	},
	"bool": {
		{false, []Kind{BoolKind}, BoolKind, "bool_to_bool"},
		{false, []Kind{StringKind}, BoolKind, "string_to_bool"},
	},
	"dyn": {{false, []Kind{DynKind}, DynKind, "to_dyn"}},
}

// refused are CEL's standard functions this package doesn't implement.
var refused = map[string]string{
	"matches":   "regular expressions (matches) are not supported",
	"type":      "type values (type()) are not supported",
	"timestamp": "timestamps are not supported",
	"duration":  "durations are not supported",
}

// call checks a call: a standard function, a refused one, or a helper.
func (c *checker) call(x *Expr) *Expr {
	y := &Expr{Op: OpCall, Pos: x.Pos, Name: x.Name, Member: x.Member, Args: make([]*Expr, len(x.Args))}
	for i, a := range x.Args {
		if y.Args[i] = c.check(a); c.err != nil {
			return nil
		}
	}
	if why, ok := refused[x.Name]; ok {
		return c.fail(x.Pos, Unsupported, "function "+x.Name, why)
	}
	if strings.HasPrefix(x.Name, "get") && standard[x.Name] {
		return c.fail(x.Pos, Unsupported, "function "+x.Name, "timestamps are not supported")
	}
	// cel-go binds a parameter argument to the first overload that fits
	// and calls only that one; size alone dispatches on the value.
	param := false
	for _, a := range y.Args {
		param = param || hasParam(a.Type)
	}
	if bs, ok := builtins[x.Name]; ok {
		found := false
		for _, b := range bs {
			if b.member != x.Member || len(b.args) != len(y.Args) {
				continue
			}
			match := true
			for i, k := range b.args {
				match = match && (k == DynKind || fits(k, y.Args[i].Type))
			}
			if !match {
				continue
			}
			t := Type{Kind: b.result}
			if !found {
				y.Type, y.Overload, found = t, b.id, true
			} else {
				if param && x.Name != "size" {
					return c.inferred(x.Pos)
				}
				y.Overload = ""
				if y.Type.Kind != DynKind && !TypeEqual(y.Type, t) {
					y.Type = Dyn
				}
			}
		}
		if !found {
			if (x.Name == "int" || x.Name == "string") && !x.Member && len(y.Args) == 1 && nullTime(y.Args[0].Type) {
				return c.timeOverload(x.Pos)
			}
			return c.noCall(x, y)
		}
		return y
	}
	if f, ok := c.env.funcs[x.Name]; ok {
		found := false
		for _, o := range f.Overloads {
			if o.Member != x.Member || len(o.Args) != len(y.Args) {
				continue
			}
			match := true
			for i, t := range o.Args {
				match = match && assignable(t, y.Args[i].Type)
			}
			if !match {
				continue
			}
			if !found {
				y.Type, y.Overload, found = o.Result, o.ID, true
			} else {
				if param {
					return c.inferred(x.Pos)
				}
				y.Overload = ""
				if y.Type.Kind != DynKind && !TypeEqual(y.Type, o.Result) {
					y.Type = Dyn
				}
			}
		}
		if !found {
			return c.noCall(x, y)
		}
		return y
	}
	return c.fail(x.Pos, Undeclared, x.Name, "undeclared reference to '"+x.Name+"'")
}

// noCall is the error for a call no overload takes, as cel-go words it:
// `size(int)`, a method's receiver before the dot.
func (c *checker) noCall(x, y *Expr) *Expr {
	if x.Member {
		return c.fail(x.Pos, TypeError, "", "found no matching overload for '"+x.Name+"' applied to '"+
			FormatType(settle(y.Args[0].Type))+".("+typeList(y.Args[1:])+")'")
	}
	return c.noOverload(x.Pos, x.Name, y.Args...)
}
