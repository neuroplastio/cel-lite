package cellite

import "strings"

// Kind is what sort of value a Type describes.
type Kind uint8

const (
	// NullKind is null's. It is the zero Kind, so the zero Value is null.
	NullKind Kind = iota
	// DynKind is any value, checked when the expression runs. A Type must
	// say it: the zero Type is null's, which NewEnv refuses as a
	// variable's or a field's type.
	DynKind
	// The kinds of CEL's scalar values, its lists and its maps: bool,
	// int (int64), uint (uint64), double (float64), string, bytes, list
	// and map.
	BoolKind
	IntKind
	UintKind
	DoubleKind
	StringKind
	BytesKind
	ListKind
	MapKind
	// StructKind is a struct the program declares (Decls.Structs), named
	// by Type.Name.
	StructKind

	// freeKind is the element type of an empty list or map literal: it
	// takes the type of whatever it is joined with, as a type parameter
	// does in cel-go's checker. Check leaves none in its output; what is
	// still free there becomes dyn.
	freeKind Kind = 255
	// paramKind is the type of a value read from a dyn with [], other
	// than by an int (`x['k']`), or read out of an empty literal: what
	// cel-go types with a fresh type parameter. cel-go then binds it to
	// the first overload or the literal that takes it, so `-x['k']` is a
	// double there and `int(x['k'])` takes only an int; Check refuses
	// such a binding by name (inferred). Otherwise it is dyn.
	paramKind Kind = 252
)

// Type is a CEL type: a kind, with a list's element, a map's key and
// value, or a struct's name. Build one with the variables below, ListOf,
// MapOf and StructOf.
type Type struct {
	Kind Kind
	// Name is a struct's name; empty for every other kind.
	Name string
	// Elem is a list's element type, or a map's value type.
	Elem *Type
	// Key is a map's key type.
	Key *Type
}

// The types that have no parameters.
var (
	Dyn    = Type{Kind: DynKind}
	Null   = Type{Kind: NullKind}
	Bool   = Type{Kind: BoolKind}
	Int    = Type{Kind: IntKind}
	Uint   = Type{Kind: UintKind}
	Double = Type{Kind: DoubleKind}
	String = Type{Kind: StringKind}
	Bytes  = Type{Kind: BytesKind}
)

// free is the element type of an empty literal (freeKind).
var free = Type{Kind: freeKind}

// ListOf is the type of a list of elem.
func ListOf(elem Type) Type { return Type{Kind: ListKind, Elem: &elem} }

// MapOf is the type of a map from key to val.
func MapOf(key, val Type) Type { return Type{Kind: MapKind, Key: &key, Elem: &val} }

// StructOf is the type of the struct the program declares as name.
func StructOf(name string) Type { return Type{Kind: StructKind, Name: name} }

// TypeEqual reports whether a and b are the same type.
func TypeEqual(a, b Type) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case ListKind:
		return TypeEqual(elemOf(a), elemOf(b))
	case MapKind:
		return TypeEqual(keyOf(a), keyOf(b)) && TypeEqual(elemOf(a), elemOf(b))
	case StructKind:
		return a.Name == b.Name
	}
	return true
}

// FormatType writes a type as CEL does: int, list(string),
// map(string, int), a struct's name.
func FormatType(t Type) string {
	var b strings.Builder
	formatType(&b, t)
	return b.String()
}

func formatType(b *strings.Builder, t Type) {
	switch t.Kind {
	case ListKind:
		b.WriteString("list(")
		formatType(b, elemOf(t))
		b.WriteByte(')')
	case MapKind:
		b.WriteString("map(")
		formatType(b, keyOf(t))
		b.WriteString(", ")
		formatType(b, elemOf(t))
		b.WriteByte(')')
	case StructKind:
		b.WriteString(t.Name)
	default:
		b.WriteString(kindNames[t.Kind])
	}
}

var kindNames = map[Kind]string{
	DynKind: "dyn", NullKind: "null_type", BoolKind: "bool", IntKind: "int", UintKind: "uint",
	DoubleKind: "double", StringKind: "string", BytesKind: "bytes", ListKind: "list", MapKind: "map",
	freeKind: "dyn", paramKind: "dyn",
}

// elemOf is a list's element or a map's value type; dyn when unset.
func elemOf(t Type) Type {
	if t.Elem == nil {
		return Dyn
	}
	return *t.Elem
}

// keyOf is a map's key type; dyn when unset.
func keyOf(t Type) Type {
	if t.Key == nil {
		return Dyn
	}
	return *t.Key
}

// isDynLike reports whether t is dyn, still free or a parameter: each
// takes any type.
func isDynLike(t Type) bool { return t.Kind == DynKind || t.Kind == freeKind || t.Kind == paramKind }

// assignable reports whether a value of type b can stand where a is
// expected, and the other way round: cel-go's isAssignable, which is
// symmetric for the types this package has. Dyn and free types match
// anything; null matches null, dyn and structs.
func assignable(a, b Type) bool {
	if isDynLike(a) || isDynLike(b) {
		return true
	}
	if a.Kind == NullKind {
		return b.Kind == NullKind || b.Kind == StructKind
	}
	if b.Kind == NullKind {
		return a.Kind == StructKind
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case ListKind:
		return assignable(elemOf(a), elemOf(b))
	case MapKind:
		return assignable(keyOf(a), keyOf(b)) && assignable(elemOf(a), elemOf(b))
	case StructKind:
		return a.Name == b.Name
	}
	return true
}

// lessSpecific is cel-go's isEqualOrLessSpecific: a is b, or b with dyn
// (or a free type) somewhere in its place.
func lessSpecific(a, b Type) bool {
	if isDynLike(a) {
		return true
	}
	if isDynLike(b) || a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case ListKind:
		return lessSpecific(elemOf(a), elemOf(b))
	case MapKind:
		return lessSpecific(keyOf(a), keyOf(b)) && lessSpecific(elemOf(a), elemOf(b))
	case StructKind:
		return a.Name == b.Name
	}
	return true
}

// mostGeneral is cel-go's mostGeneral: of two assignable types, a when
// it is the less specific, else b. The order matters where neither is
// less specific (null and a struct): b wins. A free type in the winner is
// then filled from the other, as cel-go binds a type parameter.
func mostGeneral(a, b Type) Type {
	if lessSpecific(a, b) {
		return fill(a, b)
	}
	return fill(b, a)
}

// fill is t with each free type in it replaced by the type in the same
// place in from, and each parameter by a dyn there. (Check refuses a
// parameter that would take any other type: binds.)
func fill(t, from Type) Type {
	switch {
	case t.Kind == freeKind || t.Kind == paramKind && from.Kind == DynKind:
		return from
	case t.Kind == ListKind && from.Kind == ListKind:
		if e := elemOf(t); hasVar(e) {
			return ListOf(fill(e, elemOf(from)))
		}
	case t.Kind == MapKind && from.Kind == MapKind:
		k, v := keyOf(t), elemOf(t)
		if hasVar(k) || hasVar(v) {
			return MapOf(fill(k, keyOf(from)), fill(v, elemOf(from)))
		}
	}
	return t
}

// hasVar reports whether t is or holds a free type or a parameter.
func hasVar(t Type) bool {
	switch t.Kind {
	case freeKind, paramKind:
		return true
	case ListKind:
		return hasVar(elemOf(t))
	case MapKind:
		return hasVar(keyOf(t)) || hasVar(elemOf(t))
	}
	return false
}

// binds reports whether joining a with b, two assignable types, would
// give a parameter in one of them the concrete type in the same place in
// the other, as cel-go does: `[x['k'], 'a']` is a list(string) there.
func binds(a, b Type) bool {
	switch {
	case a.Kind == paramKind:
		return !isDynLike(b)
	case b.Kind == paramKind:
		return !isDynLike(a)
	case a.Kind == ListKind && b.Kind == ListKind:
		return binds(elemOf(a), elemOf(b))
	case a.Kind == MapKind && b.Kind == MapKind:
		return binds(keyOf(a), keyOf(b)) || binds(elemOf(a), elemOf(b))
	}
	return false
}

// settle is t with every free type and parameter in it made dyn: a
// checked expression's types have none.
func settle(t Type) Type {
	if !hasVar(t) {
		return t
	}
	switch t.Kind {
	case ListKind:
		return ListOf(settle(elemOf(t)))
	case MapKind:
		return MapOf(settle(keyOf(t)), settle(elemOf(t)))
	}
	return Dyn
}
