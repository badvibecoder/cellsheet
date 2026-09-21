package clipboard

import (
	"strconv"
	"strings"

	"github.com/badvibecoder/cellsheet/internal/grid"
)

// TranslateFormula shifts the relative cell references in a formula by a row and
// column offset — what a spreadsheet does when a formula is copied to a
// different cell.
//
// A reference is relative unless a part is marked with '$'. So copying
// "=SUM(B1:B5)" one column to the right gives "=SUM(C1:C5)", while "$B$1" is
// left exactly as it is and "B$1" only shifts its column.
//
// Text literals and quoted sheet names are copied untouched, so a string that
// merely looks like a reference is never rewritten. A reference that would leave
// the sheet becomes "#REF!", which is the same failure Excel reports.
func TranslateFormula(src string, dRow, dCol int) string {
	if dRow == 0 && dCol == 0 {
		return src
	}
	var b strings.Builder
	b.Grow(len(src) + 8)
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"':
			j := endOfString(src, i)
			b.WriteString(src[i:j])
			i = j
		case c == '\'':
			j := endOfQuoted(src, i)
			b.WriteString(src[i:j])
			i = j
		case isRefRune(c):
			j := i
			for j < len(src) && isRefRune(src[j]) {
				j++
			}
			tok := src[i:j]
			// A name followed by '!' is a sheet name, not a reference. The
			// reference after it is a separate token and is handled on the next
			// pass, so the sheet is preserved while its cell shifts.
			if j < len(src) && src[j] == '!' {
				b.WriteString(tok)
			} else {
				b.WriteString(shiftRef(tok, dRow, dCol))
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// shiftRef rewrites one identifier when it is a well-formed A1 reference.
// Anything else (SUM, a bare number, a malformed token) is returned unchanged.
func shiftRef(tok string, dRow, dCol int) string {
	i := 0
	colAbs := false
	if i < len(tok) && tok[i] == '$' {
		colAbs = true
		i++
	}
	ls := i
	for i < len(tok) && isLetter(tok[i]) {
		i++
	}
	if i == ls || i-ls > 3 {
		return tok
	}
	colName := tok[ls:i]

	rowAbs := false
	if i < len(tok) && tok[i] == '$' {
		rowAbs = true
		i++
	}
	rs := i
	for i < len(tok) && isDigit(tok[i]) {
		i++
	}
	if i == rs || i != len(tok) {
		return tok
	}

	col, ok := grid.ParseColName(colName)
	if !ok || col >= grid.MaxCols {
		return tok
	}
	row, err := strconv.Atoi(tok[rs:i])
	if err != nil {
		return tok
	}
	row--
	if row < 0 || row >= grid.MaxRows {
		return tok
	}

	if !colAbs {
		col += dCol
	}
	if !rowAbs {
		row += dRow
	}
	if col < 0 || col >= grid.MaxCols || row < 0 || row >= grid.MaxRows {
		return "#REF!"
	}

	name := grid.ColName(col)
	if colName == strings.ToLower(colName) {
		name = strings.ToLower(name)
	}
	var b strings.Builder
	if colAbs {
		b.WriteByte('$')
	}
	b.WriteString(name)
	if rowAbs {
		b.WriteByte('$')
	}
	b.WriteString(strconv.Itoa(row + 1))
	return b.String()
}

// endOfString returns the index just past a double-quoted literal, honouring the
// doubled-quote escape. An unterminated literal runs to the end of the source.
func endOfString(s string, i int) int {
	i++
	for i < len(s) {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return len(s)
}

// endOfQuoted is endOfString for single-quoted sheet names.
func endOfQuoted(s string, i int) int {
	i++
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return len(s)
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isRefRune is every byte that can appear inside a cell reference or a function
// name, which is all this scanner needs to tell one token from the next.
func isRefRune(c byte) bool {
	return isLetter(c) || isDigit(c) || c == '$' || c == '.'
}
