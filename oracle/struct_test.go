package oracle

import (
	"math"
	"math/rand/v2"
	"reflect"
	"strconv"
	"testing"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/ext"

	"github.com/neuroplastio/cel-lite"
)

// Item is a struct both engines read: cel-go as a native Go type, cellite
// through an Object over the same value.
type Item struct {
	Title  string            `cel:"title"`
	Count  int64             `cel:"count"`
	Tags   []string          `cel:"tags"`
	Labels map[string]string `cel:"labels"`
	Score  float64           `cel:"score"`
	Flag   bool              `cel:"flag"`
	Next   *Item             `cel:"next"`
}

// itemStruct is Item as cellite declares it, its fields in Item's order.
var itemStruct = cellite.Struct{Name: "Item", Fields: []cellite.Field{
	{Name: "title", Type: cellite.String},
	{Name: "count", Type: cellite.Int},
	{Name: "tags", Type: cellite.ListOf(cellite.String)},
	{Name: "labels", Type: cellite.MapOf(cellite.String, cellite.String)},
	{Name: "score", Type: cellite.Double},
	{Name: "flag", Type: cellite.Bool},
	{Name: "next", Type: cellite.StructOf("Item")},
}}

// liteItem is an *Item as cellite reads it.
type liteItem struct{ p *Item }

func (o liteItem) Field(i int) cellite.Value {
	switch i {
	case 0:
		return cellite.StringValue(o.p.Title)
	case 1:
		return cellite.IntValue(o.p.Count)
	case 2:
		return cellite.ValueOf(o.p.Tags)
	case 3:
		return cellite.ValueOf(o.p.Labels)
	case 4:
		return cellite.DoubleValue(o.p.Score)
	case 5:
		return cellite.BoolValue(o.p.Flag)
	case 6:
		// An unset struct field reads as the struct's zero value, as cel-go
		// reads a native type's nil pointer, and protobuf an unset message.
		if o.p.Next == nil {
			return cellite.ObjectValue("Item", liteItem{&Item{}})
		}
		return cellite.ObjectValue("Item", liteItem{o.p.Next})
	}
	return cellite.NullValue()
}

// liteItems is a []*Item as cellite reads it.
type liteItems []*Item

func (l liteItems) Len() int                 { return len(l) }
func (l liteItems) At(i int) cellite.Value   { return cellite.ObjectValue("Item", liteItem{l[i]}) }
func liteItemValue(p *Item) cellite.Value    { return cellite.ObjectValue("Item", liteItem{p}) }
func liteItemsValue(l []*Item) cellite.Value { return cellite.ListValue(liteItems(l)) }

// liteItemMap is a map[string]*Item as cellite reads it.
type liteItemMap map[string]*Item

func (m liteItemMap) Len() int { return len(m) }

func (m liteItemMap) Get(k cellite.Value) (cellite.Value, bool) {
	if k.Kind() != cellite.StringKind {
		return cellite.Value{}, false
	}
	p, ok := m[k.String()]
	if !ok {
		return cellite.Value{}, false
	}
	return liteItemValue(p), true
}

func (m liteItemMap) Range(f func(k, v cellite.Value) bool) {
	for k, p := range m {
		if !f(cellite.StringValue(k), liteItemValue(p)) {
			return
		}
	}
}

// toLiteItems is the struct world's toLite.
func toLiteItems(v any) any {
	switch v := v.(type) {
	case *Item:
		return liteItemValue(v)
	case []*Item:
		return liteItemsValue(v)
	case map[string]*Item:
		return cellite.MapValue(liteItemMap(v))
	}
	return v
}

// canonItem writes an Item as canonLite and canonCel write a map, so two
// engines' structs compare by their fields.
func canonItem(p *Item) any {
	if p == nil {
		return "null"
	}
	tags := make([]any, len(p.Tags))
	for i, t := range p.Tags {
		tags[i] = "string:" + t
	}
	labels := map[string]any{}
	for k, v := range p.Labels {
		labels["string:"+k] = "string:" + v
	}
	return map[string]any{
		"title": "string:" + p.Title, "count": "int:" + strconv.FormatInt(p.Count, 10), "tags": tags,
		"labels": labels, "score": canonDouble(p.Score), "flag": "bool:" + strconv.FormatBool(p.Flag),
		"next": canonItem(p.Next),
	}
}

// structDecls are the struct world's variables.
var structDecls = []decl{
	{"it", cellite.StructOf("Item")},
	{"its", cellite.ListOf(cellite.StructOf("Item"))},
	{"byName", cellite.MapOf(cellite.String, cellite.StructOf("Item"))},
	{"b", cellite.Bool}, {"s", cellite.String}, {"i", cellite.Int}, {"dy", cellite.Dyn},
}

// newStructWorld declares Item and structDecls to both engines.
func newStructWorld(t testing.TB) *world {
	t.Helper()
	w := &world{decls: structDecls, toLite: toLiteItems}
	var vars []cellite.Var
	opts := []cel.EnvOption{ext.NativeTypes(reflect.TypeFor[Item](), ext.ParseStructTags(true))}
	itemType := cel.ObjectType("oracle.Item")
	for _, d := range structDecls {
		vars = append(vars, cellite.Var{Name: d.name, Type: d.typ})
		typ := celType(d.typ)
		switch d.name {
		case "it":
			typ = itemType
		case "its":
			typ = cel.ListType(itemType)
		case "byName":
			typ = cel.MapType(cel.StringType, itemType)
		}
		opts = append(opts, cel.Variable(d.name, typ))
	}
	var err error
	if w.lite, err = cellite.NewEnv(cellite.Decls{Structs: []cellite.Struct{itemStruct}, Vars: vars}); err != nil {
		t.Fatal(err)
	}
	if w.cel, err = cel.NewEnv(opts...); err != nil {
		t.Fatal(err)
	}
	return w
}

// randItem is a random Item, with a Next of its own while depth lasts.
func randItem(r *rand.Rand, depth int) *Item {
	words := []string{"", "a", "k", "x", "é"}
	p := &Item{
		Title: words[r.IntN(len(words))],
		Count: []int64{0, 1, -1, 7, math.MaxInt64, math.MinInt64}[r.IntN(6)],
		Score: []float64{0, 0.5, -1.5, math.NaN(), math.Inf(1)}[r.IntN(5)],
		Flag:  r.IntN(2) == 0,
	}
	for range r.IntN(3) {
		p.Tags = append(p.Tags, words[r.IntN(len(words))])
	}
	if n := r.IntN(3); n > 0 {
		p.Labels = map[string]string{}
		for range n {
			p.Labels[words[r.IntN(len(words))]] = words[r.IntN(len(words))]
		}
	}
	if depth > 0 && r.IntN(3) != 0 {
		p.Next = randItem(r, depth-1)
	}
	return p
}

// structBinding is a random binding of structDecls: each variable present,
// absent or unknown, as the generator's are.
func structBinding(r *rand.Rand) binding {
	b := binding{vals: map[string]any{}}
	for _, d := range structDecls {
		switch x := r.Float64(); {
		case x < 0.07:
			continue
		case x < 0.15:
			b.unknown = append(b.unknown, d.name)
			continue
		}
		switch d.name {
		case "it":
			b.vals["it"] = randItem(r, 2)
		case "its":
			var l []*Item
			for range r.IntN(3) {
				l = append(l, randItem(r, 1))
			}
			b.vals["its"] = l
		case "byName":
			m := map[string]*Item{}
			for _, k := range []string{"a", "k"}[:r.IntN(3)%2+r.IntN(2)] {
				m[k] = randItem(r, 1)
			}
			b.vals["byName"] = m
		case "b":
			b.vals["b"] = r.IntN(2) == 0
		case "s":
			b.vals["s"] = []string{"", "a", "k"}[r.IntN(3)]
		case "i":
			b.vals["i"] = int64(r.IntN(3) - 1)
		case "dy":
			b.vals["dy"] = []any{randItem(r, 1), "a", int64(1), map[string]any{"title": "t"}}[r.IntN(4)]
		}
	}
	return b
}

// structCorpus are expressions on Item: fields of every type, structs
// nested, absent and in lists and maps, read through dyn, and what
// cellite refuses about structs.
var structCorpus = []string{
	`it`, `it.title`, `it.count + 1`, `it.tags`, `it.tags[0]`, `size(it.tags)`, `'a' in it.tags`, `it.tags + ['x']`,
	`it.labels`, `it.labels.k`, `it.labels['k']`, `has(it.labels.k)`, `'k' in it.labels`, `it.labels.k == s`,
	`it.score`, `it.score > 0.5`, `it.score == it.score`, `it.flag`, `!it.flag && it.count > 0`, `it.title.size()`,
	`it.title + s`, `it.count == i`, `it.count == dyn(i)`, `it.count * 2`,
	`it.next`, `it.next.title`, `it.next.next.count`, `it.next.next.next`, `it.next == null`, `null == it.next`,
	`it.next != null && it.next.flag`, `it == null`,
	`its`, `size(its)`, `its.size() > 0 && its[0].flag`, `its[0].title`, `its[i].count`, `its[0].next.title`,
	`byName`, `byName.a.title`, `byName['a'].count`, `'a' in byName`, `has(byName.a)`, `byName.k.next`,
	`b ? it : it.next`, `(b ? it : it.next).title`, `[it, it.next]`, `[it, it.next][1].title`, `{'x': it}.x.title`,
	`[it][0].labels.k`, `size([it, it])`,
	`dyn(it).title`, `dyn(it).nope`, `dyn(it)['title']`, `dy.title`, `dy.count + 1`, `has(dy.title)`, `dy.next`,
	`dyn(it).next.title`, `dyn(its)[0].title`,
	`it.nope`, `it.title.nope`, `it == it`, `it != it.next`, `has(it.title)`, `has(it.next)`, `Item{title: 'a'}`,
	`it in its`, `it.title in its`, `dyn(it) == dyn(it)`, `dyn(it) == null`, `dyn(it) in [dyn(it)]`, `dy == dy`,
	`dyn(it)['title']`,
}

// structExcluded are the corpus's expressions where cellite follows CEL's
// language definition and cel-go doesn't (doc.go, "Where cel-lite and
// cel-go differ"), by name, with why.
var structExcluded = map[string]string{
	"dyn(it)['title']": "cel-go indexes a struct by a field's name; in the definition [] reads a list or a map",
}

// TestOracleStructs compares structCorpus, each on random bindings of
// structDecls.
func TestOracleStructs(t *testing.T) {
	w := newStructWorld(t)
	r := rand.New(rand.NewPCG(9, 10))
	var bs []binding
	for range 12 {
		bs = append(bs, structBinding(r))
	}
	bs = append(bs, binding{vals: map[string]any{}})
	for _, src := range structCorpus {
		if structExcluded[src] != "" {
			continue
		}
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("%s\n\t%s", src, diff)
		}
	}
}
