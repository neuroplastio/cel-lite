package cellite

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits on what Parse reads. cel-go has its own (100,000 code points, a
// parse depth of 250); these are lower, and a deeper expression is
// refused rather than read. The parser, the checker, the planner and the
// evaluator recurse once per level, and a TinyGo goroutine's stack is a
// fixed 64 KB, which an overflow corrupts silently: the limits keep each
// of them within 16 KB there, as `make stack` measures and the tests
// check. Brackets, calls and conditionals also nest at most 16 deep. A
// limit rises only with a new measurement.
const (
	// MaxSize is the longest source Parse reads, in bytes.
	MaxSize = 100_000
	// MaxDepth is how deep an expression's tree may be. && and || chains
	// are built balanced, as cel-go builds them, so a long chain is
	// shallow; `a + b + c` chains are not.
	MaxDepth = 50
	// maxNest is how deeply brackets, calls and conditionals may nest:
	// each level is nine of the parser's own frames, about 1 KB under
	// TinyGo.
	maxNest = 16
)

// reserved are the words CEL keeps from identifiers (cel-go's
// reservedIds). A field may still have one as its name: `x.if`.
var reserved = map[string]bool{
	"as": true, "break": true, "const": true, "continue": true, "else": true, "false": true,
	"for": true, "function": true, "if": true, "import": true, "in": true, "let": true,
	"loop": true, "package": true, "namespace": true, "null": true, "return": true, "true": true,
	"var": true, "void": true, "while": true,
}

// macros are cel-go's standard macros besides has(), each with the
// argument counts that make a call one. They are refused: comprehensions
// are not implemented.
var macros = map[string][]int{
	"all": {2}, "exists": {2}, "exists_one": {2}, "map": {2, 3}, "filter": {2},
}

// parser reads tokens into an Expr tree.
type parser struct {
	src  string
	toks []token
	i    int
	tok  token
	err  *Error
	nest int
	// n holds the tree's nodes. A node takes a token or more, so the
	// tokens bound their number.
	n nodes
}

// Parse reads a CEL expression into its tree, unchecked: names are
// OpIdent until Check resolves them. A syntax error and a feature this
// package doesn't implement are errors (*Error). Each byte of src that
// isn't UTF-8 reads as U+FFFD, as in cel-go, and positions count in that
// text.
func Parse(src string) (*Expr, error) {
	if len(src) > MaxSize {
		return nil, newError(src, 0, SyntaxError, "", "expression is larger than "+strconv.Itoa(MaxSize)+" bytes")
	}
	src = validUTF8(src)
	p := &parser{src: src, toks: lex(src)}
	p.n.size = len(p.toks)
	p.tok = p.toks[0]
	x := p.expr()
	if p.err == nil && p.tok.kind != tEOF {
		p.unexpected()
	}
	if p.err != nil {
		return nil, p.err
	}
	// The parser's own recursion is bounded by maxNest; the tree's depth
	// is checked once it is built.
	if d := deepNode(x, 1); d != nil {
		return nil, newError(src, d.Pos, SyntaxError, "", "expression is nested deeper than "+strconv.Itoa(MaxDepth))
	}
	return x, nil
}

// validUTF8 is s with each byte that isn't UTF-8 replaced by U+FFFD.
func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
	}
	return b.String()
}

// next moves to the next token; the last, tEOF or tBad, stays.
func (p *parser) next() {
	if p.i < len(p.toks)-1 {
		p.i++
	}
	p.tok = p.toks[p.i]
}

// peek is the token after the current one.
func (p *parser) peek() token {
	if p.i < len(p.toks)-1 {
		return p.toks[p.i+1]
	}
	return p.tok
}

// fail records the first error.
//
//go:noinline
func (p *parser) fail(pos int, kind ErrorKind, name, msg string) {
	if p.err == nil {
		p.err = newError(p.src, pos, kind, name, msg)
	}
}

// syntax records a syntax error at pos.
func (p *parser) syntax(pos int, msg string) { p.fail(pos, SyntaxError, "", "syntax error: "+msg) }

// unsupported refuses a feature by name.
func (p *parser) unsupported(pos int, name, msg string) { p.fail(pos, Unsupported, name, msg) }

// unexpected is a syntax error at the current token.
//
//go:noinline
func (p *parser) unexpected() {
	t := p.tok
	switch {
	case t.kind == tBad && t.text != "" && (t.text[0] == '"' || t.text[0] == '\'' || stringPrefix(t.text) > 0):
		p.syntax(t.pos, "unterminated string")
	case t.kind == tBad:
		r, _ := utf8.DecodeRuneInString(p.src[t.pos:])
		p.syntax(t.pos, "unexpected character "+strconv.QuoteRune(r))
	case t.kind == tEscIdent:
		p.unsupported(t.pos, "escaped identifier", "escaped identifiers (`a.b`) are not supported")
	case t.kind == tEOF:
		p.syntax(t.pos, "unexpected end of input")
	default:
		p.syntax(t.pos, "unexpected "+strconv.Quote(t.text))
	}
}

// expect consumes a token of kind k, or fails.
func (p *parser) expect(k tokKind) bool {
	if p.err != nil {
		return false
	}
	if p.tok.kind != k {
		if p.tok.kind == tEOF || p.tok.kind == tBad || p.tok.kind == tEscIdent {
			p.unexpected()
		} else {
			p.syntax(p.tok.pos, "expected "+tokenDesc[k]+", found "+strconv.Quote(p.tok.text))
		}
		return false
	}
	p.next()
	return true
}

// node makes a node.
func (p *parser) node(op Op, pos int, args ...*Expr) *Expr {
	probe()
	x := p.n.expr()
	x.Op, x.Pos = op, pos
	x.Args = p.n.list(len(args))
	copy(x.Args, args)
	return x
}

// expr is CEL.g4's expr: conditionalOr, then `? conditionalOr : expr`.
// The middle operand is not itself a conditional, as in CEL.
func (p *parser) expr() *Expr {
	if p.nest++; p.nest > maxNest {
		p.fail(p.tok.pos, SyntaxError, "", "expression nests deeper than "+strconv.Itoa(maxNest))
		return nil
	}
	x := p.cond()
	p.nest--
	return x
}

func (p *parser) cond() *Expr {
	c := p.binary(precOr)
	if p.err != nil || p.tok.kind != tQuestion {
		return c
	}
	pos := p.tok.pos
	p.next()
	t := p.binary(precOr)
	if !p.expect(tColon) {
		return nil
	}
	f := p.expr()
	if p.err != nil {
		return nil
	}
	return p.node(OpCond, pos, c, t, f)
}

// The binary operators' precedences, as CEL.g4's rules nest them.
const (
	precOr = 1 + iota
	precAnd
	precRelation
	precAdditive
	precMultiplicative
)

// binOp is the precedence and the node of the binary operator tok, or 0
// when tok is none.
func binOp(tok tokKind) (int, Op) {
	switch tok {
	case tOr:
		return precOr, OpOr
	case tAnd:
		return precAnd, OpAnd
	case tLt:
		return precRelation, OpLt
	case tLe:
		return precRelation, OpLe
	case tGt:
		return precRelation, OpGt
	case tGe:
		return precRelation, OpGe
	case tEq:
		return precRelation, OpEq
	case tNe:
		return precRelation, OpNe
	case tIn:
		return precRelation, OpIn
	case tPlus:
		return precAdditive, OpAdd
	case tMinus:
		return precAdditive, OpSub
	case tStar:
		return precMultiplicative, OpMul
	case tSlash:
		return precMultiplicative, OpDiv
	case tPercent:
		return precMultiplicative, OpMod
	}
	return 0, 0
}

// binary reads CEL.g4's conditionalOr down to multiplication, the
// operators of precedence prec and above, by precedence climbing: one
// frame for all five rules, which keeps the parser's recursion shallow.
// Relations and arithmetic fold to the left. A run of && or || is built
// balanced, as cel-go builds it; within one call the operators come in
// falling precedence, so an && run is done when || follows.
func (p *parser) binary(prec int) *Expr {
	x := p.unary()
	var (
		run   Op // the && or || of the run, if one is open
		terms []*Expr
		pos   []int
	)
	for p.err == nil {
		bp, op := binOp(p.tok.kind)
		if bp < prec || bp == 0 {
			break
		}
		at := p.tok.pos
		p.next()
		y := p.binary(bp + 1)
		if p.err != nil {
			return nil
		}
		switch {
		case op != OpAnd && op != OpOr:
			x = p.node(op, at, x, y)
		case op == run:
			terms, pos = append(terms, y), append(pos, at)
		default:
			if run != 0 {
				x = p.balance(run, terms, pos, 0, len(pos)-1)
			}
			run, terms, pos = op, []*Expr{x, y}, []int{at}
		}
	}
	if p.err != nil {
		return nil
	}
	if run != 0 {
		x = p.balance(run, terms, pos, 0, len(pos)-1)
	}
	return x
}

// balance builds the operators pos[lo..hi] and the terms around them
// into a balanced tree of op: cel-go's balancer, the same shape.
//
//go:noinline
func (p *parser) balance(op Op, terms []*Expr, pos []int, lo, hi int) *Expr {
	mid := (lo + hi + 1) / 2
	left := terms[mid]
	if mid != lo {
		left = p.balance(op, terms, pos, lo, mid-1)
	}
	right := terms[mid+1]
	if mid != hi {
		right = p.balance(op, terms, pos, mid+1, hi)
	}
	return p.node(op, pos[mid], left, right)
}

// unary is CEL.g4's unary: a member, or a run of `!` or a run of `-`
// before a member. As in cel-go, an even run cancels out whatever the
// operand's type (`!!x` is x), and a lone `-` before a number is that
// number's sign, while a longer run of `-` takes the number unsigned.
func (p *parser) unary() *Expr {
	if p.err != nil {
		return nil
	}
	k := p.tok.kind
	if k != tNot && k != tMinus {
		return p.member()
	}
	if k == tMinus && (p.peek().kind == tInt || p.peek().kind == tDouble) {
		return p.member()
	}
	pos, n := p.tok.pos, 0
	for p.tok.kind == k {
		n++
		p.next()
	}
	x := p.member()
	if p.err != nil || n%2 == 0 {
		return x
	}
	if k == tNot {
		return p.node(OpNot, pos, x)
	}
	return p.node(OpNeg, pos, x)
}

// member is a primary with its selections, calls and indexes.
func (p *parser) member() *Expr {
	x := p.primary()
	for p.err == nil {
		switch p.tok.kind {
		case tDot:
			pos := p.tok.pos
			p.next()
			switch p.tok.kind {
			case tQuestion:
				p.unsupported(p.tok.pos, "optional syntax", "optional syntax (.?) is not supported")
				return nil
			case tEscIdent:
				p.unexpected()
				return nil
			case tIdent:
			default:
				p.unexpected()
				return nil
			}
			name, npos := p.tok.text, p.tok.pos
			p.next()
			switch p.tok.kind {
			case tLParen:
				args := p.args()
				if p.err != nil {
					return nil
				}
				x = p.call(npos, name, x, args)
			case tLBrace:
				p.unsupported(p.tok.pos, "message construction", "constructing a message (T{…}) is not supported")
				return nil
			default:
				x = p.node(OpSelect, pos, x)
				x.Name = name
			}
		case tLBracket:
			pos := p.tok.pos
			p.next()
			if p.tok.kind == tQuestion {
				p.unsupported(p.tok.pos, "optional syntax", "optional syntax ([?) is not supported")
				return nil
			}
			i := p.expr()
			if !p.expect(tRBracket) {
				return nil
			}
			x = p.node(OpIndex, pos, x, i)
		default:
			return x
		}
	}
	return nil
}

// args reads a call's parenthesized arguments; the current token is the
// opening parenthesis. CEL allows no trailing comma here.
func (p *parser) args() []*Expr {
	p.next()
	var args []*Expr
	if p.tok.kind == tRParen {
		p.next()
		return args
	}
	for p.err == nil {
		args = append(args, p.expr())
		if p.tok.kind != tComma {
			break
		}
		p.next()
	}
	p.expect(tRParen)
	return args
}

// call makes a call node, or refuses a macro: target is the receiver of
// a method call, nil for a global one. has(x.f) is a macro, kept as
// OpHas.
//
//go:noinline
func (p *parser) call(pos int, name string, target *Expr, args []*Expr) *Expr {
	if target == nil && name == "has" && len(args) == 1 {
		a := args[0]
		if a.Op != OpSelect {
			p.syntax(a.Pos, "invalid argument to has() macro")
			return nil
		}
		x := p.node(OpHas, pos, a.Args[0])
		x.Name = a.Name
		return x
	}
	if target != nil {
		for _, n := range macros[name] {
			if n == len(args) {
				p.unsupported(pos, "macro "+name, "the macro "+name+"() is not supported: comprehensions (all, exists, exists_one, map, filter) are not implemented")
				return nil
			}
		}
		args = append([]*Expr{target}, args...)
	}
	x := p.node(OpCall, pos, args...)
	x.Name, x.Member = name, target != nil
	return x
}

// primary is CEL.g4's primary.
func (p *parser) primary() *Expr {
	if p.err != nil {
		return nil
	}
	t := p.tok
	switch t.kind {
	case tDot, tIdent:
		if t.kind == tDot {
			p.next()
			if p.tok.kind != tIdent {
				p.unexpected()
				return nil
			}
		}
		id := p.tok
		if reserved[id.text] {
			p.syntax(id.pos, "reserved identifier: "+id.text)
			return nil
		}
		p.next()
		switch p.tok.kind {
		case tLParen:
			args := p.args()
			if p.err != nil {
				return nil
			}
			return p.call(id.pos, id.text, nil, args)
		case tLBrace:
			p.unsupported(p.tok.pos, "message construction", "constructing a message (T{…}) is not supported")
			return nil
		}
		x := p.node(OpIdent, id.pos)
		x.Name = id.text
		return x
	case tLParen:
		p.next()
		x := p.expr()
		if !p.expect(tRParen) {
			return nil
		}
		return x
	case tLBracket:
		return p.list()
	case tLBrace:
		return p.mapLit()
	case tMinus:
		if n := p.peek(); n.kind == tInt || n.kind == tDouble {
			p.next()
			return p.literal(t.pos, "-")
		}
	case tInt, tUint, tDouble, tString, tBytes, tTrue, tFalse, tNull:
		return p.literal(t.pos, "")
	}
	p.unexpected()
	return nil
}

// list reads a list literal: `[`, elements, an optional trailing comma
// (`[,]` is empty, as in CEL.g4), `]`.
func (p *parser) list() *Expr {
	pos := p.tok.pos
	p.next()
	var elems []*Expr
	if p.tok.kind == tComma {
		p.next()
	} else {
		for p.err == nil && p.tok.kind != tRBracket {
			if p.tok.kind == tQuestion {
				p.unsupported(p.tok.pos, "optional syntax", "optional elements ([?x]) are not supported")
				return nil
			}
			elems = append(elems, p.expr())
			if p.tok.kind != tComma {
				break
			}
			p.next()
		}
	}
	if !p.expect(tRBracket) {
		return nil
	}
	return p.node(OpList, pos, elems...)
}

// mapLit reads a map literal, as list does.
func (p *parser) mapLit() *Expr {
	pos := p.tok.pos
	p.next()
	var kv []*Expr
	if p.tok.kind == tComma {
		p.next()
	} else {
		for p.err == nil && p.tok.kind != tRBrace {
			if p.tok.kind == tQuestion {
				p.unsupported(p.tok.pos, "optional syntax", "optional entries ({?k: v}) are not supported")
				return nil
			}
			k := p.expr()
			if !p.expect(tColon) {
				return nil
			}
			kv = append(kv, k, p.expr())
			if p.tok.kind != tComma {
				break
			}
			p.next()
		}
	}
	if !p.expect(tRBrace) {
		return nil
	}
	return p.node(OpMap, pos, kv...)
}

// literal reads the literal at the current token, with sign "-" or "".
//
//go:noinline
func (p *parser) literal(pos int, sign string) *Expr {
	t := p.tok
	p.next()
	x := p.node(OpLit, pos)
	switch t.kind {
	case tInt:
		text, base := t.text, 10
		if strings.HasPrefix(text, "0x") {
			text, base = text[2:], 16
		}
		n, err := strconv.ParseInt(sign+text, base, 64)
		if err != nil {
			p.syntax(pos, "invalid int literal")
		}
		x.Value = n
	case tUint:
		text, base := t.text[:len(t.text)-1], 10
		if strings.HasPrefix(text, "0x") {
			text, base = text[2:], 16
		}
		n, err := strconv.ParseUint(text, base, 64)
		if err != nil {
			p.syntax(pos, "invalid uint literal")
		}
		x.Value = n
	case tDouble:
		f, err := strconv.ParseFloat(sign+t.text, 64)
		if err != nil {
			p.syntax(pos, "invalid double literal")
		}
		x.Value = f
	case tString, tBytes:
		bytes := t.kind == tBytes
		text := t.text
		if bytes {
			text = text[1:]
		}
		s, err := unescape(text, bytes)
		if err != "" {
			p.syntax(pos, err)
		}
		if bytes {
			x.Value = []byte(s)
		} else {
			x.Value = s
		}
	case tTrue:
		x.Value = true
	case tFalse:
		x.Value = false
	case tNull:
		x.Value = nil
	}
	return x
}

// unescape reads a string literal's text (prefix, quotes and escapes)
// into its value, as cel-go's unescape: line breaks are normalized to \n,
// \x and octal escapes are code points in a string and bytes in bytes,
// and \u and \U are refused in bytes. The error is "" when it reads.
//
//go:noinline
func unescape(lit string, bytes bool) (string, string) {
	lit = strings.ReplaceAll(strings.ReplaceAll(lit, "\r\n", "\n"), "\r", "\n")
	raw := false
	if lit[0] == 'r' || lit[0] == 'R' {
		lit, raw = lit[1:], true
	}
	q := 1
	if len(lit) >= 6 && (strings.HasPrefix(lit, `"""`) || strings.HasPrefix(lit, "'''")) {
		q = 3
	}
	s := lit[q : len(lit)-q]
	if raw || !strings.Contains(s, `\`) {
		return s, ""
	}
	var b []byte
	for len(s) > 0 {
		c := s[0]
		if c != '\\' {
			b = append(b, c)
			s = s[1:]
			continue
		}
		if len(s) < 2 {
			return "", "unable to unescape string, found '\\' as last character"
		}
		e := s[1]
		s = s[2:]
		var v rune
		encode := true
		switch e {
		case 'a':
			v = '\a'
		case 'b':
			v = '\b'
		case 'f':
			v = '\f'
		case 'n':
			v = '\n'
		case 'r':
			v = '\r'
		case 't':
			v = '\t'
		case 'v':
			v = '\v'
		case '\\', '\'', '"', '`', '?':
			v = rune(e)
		case 'x', 'X', 'u', 'U':
			n := 2
			switch e {
			case 'x', 'X':
				encode = !bytes
			case 'u':
				n = 4
			case 'U':
				n = 8
			}
			if bytes && (e == 'u' || e == 'U') {
				return "", "unable to unescape string"
			}
			if len(s) < n {
				return "", "unable to unescape string"
			}
			for j := 0; j < n; j++ {
				if !isHex(s[j]) {
					return "", "unable to unescape string"
				}
				v = v<<4 | rune(hexVal(s[j]))
			}
			s = s[n:]
			if !bytes && !utf8.ValidRune(v) {
				return "", "invalid unicode code point"
			}
		case '0', '1', '2', '3':
			if len(s) < 2 || s[0] < '0' || s[0] > '7' || s[1] < '0' || s[1] > '7' {
				return "", "unable to unescape octal sequence in string"
			}
			v = rune(e-'0')<<6 | rune(s[0]-'0')<<3 | rune(s[1]-'0')
			s = s[2:]
			encode = !bytes
		default:
			return "", "unable to unescape string"
		}
		if v < utf8.RuneSelf || !encode {
			b = append(b, byte(v))
		} else {
			b = utf8.AppendRune(b, v)
		}
	}
	return string(b), ""
}

func hexVal(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c >= 'a':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}
