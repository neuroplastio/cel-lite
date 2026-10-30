package cellite

// Op is what an expression node does.
type Op uint8

const (
	// OpLit is a literal: Value is nil (null), a bool, an int64, a
	// uint64, a float64, a string or a []byte.
	OpLit Op = iota + 1
	// OpIdent is a name as Parse read it, before Check resolves it. A
	// checked expression has none.
	OpIdent
	// OpVar is a declared variable: Name, and Ref its index in the Env.
	// A qualified name (`pane.id`) that the Env declares is one OpVar.
	OpVar
	// OpSelect is Args[0].Name: a struct's field, whose index in its
	// Struct's Fields Check sets as Ref, or a map's key.
	OpSelect
	// OpHas is has(Args[0].Name): whether a map holds the key.
	OpHas
	// OpIndex is Args[0][Args[1]]: a list's element or a map's value.
	OpIndex
	// OpCall calls the function Name. A method call (Member) has its
	// receiver as Args[0]. Check sets Overload.
	OpCall
	// OpCond is Args[0] ? Args[1] : Args[2].
	OpCond
	// OpAnd, OpOr and OpNot are &&, || and !. && and || hold on false
	// and true from either side, whatever the other side gives.
	OpAnd
	OpOr
	OpNot
	// OpNeg is unary minus.
	OpNeg
	// The comparisons: Args[0] op Args[1].
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
	// OpIn is Args[0] in Args[1]: a list's element or a map's key.
	OpIn
	// The arithmetic: Args[0] op Args[1].
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	// OpList is a list literal: [Args...].
	OpList
	// OpMap is a map literal: Args holds each key, then its value.
	OpMap
)

// Expr is a node of an expression: what Parse reads and Check types. It
// is plain data, so a program that generates code from an expression
// walks it with a switch on Op. Parse and Check make a tree's nodes in a
// few chunks: a program may change a node or give it new Args, and
// appending to a node's Args copies them.
type Expr struct {
	Op Op
	// Pos is the node's byte offset in the source: an operator's for an
	// operation, else where the node starts.
	Pos int
	// Type is the node's type, set by Check.
	Type Type
	// Value is a literal's value (OpLit).
	Value any
	// Name is a variable's or an identifier's name (OpVar, OpIdent), the
	// field or key selected (OpSelect, OpHas), or the function called
	// (OpCall).
	Name string
	// Ref is a variable's index in the Env's Vars (OpVar), or a field's
	// index in its Struct's Fields (OpSelect on a struct); set by Check.
	Ref int
	// Overload names what a call runs, set by Check: a standard
	// function's overload as cel-go names it ("size_string",
	// "string_to_int64"), or a helper's Overload.ID. It is empty when
	// more than one overload fits the argument types, and the one to run
	// is decided by the values.
	Overload string
	// Member is set on a method call, `x.f(y)`: the receiver is Args[0].
	Member bool
	// Args are the node's operands, in the order its Op gives.
	Args []*Expr
}

// nodes hands out a tree's nodes and operand lists from chunks, so that
// Parse and Check make a few allocations for a tree, not two a node. A
// chunk is never grown in place, so a node keeps its address; a list
// handed out has no room past its end, so appending to it copies.
type nodes struct {
	exprs []Expr
	args  []*Expr
	// size is a chunk's length: the tree's node count, or a bound on it.
	size int
}

// expr is a new zero node.
//
//go:noinline
func (n *nodes) expr() *Expr {
	if len(n.exprs) == cap(n.exprs) {
		n.exprs = make([]Expr, 0, max(n.size, 1))
	}
	n.exprs = n.exprs[:len(n.exprs)+1]
	return &n.exprs[len(n.exprs)-1]
}

// list is a new list of k operands, nil when k is 0.
//
//go:noinline
func (n *nodes) list(k int) []*Expr {
	if k == 0 {
		return nil
	}
	if len(n.args)+k > cap(n.args) {
		n.args = make([]*Expr, 0, max(n.size, k))
	}
	i := len(n.args)
	n.args = n.args[:i+k]
	return n.args[i : i+k : i+k]
}

// treeSize is the number of x's nodes, counting a node as often as the
// tree reaches it. x is no deeper than MaxDepth.
func treeSize(x *Expr) int {
	n := 1
	for _, a := range x.Args {
		if a != nil {
			n += treeSize(a)
		}
	}
	return n
}
