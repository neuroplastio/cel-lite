package cellite

import "sort"

// The names CEL keeps for itself, for a program that makes declarations
// from names it doesn't choose, such as a table's keys. Each call
// returns a new slice, sorted.

// ReservedWords are the words CEL keeps from identifiers: no variable,
// struct or helper takes one as its name. A field may: `x.if`.
func ReservedWords() []string { return sortedKeys(reserved) }

// TypeNames are the names CEL gives its types: no struct or helper takes
// one as its name. A variable may, and then the name reads the variable,
// as in cel-go.
func TypeNames() []string { return sortedKeys(typeNames) }

// StandardFunctions are the functions and macros of CEL's standard
// library, the ones cel-lite implements and the ones it refuses by name:
// no helper takes one as its name.
func StandardFunctions() []string { return sortedKeys(standard) }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
