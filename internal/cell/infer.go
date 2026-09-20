package cell

import "strings"

// Infer decides what a typed string means, following the ordered rules in
// PHASE-1-SPEC.md §7.3. The first rule that matches wins.
//
//  1. starts with '='  -> a formula (handled by the caller; see IsFormula)
//  2. empty or spaces  -> Empty
//  3. currency pattern -> Currency
//  4. number pattern   -> Number
//  5. anything else    -> Text
//
// Text is stored exactly as typed, including surrounding spaces; numeric
// detection trims first, so "  450  " is the number 450.
//
// Deliberately NOT recognised in v1, and therefore Text: TRUE/FALSE, "50%",
// "(500)" as a negative, and dates. See PHASE-1-SPEC.md §7.3.
func Infer(s string) Value {
	t := strings.TrimSpace(s)
	if t == "" {
		return Empty()
	}
	if d, ok := parseCurrencyLiteral(t); ok {
		return Currency(d)
	}
	if d, ok := parseNumberLiteral(t); ok {
		return Number(d)
	}
	return Text(s)
}

// IsFormula reports whether typed input is a formula.
//
// The "-=" form the user asked for counts: "-=SUM(A1:A2)" means the negative of
// the sum, and the parser normalises it to "-SUM(A1:A2)". Checking only for a
// leading '=' here meant that form was stored as plain text and never
// calculated at all.
func IsFormula(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '=' {
		return true
	}
	return len(s) > 1 && s[0] == '-' && s[1] == '='
}

// splitDot separates an optional fractional part.
func splitDot(s string) (intPart, fracPart string, hasFrac bool) {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// stripGrouping removes thousands separators, rejecting malformed grouping
// such as "1,00" or "1234,567".
func stripGrouping(s string) (string, bool) {
	if !strings.Contains(s, ",") {
		return s, true
	}
	sign := ""
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		sign, s = s[:1], s[1:]
	}
	parts := strings.Split(s, ",")
	if len(parts[0]) < 1 || len(parts[0]) > 3 {
		return "", false
	}
	for _, p := range parts {
		if !allDigits(p) {
			return "", false
		}
	}
	for _, p := range parts[1:] {
		if len(p) != 3 {
			return "", false
		}
	}
	return sign + strings.Join(parts, ""), true
}

// parseCurrencyLiteral accepts an optional sign before or after the dollar
// sign, thousands separators, and an optional fractional part:
//
//	$50000  $50,000  $1,250.00  -$500  $-500  +$1,000  $.50
func parseCurrencyLiteral(s string) (Dec, bool) {
	neg := false
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if len(s) == 0 || s[0] != '$' {
		return Dec{}, false
	}
	s = s[1:]
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			neg = !neg
		}
		s = s[1:]
	}
	if s == "" {
		return Dec{}, false
	}
	intPart, fracPart, hasFrac := splitDot(s)
	if hasFrac && fracPart == "" {
		return Dec{}, false
	}
	ip, ok := stripGrouping(intPart)
	if !ok {
		return Dec{}, false
	}
	if ip == "" && !hasFrac {
		return Dec{}, false
	}
	if ip != "" && !allDigits(ip) {
		return Dec{}, false
	}
	if hasFrac && !allDigits(fracPart) {
		return Dec{}, false
	}
	lit := ip
	if lit == "" {
		lit = "0"
	}
	if hasFrac {
		lit += "." + fracPart
	}
	d, ok := ParseDec(lit)
	if !ok {
		return Dec{}, false
	}
	if neg {
		d = d.Neg()
	}
	return d, true
}

// parseNumberLiteral accepts an optional sign, thousands separators, an
// optional fractional part, and an optional decimal exponent:
//
//	450  -3  12.75  50,000  1e3  .5  10.00
func parseNumberLiteral(s string) (Dec, bool) {
	body := s
	exp := ""
	if i := strings.IndexAny(body, "eE"); i >= 0 {
		exp, body = body[i:], body[:i]
	}
	if exp != "" && len(exp) == 1 {
		return Dec{}, false
	}
	// Take the sign off before validating digits, since '-' is not a digit.
	sign := ""
	if len(body) > 0 && (body[0] == '+' || body[0] == '-') {
		sign, body = body[:1], body[1:]
	}
	intPart, fracPart, hasFrac := splitDot(body)
	if hasFrac && fracPart == "" {
		return Dec{}, false
	}
	ip, ok := stripGrouping(intPart)
	if !ok {
		return Dec{}, false
	}
	if ip != "" && !allDigits(ip) {
		return Dec{}, false
	}
	if hasFrac && !allDigits(fracPart) {
		return Dec{}, false
	}
	if ip == "" && !hasFrac {
		return Dec{}, false
	}
	lit := sign + ip
	if ip == "" {
		lit += "0"
	}
	if hasFrac {
		lit += "." + fracPart
	}
	lit += exp
	return ParseDec(lit)
}
