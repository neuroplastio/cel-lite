package oracle

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/neuroplastio/cel-lite"
)

// keymapFiles are where keymaps with CEL contexts live, in the checkouts
// beside this one (neuroplastio/<repo>/main). They are read as data when
// the test runs; nothing of them is kept here.
var keymapFiles = []string{
	"kubecom/main/internal/tui/keymap/*.kdl",
	"plx/main/cmd/plx/internal/keybind/defaults/*.kdl",
	"plx/main/cmd/plx/internal/tui/leader/testdata/*.kdl",
}

// contextRe finds a KDL node's context or when string.
var contextRe = regexp.MustCompile(`\b(?:context|when)\s+"((?:[^"\\]|\\.)*)"`)

// TestOracleKeymaps compares the two engines on the contexts of real
// keymaps, each on random bindings of the variables it names. The
// variables' types are read from how the expressions use them: compared
// with a string, a string; indexed or after `in`, a map of strings; else
// a bool.
func TestOracleKeymaps(t *testing.T) {
	var files []string
	for _, pat := range keymapFiles {
		m, _ := filepath.Glob(filepath.Join("..", "..", "..", pat))
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Skip("no keymaps to read: the test reads kubecom's and plx's .kdl keymaps " +
			"from their checkouts beside this one (../../../<repo>/main), which this clone hasn't")
	}
	seen := map[string]bool{}
	var srcs []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range contextRe.FindAllStringSubmatch(string(data), -1) {
			src, err := strconv.Unquote(`"` + m[1] + `"`)
			if err != nil || seen[src] {
				continue
			}
			seen[src] = true
			srcs = append(srcs, src)
		}
	}
	sort.Strings(srcs)
	types, words := inferSchema(t, srcs)
	var decls []decl
	for name, typ := range types {
		decls = append(decls, decl{name, typ})
	}
	sort.Slice(decls, func(i, j int) bool { return decls[i].name < decls[j].name })
	w := newWorld(t, decls)
	r := rand.New(rand.NewPCG(5, 6))
	for _, src := range srcs {
		var bs []binding
		for range 32 {
			bs = append(bs, keymapBinding(r, decls, words))
		}
		if diff := w.compare(src, bs); diff != "" {
			t.Errorf("%s\n\t%s", src, diff)
		}
	}
	t.Logf("%d contexts from %d files, %d variables", len(srcs), len(files), len(decls))
}

// inferSchema reads the variables the expressions name and their types,
// and the string literals they compare with.
func inferSchema(t *testing.T, srcs []string) (map[string]cellite.Type, []string) {
	types := map[string]cellite.Type{}
	words := map[string]bool{"x": true}
	declare := func(name string, typ cellite.Type) {
		if old, ok := types[name]; !ok || old.Kind == cellite.BoolKind {
			types[name] = typ
		}
	}
	strs := cellite.MapOf(cellite.String, cellite.String)
	var walk func(x, parent *cellite.Expr)
	walk = func(x, parent *cellite.Expr) {
		if x == nil {
			return
		}
		if x.Op == cellite.OpLit {
			if s, ok := x.Value.(string); ok {
				words[s] = true
			}
			return
		}
		if name, ok := qualifiedName(x); ok {
			typ := cellite.Bool
			switch {
			case parent == nil:
			case parent.Op == cellite.OpIndex && parent.Args[0] == x,
				parent.Op == cellite.OpIn && parent.Args[1] == x:
				typ = strs
			case parent.Op == cellite.OpEq || parent.Op == cellite.OpNe || parent.Op == cellite.OpIn:
				typ = cellite.String
			}
			declare(name, typ)
			return
		}
		for _, a := range x.Args {
			walk(a, x)
		}
	}
	for _, src := range srcs {
		x, err := cellite.Parse(src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		walk(x, nil)
	}
	var ws []string
	for w := range words {
		ws = append(ws, w)
	}
	sort.Strings(ws)
	return types, ws
}

// qualifiedName is the dotted name an identifier, or selections on one,
// spell.
func qualifiedName(x *cellite.Expr) (string, bool) {
	switch x.Op {
	case cellite.OpIdent:
		return x.Name, true
	case cellite.OpSelect:
		if q, ok := qualifiedName(x.Args[0]); ok {
			return q + "." + x.Name, true
		}
	}
	return "", false
}

// keymapBinding is a random binding: some variables absent, some
// unknown, strings and keys from the words the contexts use.
func keymapBinding(r *rand.Rand, decls []decl, words []string) binding {
	b := binding{vals: map[string]any{}}
	word := func() string { return words[r.IntN(len(words))] }
	for _, d := range decls {
		switch x := r.Float64(); {
		case x < 0.1:
			continue
		case x < 0.2:
			b.unknown = append(b.unknown, d.name)
			continue
		}
		switch d.typ.Kind {
		case cellite.BoolKind:
			b.vals[d.name] = r.IntN(2) == 0
		case cellite.StringKind:
			b.vals[d.name] = word()
		case cellite.MapKind:
			m := map[string]any{}
			for range r.IntN(3) {
				m[word()] = word()
			}
			b.vals[d.name] = m
		}
	}
	return b
}
