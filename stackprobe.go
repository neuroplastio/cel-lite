//go:build !(cellitestack && tinygo)

package cellite

// probe marks the deepest points of the parser's, the checker's, the
// planner's and the evaluator's recursion. It does nothing, and costs
// nothing, unless the package is built with the cellitestack tag, whose
// test measures the stack each of them needs under TinyGo (stack_test.go).
func probe() {}
