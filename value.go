package cellite

import (
	"math"
	"strconv"
	"unicode/utf8"
)

// Value is a CEL value: what an activation gives, what a helper takes and
// returns, and what Eval gives. It is small and passed by value: making
// one from a string or a number, or reading one, doesn't allocate. The
// zero Value is null.
type Value struct {
	k Kind
	// n holds a bool (0 or 1), an int's or a uint's bits, or a double's
	// bits; for an error, its errCode.
	n uint64
	// s holds a string, bytes, a struct's name, or an error's or an
	// unknown's detail.
	s string
	// o holds a List, a Map or an Object, or a helper's error.
	o any
}

// errKind and unknownKind are the two Values an evaluation can end in
// besides a value. They never leave the package as Values: Eval gives an
// error instead.
const (
	errKind     Kind = 253
	unknownKind Kind = 254
)

// List is a list a program supplies without copying it into Values, such
// as a slice of its structs.
type List interface {
	Len() int
	// At is element i, 0 <= i < Len().
	At(i int) Value
}

// Map is a map a program supplies. Its keys are bools, ints, uints or
// strings.
type Map interface {
	Len() int
	// Get is the value at key, which is a Value of any kind: a map with
	// string keys is asked for an int, and says it doesn't hold it.
	Get(key Value) (Value, bool)
	// Range calls f on each entry until f returns false.
	Range(f func(k, v Value) bool)
}

// Object is a value of a struct the program declares (Decls.Structs).
type Object interface {
	// Field is field i, its index in the Struct's Fields: Check resolves
	// a field's name to it (Expr.Ref). For a field of a struct type that
	// isn't set, an Object of the struct's zero value reads as cel-go
	// reads a native type's nil pointer and protobuf an unset message:
	// its fields are their zero values, and it is never null. NullValue
	// makes it equal null, and selecting from it an error.
	Field(i int) Value
}

// NullValue is null.
func NullValue() Value { return Value{} }

// BoolValue is a bool.
func BoolValue(b bool) Value {
	if b {
		return Value{k: BoolKind, n: 1}
	}
	return Value{k: BoolKind}
}

// IntValue is an int.
func IntValue(i int64) Value { return Value{k: IntKind, n: uint64(i)} }

// UintValue is a uint.
func UintValue(u uint64) Value { return Value{k: UintKind, n: u} }

// DoubleValue is a double.
func DoubleValue(f float64) Value { return Value{k: DoubleKind, n: math.Float64bits(f)} }

// StringValue is a string.
func StringValue(s string) Value { return Value{k: StringKind, s: s} }

// BytesValue is bytes. The Value keeps its own copy.
func BytesValue(b []byte) Value { return Value{k: BytesKind, s: string(b)} }

// ListValue is a list.
func ListValue(l List) Value { return Value{k: ListKind, o: l} }

// MapValue is a map.
func MapValue(m Map) Value { return Value{k: MapKind, o: m} }

// ObjectValue is a value of the struct the program declares as name.
func ObjectValue(name string, o Object) Value { return Value{k: StructKind, s: name, o: o} }

// Kind is the value's kind.
func (v Value) Kind() Kind { return v.k }

// Bool is a bool's value; false for any other kind.
func (v Value) Bool() bool { return v.k == BoolKind && v.n != 0 }

// Int is an int's value; 0 for any other kind.
func (v Value) Int() int64 {
	if v.k != IntKind {
		return 0
	}
	return int64(v.n)
}

// Uint is a uint's value; 0 for any other kind.
func (v Value) Uint() uint64 {
	if v.k != UintKind {
		return 0
	}
	return v.n
}

// Double is a double's value; 0 for any other kind.
func (v Value) Double() float64 {
	if v.k != DoubleKind {
		return 0
	}
	return math.Float64frombits(v.n)
}

// String is a string's value; for any other kind, its kind in angle
// brackets ("<int>"), as reflect.Value's String is. Text gives CEL's
// string() of a value.
func (v Value) String() string {
	if v.k == StringKind {
		return v.s
	}
	return "<" + kindNames[v.k] + ">"
}

// Bytes is bytes' value, a copy; nil for any other kind.
func (v Value) Bytes() []byte {
	if v.k != BytesKind {
		return nil
	}
	return []byte(v.s)
}

// List is a list's value; nil for any other kind.
func (v Value) List() List {
	if v.k != ListKind {
		return nil
	}
	return v.o.(List)
}

// Map is a map's value; nil for any other kind.
func (v Value) Map() Map {
	if v.k != MapKind {
		return nil
	}
	return v.o.(Map)
}

// Object is a struct value's Object, and StructName its struct's name;
// nil and "" for any other kind.
func (v Value) Object() Object {
	if v.k != StructKind {
		return nil
	}
	return v.o.(Object)
}

// StructName is the name of a struct value's struct; "" for any other
// kind.
func (v Value) StructName() string {
	if v.k != StructKind {
		return ""
	}
	return v.s
}

// Interface is the value as Go has it: nil, bool, int64, uint64,
// float64, string or []byte; a list ValueOf made from a slice is that
// slice, any other list a []any; a map ValueOf made from a Go map is that
// map, a map literal a map[string]any when its keys are all strings and a
// map[any]any when not, any other Map itself; a struct value its Object.
func (v Value) Interface() any {
	switch v.k {
	case NullKind:
		return nil
	case BoolKind:
		return v.n != 0
	case IntKind:
		return int64(v.n)
	case UintKind:
		return v.n
	case DoubleKind:
		return math.Float64frombits(v.n)
	case StringKind:
		return v.s
	case BytesKind:
		return []byte(v.s)
	case ListKind:
		switch l := v.o.(type) {
		case anyList:
			return []any(l)
		case stringList:
			return []string(l)
		case int64List:
			return []int64(l)
		case intList:
			return []int(l)
		case doubleList:
			return []float64(l)
		case boolList:
			return []bool(l)
		}
		l := v.o.(List)
		out := make([]any, l.Len())
		for i := range out {
			out[i] = l.At(i).Interface()
		}
		return out
	case MapKind:
		switch m := v.o.(type) {
		case anyMap:
			return map[string]any(m)
		case stringMap:
			return map[string]string(m)
		case int64Map:
			return map[string]int64(m)
		case intMap:
			return map[string]int(m)
		case boolMap:
			return map[string]bool(m)
		case doubleMap:
			return map[string]float64(m)
		case *valueMap:
			strs := true
			for _, k := range m.keys {
				strs = strs && k.k == StringKind
			}
			if strs {
				out := make(map[string]any, len(m.keys))
				for i, k := range m.keys {
					out[k.s] = m.vals[i].Interface()
				}
				return out
			}
			out := make(map[any]any, len(m.keys))
			for i, k := range m.keys {
				out[k.Interface()] = m.vals[i].Interface()
			}
			return out
		}
		return v.o
	case StructKind:
		return v.o
	}
	return nil
}

// ValueOf is a Go value as CEL reads it, without copying: nil; a bool; a
// string or []byte; a Go integer of any size, as an int if signed and a
// uint if not; a float32 or float64; a Value; a List, a Map; a slice of
// any, string, int64, int, float64, bool or Value; a map from string to
// any, string, int64, int, bool, float64 or Value. Anything else is
// reported, by the evaluation that reads it, as an error: a struct value
// goes in through ObjectValue.
func ValueOf(x any) Value {
	switch v := x.(type) {
	case nil:
		return Value{}
	case Value:
		return v
	case bool:
		return BoolValue(v)
	case string:
		return Value{k: StringKind, s: v}
	case int64:
		return IntValue(v)
	case int:
		return IntValue(int64(v))
	case int32:
		return IntValue(int64(v))
	case int16:
		return IntValue(int64(v))
	case int8:
		return IntValue(int64(v))
	case uint64:
		return UintValue(v)
	case uint:
		return UintValue(uint64(v))
	case uint32:
		return UintValue(uint64(v))
	case uint16:
		return UintValue(uint64(v))
	case uint8:
		return UintValue(uint64(v))
	case float64:
		return DoubleValue(v)
	case float32:
		return DoubleValue(float64(v))
	case []byte:
		return BytesValue(v)
	case List:
		return Value{k: ListKind, o: v}
	case Map:
		return Value{k: MapKind, o: v}
	case []any:
		return Value{k: ListKind, o: anyList(v)}
	case []string:
		return Value{k: ListKind, o: stringList(v)}
	case []int64:
		return Value{k: ListKind, o: int64List(v)}
	case []int:
		return Value{k: ListKind, o: intList(v)}
	case []float64:
		return Value{k: ListKind, o: doubleList(v)}
	case []bool:
		return Value{k: ListKind, o: boolList(v)}
	case []Value:
		return Value{k: ListKind, o: valueList(v)}
	case map[string]any:
		return Value{k: MapKind, o: anyMap(v)}
	case map[string]string:
		return Value{k: MapKind, o: stringMap(v)}
	case map[string]int64:
		return Value{k: MapKind, o: int64Map(v)}
	case map[string]int:
		return Value{k: MapKind, o: intMap(v)}
	case map[string]bool:
		return Value{k: MapKind, o: boolMap(v)}
	case map[string]float64:
		return Value{k: MapKind, o: doubleMap(v)}
	case map[string]Value:
		return Value{k: MapKind, o: valueStrMap(v)}
	}
	return errValue(eBadValue, typeName(x))
}

// typeName names a Go value's type for an error, without reflection.
func typeName(x any) string {
	switch x.(type) {
	case Object:
		return "an Object without its struct's name (use ObjectValue)"
	}
	return "a Go value of a type it can't read"
}

// The slices and maps ValueOf reads, as Lists and Maps.
type (
	anyList     []any
	stringList  []string
	int64List   []int64
	intList     []int
	doubleList  []float64
	boolList    []bool
	valueList   []Value
	anyMap      map[string]any
	stringMap   map[string]string
	int64Map    map[string]int64
	intMap      map[string]int
	boolMap     map[string]bool
	doubleMap   map[string]float64
	valueStrMap map[string]Value
)

func (l anyList) Len() int          { return len(l) }
func (l anyList) At(i int) Value    { return ValueOf(l[i]) }
func (l stringList) Len() int       { return len(l) }
func (l stringList) At(i int) Value { return StringValue(l[i]) }
func (l int64List) Len() int        { return len(l) }
func (l int64List) At(i int) Value  { return IntValue(l[i]) }
func (l intList) Len() int          { return len(l) }
func (l intList) At(i int) Value    { return IntValue(int64(l[i])) }
func (l doubleList) Len() int       { return len(l) }
func (l doubleList) At(i int) Value { return DoubleValue(l[i]) }
func (l boolList) Len() int         { return len(l) }
func (l boolList) At(i int) Value   { return BoolValue(l[i]) }
func (l valueList) Len() int        { return len(l) }
func (l valueList) At(i int) Value  { return l[i] }

func (m anyMap) Len() int { return len(m) }
func (m anyMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return ValueOf(v), ok
}
func (m anyMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), ValueOf(v)) {
			return
		}
	}
}

func (m stringMap) Len() int { return len(m) }
func (m stringMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return StringValue(v), ok
}
func (m stringMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), StringValue(v)) {
			return
		}
	}
}

func (m int64Map) Len() int { return len(m) }
func (m int64Map) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return IntValue(v), ok
}
func (m int64Map) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), IntValue(v)) {
			return
		}
	}
}

func (m intMap) Len() int { return len(m) }
func (m intMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return IntValue(int64(v)), ok
}
func (m intMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), IntValue(int64(v))) {
			return
		}
	}
}

func (m boolMap) Len() int { return len(m) }
func (m boolMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return BoolValue(v), ok
}
func (m boolMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), BoolValue(v)) {
			return
		}
	}
}

func (m doubleMap) Len() int { return len(m) }
func (m doubleMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return DoubleValue(v), ok
}
func (m doubleMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), DoubleValue(v)) {
			return
		}
	}
}

func (m valueStrMap) Len() int { return len(m) }
func (m valueStrMap) Get(k Value) (Value, bool) {
	if k.k != StringKind {
		return Value{}, false
	}
	v, ok := m[k.s]
	return v, ok
}
func (m valueStrMap) Range(f func(k, v Value) bool) {
	for k, v := range m {
		if !f(StringValue(k), v) {
			return
		}
	}
}

// valueMap is a map literal's value: its entries in order, a later one
// with the same key replacing an earlier one, as cel-go's map literal
// does. Keys are the same when they are the same kind and value, so 1 and
// 1u are two keys; a lookup then also tries the key's numeric equals.
type valueMap struct {
	keys, vals []Value
	index      map[mapKey]int
}

// mapKey is a key as valueMap indexes it.
type mapKey struct {
	k Kind
	n uint64
	s string
}

func keyOfValue(v Value) mapKey {
	if v.k == DoubleKind && v.n == 1<<63 { // -0.0 is 0.0
		return mapKey{k: DoubleKind}
	}
	return mapKey{k: v.k, n: v.n, s: v.s}
}

func newValueMap(n int) *valueMap {
	return &valueMap{keys: make([]Value, 0, n), vals: make([]Value, 0, n), index: make(map[mapKey]int, n)}
}

//go:noinline
func (m *valueMap) put(k, v Value) {
	mk := keyOfValue(k)
	if i, ok := m.index[mk]; ok {
		m.vals[i] = v
		return
	}
	m.index[mk] = len(m.keys)
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, v)
}

func (m *valueMap) Len() int { return len(m.keys) }
func (m *valueMap) Get(k Value) (Value, bool) {
	if i, ok := m.index[keyOfValue(k)]; ok {
		return m.vals[i], true
	}
	return Value{}, false
}
func (m *valueMap) Range(f func(k, v Value) bool) {
	for i, k := range m.keys {
		if !f(k, m.vals[i]) {
			return
		}
	}
}

// mapGet looks key up in m as CEL does: the key itself, then, for a
// number, the int, uint or double equal to it.
//
//go:noinline
func mapGet(m Map, key Value) (Value, bool) {
	if v, ok := m.Get(key); ok {
		return v, true
	}
	switch key.k {
	case IntKind:
		if i := int64(key.n); i >= 0 {
			return m.Get(UintValue(uint64(i)))
		}
	case UintKind:
		if key.n <= math.MaxInt64 {
			return m.Get(IntValue(int64(key.n)))
		}
	case DoubleKind:
		f := math.Float64frombits(key.n)
		if i, ok := doubleToIntLossless(f); ok {
			if v, ok := m.Get(IntValue(i)); ok {
				return v, true
			}
		}
		if u, ok := doubleToUintLossless(f); ok {
			return m.Get(UintValue(u))
		}
	}
	return Value{}, false
}

func doubleToIntLossless(f float64) (int64, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) || f <= math.MinInt64 || f >= math.MaxInt64 {
		return 0, false
	}
	i := int64(f)
	return i, float64(i) == f
}

func doubleToUintLossless(f float64) (uint64, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) || f < 0 || f >= twoTo64 {
		return 0, false
	}
	u := uint64(f)
	return u, float64(u) == f
}

var twoTo64 = math.Ldexp(1, 64)

// Text is CEL's string() of a value: a string as it is; an int, a uint
// and a bool written out; a double in Go's %g; bytes that are valid
// UTF-8 as that text. Any other value is an error.
func Text(v Value) (string, error) {
	r := toString(v)
	if r.k == errKind {
		return "", errorOf(r)
	}
	return r.s, nil
}

// toString is string(v) as an evaluation gives it.
func toString(v Value) Value {
	switch v.k {
	case StringKind:
		return v
	case IntKind:
		return StringValue(strconv.FormatInt(int64(v.n), 10))
	case UintKind:
		return StringValue(strconv.FormatUint(v.n, 10))
	case DoubleKind:
		return StringValue(strconv.FormatFloat(math.Float64frombits(v.n), 'g', -1, 64))
	case BoolKind:
		return StringValue(strconv.FormatBool(v.n != 0))
	case BytesKind:
		if !utf8.ValidString(v.s) {
			return errValue(eMessage, "invalid UTF-8 in bytes, cannot convert to string")
		}
		return StringValue(v.s)
	}
	return errValue(eMessage, "type conversion error from '"+kindNames[v.k]+"' to 'string'")
}
