// Package lexer turns formula source into tokens.
//
// A cell reference and a function name look identical to a lexer (A1 and SUM are
// both just identifiers), so both become Ident and the parser decides which is
// which. That keeps this package small and keeps the grammar in one place.
package lexer

import (
	"fmt"
	"strings"
)

// Kind is a token type.
type Kind uint8

const (
	EOF Kind = iota
	Number
	Currency
	String
	QuotedName
	Ident

	Plus
	Minus
	Star
	Slash
	Percent
	Caret

	LParen
	RParen
	Comma
	Colon
	Bang

	Eq
	Ne
	Lt
	Gt
	Le
	Ge
)

func (k Kind) String() string {
	switch k {
	case EOF:
		return "end of formula"
	case Number:
		return "number"
	case Currency:
		return "currency amount"
	case String:
		return "text"
	case QuotedName:
		return "quoted name"
	case Ident:
		return "name"
	case Plus:
		return "'+'"
	case Minus:
		return "'-'"
	case Star:
		return "'*'"
	case Slash:
		return "'/'"
	case Percent:
		return "'%'"
	case Caret:
		return "'^'"
	case LParen:
		return "'('"
	case RParen:
		return "')'"
	case Comma:
		return "','"
	case Colon:
		return "':'"
	case Bang:
		return "'!'"
	case Eq:
		return "'='"
	case Ne:
		return "'<>'"
	case Lt:
		return "'<'"
	case Gt:
		return "'>'"
	case Le:
		return "'<='"
	case Ge:
		return "'>='"
	default:
		return "token"
	}
}

// Token is one lexical unit.
type Token struct {
	Kind Kind
	Text string // identifiers and literals keep their original text
	Pos  int    // byte offset in the source, for error messages
}

// Error is a lexical error with a position.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s at offset %d", e.Msg, e.Pos) }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

// Lex tokenises src. A leading '=' is optional and is skipped, so both
// "=SUM(A1)" and "SUM(A1)" lex identically.
//
// The non-standard "-=" form the user asked for ("-=SUM(A1+D1)") is normalised
// here to "-SUM(A1+D1)", which parses as unary minus applied to the call.
func Lex(src string) ([]Token, error) {
	s := strings.TrimSpace(src)
	s = strings.TrimPrefix(s, "=")
	if strings.HasPrefix(s, "-=") {
		s = "-" + s[2:]
	}

	var out []Token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '"':
			text, n, err := scanString(s[i:])
			if err != nil {
				return nil, &Error{Pos: i, Msg: err.Error()}
			}
			out = append(out, Token{Kind: String, Text: text, Pos: i})
			i += n
		case c == '\'':
			name, n, err := scanQuotedName(s[i:])
			if err != nil {
				return nil, &Error{Pos: i, Msg: err.Error()}
			}
			out = append(out, Token{Kind: QuotedName, Text: name, Pos: i})
			i += n
		case c == '$' && i+1 < len(s) && isDigit(s[i+1]):
			text, n := scanCurrency(s[i:])
			out = append(out, Token{Kind: Currency, Text: text, Pos: i})
			i += n
		case c == '$':
			// An absolute reference such as $A$1. The '$' is accepted and
			// ignored for now; absolute addressing arrives with copy/paste
			// translation (PHASE-1-SPEC.md §18.2).
			text, n := scanIdent(s[i:])
			out = append(out, Token{Kind: Ident, Text: text, Pos: i})
			i += n
		case isDigit(c) || (c == '.' && i+1 < len(s) && isDigit(s[i+1])):
			text, n := scanNumber(s[i:])
			out = append(out, Token{Kind: Number, Text: text, Pos: i})
			i += n
		case isIdentStart(c):
			text, n := scanIdent(s[i:])
			out = append(out, Token{Kind: Ident, Text: text, Pos: i})
			i += n
		default:
			tok, n, err := scanOperator(s[i:])
			if err != nil {
				return nil, &Error{Pos: i, Msg: err.Error()}
			}
			tok.Pos = i
			out = append(out, tok)
			i += n
		}
	}
	out = append(out, Token{Kind: EOF, Pos: len(s)})
	return out, nil
}

func scanOperator(s string) (Token, int, error) {
	switch {
	case strings.HasPrefix(s, "<="):
		return Token{Kind: Le}, 2, nil
	case strings.HasPrefix(s, ">="):
		return Token{Kind: Ge}, 2, nil
	case strings.HasPrefix(s, "<>"):
		return Token{Kind: Ne}, 2, nil
	}
	switch s[0] {
	case '+':
		return Token{Kind: Plus}, 1, nil
	case '-':
		return Token{Kind: Minus}, 1, nil
	case '*':
		return Token{Kind: Star}, 1, nil
	case '/':
		return Token{Kind: Slash}, 1, nil
	case '%':
		return Token{Kind: Percent}, 1, nil
	case '^':
		return Token{Kind: Caret}, 1, nil
	case '(':
		return Token{Kind: LParen}, 1, nil
	case ')':
		return Token{Kind: RParen}, 1, nil
	case ',':
		return Token{Kind: Comma}, 1, nil
	case ':':
		return Token{Kind: Colon}, 1, nil
	case '!':
		return Token{Kind: Bang}, 1, nil
	case '=':
		return Token{Kind: Eq}, 1, nil
	case '<':
		return Token{Kind: Lt}, 1, nil
	case '>':
		return Token{Kind: Gt}, 1, nil
	}
	return Token{}, 0, fmt.Errorf("unexpected character %q", string(s[0]))
}

func scanString(s string) (string, int, error) {
	// s[0] is '"'. Excel escapes a quote by doubling it.
	var b strings.Builder
	i := 1
	for i < len(s) {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i += 2
				continue
			}
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
		i++
	}
	return "", 0, fmt.Errorf("unterminated text literal")
}

func scanQuotedName(s string) (string, int, error) {
	// s[0] is '\''. A literal apostrophe is doubled.
	var b strings.Builder
	i := 1
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
		i++
	}
	return "", 0, fmt.Errorf("unterminated sheet name")
}

func scanIdent(s string) (string, int) {
	i := 0
	for i < len(s) && (isIdentPart(s[i]) || s[i] == '$' || s[i] == '.') {
		i++
	}
	return s[:i], i
}

func scanNumber(s string) (string, int) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			i = j
			for i < len(s) && isDigit(s[i]) {
				i++
			}
		}
	}
	return s[:i], i
}

// scanCurrency reads a $-prefixed literal: $50,000.25
func scanCurrency(s string) (string, int) {
	i := 1
	for i < len(s) && (isDigit(s[i]) || s[i] == ',' || s[i] == '.') {
		i++
	}
	return s[:i], i
}
