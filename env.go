package cellite

import (
	"errors"
	"strconv"
	"strings"
)

// Decls is what a program declares to its expressions: variables, the
// structs their values may be, and helper functions.
type Decls struct {
	// Vars are the variables an expression may read, in the order an
	// Activation indexes them.
	Vars []Var
	// Structs are the struct types the variables' values may be or hold.
	Structs []Struct
	// Funcs are the helpers an expression may call, beside CEL's
	// standard functions.
	Funcs []Func
}

// Var is a variable. Its name may be qualified, `pane.id`: an expression
// that reads `pane.id` reads this variable, even where `pane` is another.
type Var struct {
	Name string
	Type Type
}

// Struct is a struct type. A value of it is an Object, whose Field(i)
// gives Fields[i].
type Struct struct {
	Name   string
	Fields []Field
}

// Field is a struct's field.
type Field struct {
	Name string
	Type Type
}

// Func is a helper function, with one or more overloads. A call is
// checked against them when an expression is compiled.
type Func struct {
	Name      string
	Overloads []Overload
}

// Overload is one signature of a helper and the Go function that runs
// it.
type Overload struct {
	// ID names the overload in a checked expression (Expr.Overload), for
	// a program that generates code from it. It is unique in the Env.
	ID string
	// Member makes it a method: `x.f(y)`, with the receiver as Args[0].
	Member bool
	// Args are the parameters' types, and Result the type it returns.
	// Dyn takes, or gives, a value of any type.
	Args   []Type
	Result Type
	// Fn runs it. Its arguments are the values, not errors or unknowns,
	// which a call never passes on. An error it returns is the
	// expression's (EvalError.Err).
	Fn func(args []Value) (Value, error)
}

// Env is the environment expressions are checked and compiled in: a
// program's Decls, read. It is safe for concurrent use.
type Env struct {
	vars    []Var
	varIdx  map[string]int
	structs map[string]*structInfo
	funcs   map[string]*Func
}

// structInfo is a declared struct with its fields' indexes.
type structInfo struct {
	Struct
	fields map[string]int
}

// typeNames are the names CEL gives its types: an identifier that is one
// is a type value, which this package doesn't implement, unless a
// variable takes the name.
var typeNames = map[string]bool{
	"bool": true, "bytes": true, "double": true, "dyn": true, "int": true, "list": true, "map": true,
	"null_type": true, "string": true, "type": true, "uint": true,
	"google.protobuf.Timestamp": true, "google.protobuf.Duration": true,
}

// standard are the functions of CEL's standard library: the ones this
// package implements, and the ones it refuses by name. A helper can't
// take any of their names, nor a macro's.
var standard = map[string]bool{
	"size": true, "contains": true, "startsWith": true, "endsWith": true,
	"int": true, "uint": true, "double": true, "string": true, "bytes": true, "bool": true, "dyn": true,
	"has": true, "matches": true, "type": true, "timestamp": true, "duration": true,
	"getFullYear": true, "getMonth": true, "getDayOfYear": true, "getDayOfMonth": true, "getDate": true,
	"getDayOfWeek": true, "getHours": true, "getMinutes": true, "getSeconds": true, "getMilliseconds": true,
	"all": true, "exists": true, "exists_one": true, "map": true, "filter": true,
}

// NewEnv reads a program's declarations. A name that isn't an identifier
// (a variable's may be qualified), a reserved word, a struct or helper
// named as a type, a name declared twice, a type that names an undeclared
// struct, a helper named as a standard function, and an overload ID used
// twice are errors. A variable may take a type's name, as in cel-go: the
// variable wins, so with `list` declared, `list` reads it.
func NewEnv(d Decls) (*Env, error) {
	e := &Env{varIdx: map[string]int{}, structs: map[string]*structInfo{}, funcs: map[string]*Func{}}
	for _, s := range d.Structs {
		if !isIdent(s.Name) || reserved[s.Name] || typeNames[s.Name] {
			return nil, declError("struct", s.Name, "not an identifier, or reserved")
		}
		if e.structs[s.Name] != nil {
			return nil, declError("struct", s.Name, "declared twice")
		}
		info := &structInfo{Struct: s, fields: make(map[string]int, len(s.Fields))}
		for i, f := range s.Fields {
			if !isIdent(f.Name) {
				return nil, declError("struct "+s.Name+" field", f.Name, "not an identifier")
			}
			if _, dup := info.fields[f.Name]; dup {
				return nil, declError("struct "+s.Name+" field", f.Name, "declared twice")
			}
			info.fields[f.Name] = i
		}
		e.structs[s.Name] = info
	}
	for _, s := range d.Structs {
		for _, f := range s.Fields {
			if err := e.checkType(f.Type); err != nil {
				return nil, declError("struct "+s.Name+" field", f.Name, err.Error())
			}
			if f.Type.Kind == NullKind {
				return nil, declError("struct "+s.Name+" field", f.Name, "its type is null_type: declare dyn as Dyn")
			}
		}
	}
	for _, v := range d.Vars {
		first, _, _ := strings.Cut(v.Name, ".")
		for _, part := range strings.Split(v.Name, ".") {
			if !isIdent(part) {
				return nil, declError("variable", v.Name, "not a (qualified) identifier")
			}
		}
		if reserved[first] {
			return nil, declError("variable", v.Name, "the name is reserved")
		}
		if _, dup := e.varIdx[v.Name]; dup {
			return nil, declError("variable", v.Name, "declared twice")
		}
		if err := e.checkType(v.Type); err != nil {
			return nil, declError("variable", v.Name, err.Error())
		}
		if v.Type.Kind == NullKind {
			return nil, declError("variable", v.Name, "its type is null_type: declare dyn as Dyn")
		}
		e.varIdx[v.Name] = len(e.vars)
		e.vars = append(e.vars, v)
	}
	ids := map[string]bool{}
	for i := range d.Funcs {
		f := &d.Funcs[i]
		if !isIdent(f.Name) || reserved[f.Name] || typeNames[f.Name] || standard[f.Name] {
			return nil, declError("function", f.Name, "not an identifier, or a standard function's name")
		}
		if e.funcs[f.Name] != nil {
			return nil, declError("function", f.Name, "declared twice")
		}
		if len(f.Overloads) == 0 {
			return nil, declError("function", f.Name, "has no overloads")
		}
		for _, o := range f.Overloads {
			if o.ID == "" || ids[o.ID] {
				return nil, declError("function "+f.Name+" overload", o.ID, "empty or used twice")
			}
			ids[o.ID] = true
			if o.Fn == nil {
				return nil, declError("function "+f.Name+" overload", o.ID, "has no Fn")
			}
			if o.Member && len(o.Args) == 0 {
				return nil, declError("function "+f.Name+" overload", o.ID, "a method needs its receiver as Args[0]")
			}
			for _, t := range append(append([]Type(nil), o.Args...), o.Result) {
				if err := e.checkType(t); err != nil {
					return nil, declError("function "+f.Name+" overload", o.ID, err.Error())
				}
			}
		}
		e.funcs[f.Name] = f
	}
	return e, nil
}

// declError is an error in a declaration.
func declError(what, name, why string) error {
	return errors.New("cellite: " + what + " " + strconv.Quote(name) + ": " + why)
}

// checkType reports a type that names an undeclared struct, or a kind
// this package doesn't have.
func (e *Env) checkType(t Type) error {
	switch t.Kind {
	case DynKind, NullKind, BoolKind, IntKind, UintKind, DoubleKind, StringKind, BytesKind:
		return nil
	case ListKind:
		return e.checkType(elemOf(t))
	case MapKind:
		k := keyOf(t)
		switch k.Kind {
		case DynKind, BoolKind, IntKind, UintKind, StringKind:
		default:
			return errors.New("a map's key is a bool, an int, a uint, a string or dyn, not " + FormatType(k))
		}
		return e.checkType(elemOf(t))
	case StructKind:
		if e.structs[t.Name] == nil {
			return errors.New("struct " + strconv.Quote(t.Name) + " is not declared")
		}
		return nil
	}
	return errors.New("not a type")
}

// isIdent reports whether s is a CEL identifier.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isLetter(c) || c == '_' || i > 0 && isDigit(c)) {
			return false
		}
	}
	return true
}

// Vars are the variables, in the order declared: Var(i) of an Activation
// is Vars()[i]. The slice is the Env's own; don't change it.
func (e *Env) Vars() []Var { return e.vars }

// VarIndex is the index of the variable declared as name.
func (e *Env) VarIndex(name string) (int, bool) {
	i, ok := e.varIdx[name]
	return i, ok
}

// Presence says whether an activation has a variable's value.
type Presence uint8

const (
	// Present: the value is given.
	Present Presence = iota
	// Absent: reading the variable is an error, as reading an attribute
	// that isn't there is in CEL.
	Absent
	// Unknown: the value isn't known yet. An expression that depends on
	// it is unknown; one that doesn't, such as `false && x`, is not.
	Unknown
)

// Activation gives an evaluation its variables' values, by their index
// in the Env (Env.Vars).
type Activation interface {
	// Var is variable i's value, or that it is Absent or Unknown.
	Var(i int) (Value, Presence)
}

// Activation is an activation from a map of the variables' values by
// name, each read by ValueOf: a variable the map doesn't hold is absent,
// and one named in unknown is unknown.
func (e *Env) Activation(vals map[string]any, unknown ...string) Activation {
	a := &mapActivation{vals: make([]Value, len(e.vars)), has: make([]Presence, len(e.vars))}
	for i := range a.has {
		a.has[i] = Absent
	}
	for name, v := range vals {
		if i, ok := e.varIdx[name]; ok {
			a.vals[i], a.has[i] = ValueOf(v), Present
		}
	}
	for _, name := range unknown {
		if i, ok := e.varIdx[name]; ok {
			a.has[i] = Unknown
		}
	}
	return a
}

type mapActivation struct {
	vals []Value
	has  []Presence
}

func (a *mapActivation) Var(i int) (Value, Presence) { return a.vals[i], a.has[i] }
