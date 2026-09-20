package parser

import "strings"

// AutoClose appends the closing parentheses a formula is missing.
//
//	=SUM(A1:A4     ->  =SUM(A1:A4)
//	=((1+2         ->  =((1+2))
//
// This is the one repair that is unambiguously what the user meant. A formula
// cannot meaningfully end in the middle of a call, so closing it is the only
// completion; anything else malformed is left alone and reported as an error,
// because guessing there would be worse than complaining.
//
// Text in double quotes and sheet names in single quotes are skipped, and a
// doubled quote is an escape, so =CONCAT("(", A1 keeps its literal bracket.
// More closing than opening parentheses is left untouched: silently dropping
// one could change what the user wrote.
func AutoClose(src string) string {
	depth := 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '"', '\'':
			for i++; i < len(src); i++ {
				if src[i] != c {
					continue
				}
				if i+1 < len(src) && src[i+1] == c {
					i++ // a doubled quote is a literal one
					continue
				}
				break
			}
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	if depth <= 0 {
		return src
	}
	return src + strings.Repeat(")", depth)
}
