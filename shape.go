package cellite

import "strconv"

// A program may build a tree, or change one that Parse or Check gave it,
// and hand it to Check or Program. These walks refuse a tree that Check
// couldn't take or couldn't have returned, so that nothing panics later:
// not on any tree, as not on any source.

// arity is how many operands a node of each Op has: -1 for any number,
// -2 for an even number.
var arity = [...]int8{
	OpLit: 0, OpIdent: 0, OpVar: 0, OpSelect: 1, OpHas: 1, OpIndex: 2, OpCall: -1, OpCond: 3,
	OpAnd: 2, OpOr: 2, OpNot: 1, OpNeg: 1,
	OpEq: 2, OpNe: 2, OpLt: 2, OpLe: 2, OpGt: 2, OpGe: 2, OpIn: 2,
	OpAdd: 2, OpSub: 2, OpMul: 2, OpDiv: 2, OpMod: 2,
	OpList: -1, OpMap: -2,
}

// malformed is the error for a tree that isn't one: nil, deeper than
// MaxDepth, or with a node whose Op isn't one or whose operands don't
// fit its Op.
func malformed(x *Expr) error {
	if x == nil {
		return &Error{Kind: SyntaxError, Msg: "no expression: the tree is nil"}
	}
	if deepNode(x, 1) != nil {
		return &Error{Kind: SyntaxError, Pos: x.Pos, Msg: "expression is nested deeper than " + strconv.Itoa(MaxDepth)}
	}
	if m := misshapen(x); m != nil {
		return &Error{Kind: SyntaxError, Pos: m.Pos, Msg: "a malformed node: op " + strconv.Itoa(int(m.Op)) +
			" with " + strconv.Itoa(len(m.Args)) + " operands"}
	}
	return nil
}

// misshapen is a node of x whose Op isn't one, or whose operands don't
// fit its Op: too many, too few, or nil. x is no deeper than MaxDepth.
func misshapen(x *Expr) *Expr {
	probe()
	if x.Op == 0 || int(x.Op) >= len(arity) {
		return x
	}
	switch n := arity[x.Op]; {
	case n >= 0 && len(x.Args) != int(n),
		n == -2 && len(x.Args)%2 != 0,
		x.Op == OpCall && x.Member && len(x.Args) == 0:
		return x
	}
	for _, a := range x.Args {
		if a == nil {
			return x
		}
		if m := misshapen(a); m != nil {
			return m
		}
	}
	return nil
}

// unchecked is a node of x, a tree meant to be one Check returned in e,
// that refers to what e doesn't declare: a variable or a field by an
// index out of range, a helper e hasn't, or a call whose operands an
// overload can't take in number. x is well formed.
func (e *Env) unchecked(x *Expr) *Expr {
	probe()
	switch x.Op {
	case OpVar:
		if x.Ref < 0 || x.Ref >= len(e.vars) {
			return x
		}
	case OpSelect:
		if t := x.Args[0].Type; t.Kind == StructKind {
			if s := e.structs[t.Name]; s == nil || x.Ref < 0 || x.Ref >= len(s.Fields) {
				return x
			}
		}
	case OpCall:
		if !e.callable(x) {
			return x
		}
	}
	for _, a := range x.Args {
		if n := e.unchecked(a); n != nil {
			return n
		}
	}
	return nil
}

// callable reports whether a call names a function e has, with as many
// operands as one of its overloads takes.
//
//go:noinline
func (e *Env) callable(x *Expr) bool {
	if b, ok := builtins[x.Name]; ok {
		return len(x.Args) == len(b[0].args)
	}
	f := e.funcs[x.Name]
	if f == nil {
		return false
	}
	for _, o := range f.Overloads {
		if o.ID == x.Overload || x.Overload == "" {
			if len(o.Args) == len(x.Args) {
				return true
			}
		}
	}
	return false
}
