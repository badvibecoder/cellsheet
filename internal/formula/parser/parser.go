// Package parser turns tokens into a syntax tree.
//
// It is a precedence-driven (Pratt) parser rather than a hand-written recursive
// descent for one specific grammar. That choice is what makes operator
// extensibility real: adding a row to the ops table adds an operator, with no
// change here.
package parser

import (
	"fmt"
	"strings"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/formula/ast"
	"github.com/badvibecoder/cellsheet/internal/formula/lexer"
	"github.com/badvibecoder/cellsheet/internal/formula/ops"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

// Error is a syntax error with the byte offset where it was found.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s (at offset %d)", e.Msg, e.Pos) }

type parser struct {
	toks []lexer.Token
	pos  int
}

// Parse compiles formula source into a tree. The leading '=' is optional.
func Parse(src string) (ast.Expr, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	if p.peek().Kind == lexer.EOF {
		return nil, &Error{Pos: 0, Msg: "empty formula"}
	}
	e, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.Kind != lexer.EOF {
		return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("unexpected %s", t.Kind)}
	}
	return e, nil
}

func (p *parser) peek() lexer.Token { return p.toks[p.pos] }

func (p *parser) at(k lexer.Kind) bool { return p.peek().Kind == k }

func (p *parser) next() lexer.Token {
	t := p.toks[p.pos]
	if t.Kind != lexer.EOF {
		p.pos++
	}
	return t
}

func (p *parser) expect(k lexer.Kind) error {
	t := p.peek()
	if t.Kind != k {
		return &Error{Pos: t.Pos, Msg: fmt.Sprintf("expected %s, found %s", k, t.Kind)}
	}
	p.pos++
	return nil
}

// parseExpr parses an expression whose operators must bind at least as tightly
// as minPrec.
func (p *parser) parseExpr(minPrec int) (ast.Expr, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()

		// Postfix percent binds tighter than negation, so -50% is -(50%).
		if ops.Percent(t.Kind) {
			if ops.PrecPercent < minPrec {
				break
			}
			p.next()
			left = &ast.Unary{Op: ast.OpPercent, X: left}
			continue
		}

		in, ok := ops.LookupInfix(t.Kind)
		if !ok || in.Prec < minPrec {
			break
		}
		p.next()
		nextMin := in.Prec + 1
		if in.Right {
			nextMin = in.Prec
		}
		right, err := p.parseExpr(nextMin)
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{Op: in.Op, X: left, Y: right}
	}
	return left, nil
}

func (p *parser) parsePrefix() (ast.Expr, error) {
	t := p.peek()
	if pre, ok := ops.LookupPrefix(t.Kind); ok {
		p.next()
		// Parsing the operand at the unary precedence makes ^ bind tighter
		// (-2^2 is -4) while * does not (-2*3 is -6).
		x, err := p.parseExpr(pre.Prec)
		if err != nil {
			return nil, err
		}
		return &ast.Unary{Op: pre.Op, X: x, IsPre: true}, nil
	}
	return p.parseRangeOrPrimary()
}

// parseRangeOrPrimary parses a primary and, if a colon follows, a range.
func (p *parser) parseRangeOrPrimary() (ast.Expr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if !p.at(lexer.Colon) {
		return e, nil
	}
	p.next()
	rhs, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return makeRange(e, rhs)
}

func makeRange(lhs, rhs ast.Expr) (ast.Expr, error) {
	l, ok := lhs.(ast.Ref)
	if !ok {
		return nil, &Error{Msg: "the left side of ':' must be a cell reference"}
	}
	r, ok := rhs.(ast.Ref)
	if !ok {
		return nil, &Error{Msg: "the right side of ':' must be a cell reference"}
	}
	// A range must lie within one sheet. Sheet2!A1:B5 means Sheet2!A1:Sheet2!B5.
	switch {
	case l.Sheet == "" && r.Sheet != "":
		l.Sheet = r.Sheet
	case r.Sheet == "" && l.Sheet != "":
		r.Sheet = l.Sheet
	case l.Sheet != r.Sheet:
		return nil, &Error{Msg: "a range cannot span two sheets"}
	}
	// Normalise so From is the top-left corner.
	if r.Row < l.Row {
		l.Row, r.Row = r.Row, l.Row
	}
	if r.Col < l.Col {
		l.Col, r.Col = r.Col, l.Col
	}
	return ast.Range{From: l, To: r}, nil
}

func (p *parser) parsePrimary() (ast.Expr, error) {
	t := p.peek()
	switch t.Kind {
	case lexer.Number:
		p.next()
		d, ok := cell.ParseDec(t.Text)
		if !ok {
			return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("%q is not a valid number", t.Text)}
		}
		return ast.Literal{Value: cell.Number(d)}, nil

	case lexer.Currency:
		p.next()
		v := cell.Infer(t.Text)
		if v.Kind != cell.KindCurrency {
			return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("%q is not a valid currency amount", t.Text)}
		}
		return ast.Literal{Value: v}, nil

	case lexer.String:
		p.next()
		return ast.Literal{Value: cell.Text(t.Text)}, nil

	case lexer.LParen:
		p.next()
		e, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.RParen); err != nil {
			return nil, err
		}
		return e, nil

	case lexer.QuotedName:
		p.next()
		return p.afterSheetName(t)

	case lexer.Ident:
		p.next()
		if p.at(lexer.Bang) {
			return p.afterSheetName(t)
		}
		if p.at(lexer.LParen) {
			return p.parseCall(t)
		}
		if ref, ok := refFromIdent(t.Text); ok {
			return ref, nil
		}
		return ast.Name{Ident: t.Text}, nil
	}
	return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("unexpected %s", t.Kind)}
}

func (p *parser) afterSheetName(name lexer.Token) (ast.Expr, error) {
	if err := p.expect(lexer.Bang); err != nil {
		return nil, err
	}
	t := p.peek()
	if t.Kind != lexer.Ident {
		return nil, &Error{Pos: t.Pos, Msg: "expected a cell reference after '!'"}
	}
	p.next()
	ref, ok := refFromIdent(t.Text)
	if !ok {
		return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("%q is not a cell reference", t.Text)}
	}
	ref.Sheet = name.Text
	return ref, nil
}

func (p *parser) parseCall(name lexer.Token) (ast.Expr, error) {
	if err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	call := &ast.Call{Name: strings.ToUpper(name.Text)}
	if p.at(lexer.RParen) {
		p.next()
		return call, nil
	}
	for {
		arg, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		call.Args = append(call.Args, arg)
		t := p.peek()
		switch t.Kind {
		case lexer.Comma:
			p.next()
		case lexer.RParen:
			p.next()
			return call, nil
		default:
			return nil, &Error{Pos: t.Pos, Msg: fmt.Sprintf("expected ',' or ')', found %s", t.Kind)}
		}
	}
}

// refFromIdent converts an identifier into a cell reference when it looks like
// one. "SUM" and "Sheet1" do not; "A1" and "$B$7" do.
func refFromIdent(s string) (ast.Ref, bool) {
	ref, ok := grid.ParseRef(s)
	if !ok {
		return ast.Ref{}, false
	}
	return ast.Ref{Row: int(ref.Row), Col: int(ref.Col)}, true
}

// Refs walks a tree and reports every sheet-qualified name it mentions, which
// the workbook layer uses to warn before deleting or renaming a sheet.
func Refs(e ast.Expr, fn func(sheet string, r ast.Ref)) {
	switch n := e.(type) {
	case ast.Ref:
		fn(n.Sheet, n)
	case ast.Range:
		fn(n.From.Sheet, n.From)
		fn(n.To.Sheet, n.To)
	case *ast.Unary:
		Refs(n.X, fn)
	case *ast.Binary:
		Refs(n.X, fn)
		Refs(n.Y, fn)
	case *ast.Call:
		for _, a := range n.Args {
			Refs(a, fn)
		}
	}
}
