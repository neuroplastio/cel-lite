//go:build cellitestack && tinygo

package cellite

import _ "unsafe" // for go:linkname

// stackLow is the lowest stack pointer probe has seen since it was last
// reset.
var stackLow uintptr

// probe records how deep the goroutine's stack is here.
//
//go:noinline
func probe() {
	if sp := stackHere(); stackLow == 0 || sp < stackLow {
		stackLow = sp
	}
}

// stackHere is TinyGo's stack pointer: on wasm, the goroutine's shadow
// stack, where its frames keep what doesn't fit in wasm's own locals.
//
//go:linkname stackHere tinygo_getCurrentStackPointer
func stackHere() uintptr
