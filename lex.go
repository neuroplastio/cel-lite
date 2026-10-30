package cellite

// tokKind is a token's kind, as CEL's grammar (CEL.g4) names them.
type tokKind uint8

const (
	tEOF tokKind = iota
	tIdent
	tInt    // NUM_INT, unsigned: a sign is the parser's
	tUint   // NUM_UINT
	tDouble // NUM_FLOAT, unsigned
	tString
	tBytes
	tTrue
	tFalse
	tNull
	tIn
	tEscIdent // `a.b`: refused
	tEq
	tNe
	tLt
	tLe
	tGt
	tGe
	tAnd
	tOr
	tLBracket
	tRBracket
	tLBrace
	tRBrace
	tLParen
	tRParen
	tDot
	tComma
	tMinus
	tNot
	tQuestion
	tColon
	tPlus
	tStar
	tSlash
	tPercent
	tBad // a character no token starts with
)

// token is a token: its kind and its source text, at byte offset pos.
type token struct {
	kind tokKind
	pos  int
	text string
}

// tokenDesc names a token kind in a syntax error.
var tokenDesc = [...]string{
	tEOF: "end of input", tIdent: "identifier", tInt: "number", tUint: "number", tDouble: "number",
	tString: "string", tBytes: "bytes", tTrue: "'true'", tFalse: "'false'", tNull: "'null'", tIn: "'in'",
	tEscIdent: "'`'", tEq: "'=='", tNe: "'!='", tLt: "'<'", tLe: "'<='", tGt: "'>'", tGe: "'>='",
	tAnd: "'&&'", tOr: "'||'", tLBracket: "'['", tRBracket: "']'", tLBrace: "'{'", tRBrace: "'}'",
	tLParen: "'('", tRParen: "')'", tDot: "'.'", tComma: "','", tMinus: "'-'", tNot: "'!'",
	tQuestion: "'?'", tColon: "':'", tPlus: "'+'", tStar: "'*'", tSlash: "'/'", tPercent: "'%'",
	tBad: "character",
}

// lex splits src into tokens, ending with tEOF. A token CEL can't read
// (an unterminated string, a stray character) ends the list with tBad at
// its position.
func lex(src string) []token {
	var out []token
	i := 0
	for {
		// White space and comments, as CEL.g4's WHITESPACE and COMMENT.
		for i < len(src) {
			c := src[i]
			if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' {
				i++
			} else if c == '/' && i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
			} else {
				break
			}
		}
		if i >= len(src) {
			return append(out, token{kind: tEOF, pos: i})
		}
		start := i
		c := src[i]
		kind := tBad
		switch {
		case isLetter(c) || c == '_':
			if n := stringPrefix(src[i:]); n > 0 {
				end, ok := scanString(src, i+n)
				if !ok {
					return append(out, token{kind: tBad, pos: start, text: src[start:]})
				}
				kind, i = tString, end
				if c == 'b' || c == 'B' {
					kind = tBytes
				}
				break
			}
			for i < len(src) && (isLetter(src[i]) || isDigit(src[i]) || src[i] == '_') {
				i++
			}
			kind = tIdent
			switch src[start:i] {
			case "true":
				kind = tTrue
			case "false":
				kind = tFalse
			case "null":
				kind = tNull
			case "in":
				kind = tIn
			}
		case isDigit(c) || c == '.' && i+1 < len(src) && isDigit(src[i+1]):
			kind, i = scanNumber(src, i)
		case c == '"' || c == '\'':
			end, ok := scanString(src, i)
			if !ok {
				return append(out, token{kind: tBad, pos: start, text: src[start:]})
			}
			kind, i = tString, end
		case c == '`':
			j := i + 1
			for j < len(src) && (isLetter(src[j]) || isDigit(src[j]) || src[j] == '_' || src[j] == '.' ||
				src[j] == '-' || src[j] == '/' || src[j] == ' ') {
				j++
			}
			if j > i+1 && j < len(src) && src[j] == '`' {
				kind, i = tEscIdent, j+1
			} else {
				i++
			}
		default:
			kind, i = punct(src, i)
		}
		out = append(out, token{kind: kind, pos: start, text: src[start:i]})
		if kind == tBad {
			return out
		}
	}
}

// punct reads an operator or a bracket at src[i].
func punct(src string, i int) (tokKind, int) {
	two := ""
	if i+1 < len(src) {
		two = src[i : i+2]
	}
	switch two {
	case "==":
		return tEq, i + 2
	case "!=":
		return tNe, i + 2
	case "<=":
		return tLe, i + 2
	case ">=":
		return tGe, i + 2
	case "&&":
		return tAnd, i + 2
	case "||":
		return tOr, i + 2
	}
	switch src[i] {
	case '<':
		return tLt, i + 1
	case '>':
		return tGt, i + 1
	case '[':
		return tLBracket, i + 1
	case ']':
		return tRBracket, i + 1
	case '{':
		return tLBrace, i + 1
	case '}':
		return tRBrace, i + 1
	case '(':
		return tLParen, i + 1
	case ')':
		return tRParen, i + 1
	case '.':
		return tDot, i + 1
	case ',':
		return tComma, i + 1
	case '-':
		return tMinus, i + 1
	case '!':
		return tNot, i + 1
	case '?':
		return tQuestion, i + 1
	case ':':
		return tColon, i + 1
	case '+':
		return tPlus, i + 1
	case '*':
		return tStar, i + 1
	case '/':
		return tSlash, i + 1
	case '%':
		return tPercent, i + 1
	}
	return tBad, i
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func isDigit(c byte) bool  { return '0' <= c && c <= '9' }
func isHex(c byte) bool    { return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' }

// stringPrefix is the length of a string literal's prefix at s (r, b, or
// b then r, in either case) when a quote follows it, else 0.
func stringPrefix(s string) int {
	n := 0
	if n < len(s) && (s[n] == 'b' || s[n] == 'B') {
		n++
	}
	if n < len(s) && (s[n] == 'r' || s[n] == 'R') {
		n++
	}
	if n > 0 && n < len(s) && (s[n] == '"' || s[n] == '\'') {
		return n
	}
	return 0
}

// scanString reads a string literal whose quote is at src[q], after any
// prefix, and returns the offset just past it. CEL.g4's STRING: a single
// quote stops at a line break, a triple quote doesn't, a raw string reads
// backslashes as they are, and a non-raw one skips an escape's second
// character, which unescape checks later.
func scanString(src string, q int) (int, bool) {
	raw := q > 0 && (src[q-1] == 'r' || src[q-1] == 'R')
	quote := src[q]
	triple := q+2 < len(src) && src[q+1] == quote && src[q+2] == quote
	i := q + 1
	if triple {
		i = q + 3
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\\' && !raw:
			i += 2
			continue
		case triple:
			if c == quote && i+2 < len(src) && src[i+1] == quote && src[i+2] == quote {
				return i + 3, true
			}
		case c == quote:
			return i + 1, true
		case c == '\n' || c == '\r':
			return 0, false
		}
		i++
	}
	return 0, false
}

// scanNumber reads NUM_FLOAT, NUM_UINT or NUM_INT at src[i], taking the
// longest, as ANTLR's lexer does: `1.5e3` is a double, `1.e3` is 1, a dot
// and the identifier e3, and `0x` with no hex digit is 0 and the
// identifier x.
func scanNumber(src string, i int) (tokKind, int) {
	start := i
	if src[i] == '0' && i+2 < len(src) && src[i+1] == 'x' && isHex(src[i+2]) {
		i += 2
		for i < len(src) && isHex(src[i]) {
			i++
		}
		if i < len(src) && (src[i] == 'u' || src[i] == 'U') {
			return tUint, i + 1
		}
		return tInt, i
	}
	for i < len(src) && isDigit(src[i]) {
		i++
	}
	float := false
	if i+1 < len(src) && src[i] == '.' && isDigit(src[i+1]) {
		float = true
		i += 2
		for i < len(src) && isDigit(src[i]) {
			i++
		}
	}
	if e := exponent(src, i); e > i && (float || i > start) {
		return tDouble, e
	}
	if float {
		return tDouble, i
	}
	if i < len(src) && (src[i] == 'u' || src[i] == 'U') {
		return tUint, i + 1
	}
	return tInt, i
}

// exponent is the offset past an exponent at src[i] (e, a sign, digits),
// or i when there is none.
func exponent(src string, i int) int {
	if i >= len(src) || src[i] != 'e' && src[i] != 'E' {
		return i
	}
	j := i + 1
	if j < len(src) && (src[j] == '+' || src[j] == '-') {
		j++
	}
	if j >= len(src) || !isDigit(src[j]) {
		return i
	}
	for j < len(src) && isDigit(src[j]) {
		j++
	}
	return j
}
