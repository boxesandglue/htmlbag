package htmlbag

import (
	"strconv"
	"strings"
)

// formatCounterStyle renders the number n in the CSS counter style named by
// style: the predefined styles of CSS Counter Styles 3 §6 that a list marker
// or counter() commonly uses. Unknown names render as decimal, which is what
// the specification prescribes for a counter style it does not know. The
// symbolic styles (disc, circle, square) ignore n; "none" is the empty
// string.
func formatCounterStyle(n int, style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "none":
		return ""
	case "disc":
		return "•"
	case "circle":
		return "◦"
	case "square":
		return "□"
	case "decimal-leading-zero":
		if n >= 0 && n < 10 {
			return "0" + strconv.Itoa(n)
		}
		if n < 0 && n > -10 {
			return "-0" + strconv.Itoa(-n)
		}
		return strconv.Itoa(n)
	case "lower-alpha", "lower-latin":
		return alphabetic(n, "abcdefghijklmnopqrstuvwxyz")
	case "upper-alpha", "upper-latin":
		return alphabetic(n, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	case "lower-greek":
		return alphabetic(n, "αβγδεζηθικλμνξοπρστυφχψω")
	case "lower-roman":
		return strings.ToLower(roman(n))
	case "upper-roman":
		return roman(n)
	}
	return strconv.Itoa(n)
}

// counterStyleIsNumeric reports whether a list marker in this style is a
// number or letter that takes the "." suffix of an ordered list, as opposed to
// a bullet symbol that stands alone.
func counterStyleIsNumeric(style string) bool {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "none", "disc", "circle", "square":
		return false
	}
	return true
}

// alphabetic is the CSS "alphabetic" system: a, b, ..., z, aa, ab, ... with
// the given symbols. Zero and negative numbers have no alphabetic form and
// fall back to decimal, as the specification says.
func alphabetic(n int, symbols string) string {
	if n <= 0 {
		return strconv.Itoa(n)
	}
	syms := []rune(symbols)
	base := len(syms)
	var out []rune
	for n > 0 {
		n--
		out = append([]rune{syms[n%base]}, out...)
		n /= base
	}
	return string(out)
}

// roman is the CSS "additive" roman system for 1 to 3999. Outside that range
// there is no roman form and the number is rendered as decimal.
func roman(n int) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	var sb strings.Builder
	for _, step := range []struct {
		value  int
		symbol string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
		{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	} {
		for n >= step.value {
			sb.WriteString(step.symbol)
			n -= step.value
		}
	}
	return sb.String()
}
