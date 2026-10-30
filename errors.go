package cellite

import (
	"errors"
	"strconv"
)

// ErrorKind says why an expression was refused.
type ErrorKind uint8

const (
	// SyntaxError: the source doesn't parse as CEL.
	SyntaxError ErrorKind = iota + 1
	// Undeclared: a name the Env doesn't declare, a variable or a
	// function. Error.Name is the name.
	Undeclared
	// TypeError: the types don't fit, such as `'a' + 1`, or a context
	// that must be a bool and isn't.
	TypeError
	// Unsupported: valid CEL that this package doesn't implement, refused
	// by name so that an expression it accepts means what it means in
	// CEL. Error.Name is the feature: "macro all", "function matches".
	Unsupported
)

// Error is why Parse, Check or Compile refused an expression.
type Error struct {
	Kind ErrorKind
	// Pos is the byte offset in the source; Line and Col are where that
	// is, from 1, Col counted in characters.
	Pos, Line, Col int
	// Name is the undeclared name, or the unsupported feature.
	Name string
	Msg  string
}

// Error is the message with its position: "1:7: undeclared reference to
// 'x'", or "offset 6: …" from Check, which has no source.
func (e *Error) Error() string {
	if e.Line == 0 {
		return "offset " + strconv.Itoa(e.Pos) + ": " + e.Msg
	}
	return strconv.Itoa(e.Line) + ":" + strconv.Itoa(e.Col) + ": " + e.Msg
}

// newError is an Error at pos in src.
func newError(src string, pos int, kind ErrorKind, name, msg string) *Error {
	line, col := 1, 1
	if pos > len(src) {
		pos = len(src)
	}
	for _, c := range src[:pos] {
		if c == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return &Error{Kind: kind, Pos: pos, Line: line, Col: col, Name: name, Msg: msg}
}

// ErrUnknown is what Eval gives when the value depends on a variable the
// activation says is unknown (Unknown): partial evaluation.
var ErrUnknown = errors.New("cellite: unknown")

// EvalError is an error an expression met while it ran: a missing key, a
// division by zero, an overflow, or a helper's own error (Err).
type EvalError struct {
	Msg string
	// Err is the helper's error, when a helper failed.
	Err error
}

func (e *EvalError) Error() string { return e.Msg }

// Unwrap is the helper's error.
func (e *EvalError) Unwrap() error { return e.Err }
