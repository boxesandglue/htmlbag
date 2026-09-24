package htmlbag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// parseTabStops reads -bag-tab-stops:
//
//	none | <tab-stop>#
//	<tab-stop> = <length-percentage>
//	             [ start | end | left | right | center | decimal | decimal(<string>) ]?
//	             [ leader( <string> | dotted | solid | space ) ]?
//
// left and right are synonyms of start and end: like the stops of a word
// processor they follow the text direction. A percentage is a part of the
// paragraph width, resolved when the paragraph is broken into lines. none
// returns an empty, non-nil list, which switches inherited stops off.
func parseTabStops(v string, cur, root bag.ScaledPoint) ([]frontend.TabStop, error) {
	v = strings.TrimSpace(v)
	if v == "none" {
		return []frontend.TabStop{}, nil
	}
	var stops []frontend.TabStop
	for _, part := range splitCSSList(v, ',') {
		ts, err := parseTabStop(part, cur, root)
		if err != nil {
			return nil, fmt.Errorf("-bag-tab-stops: %w", err)
		}
		stops = append(stops, ts)
	}
	if len(stops) == 0 {
		return nil, fmt.Errorf("-bag-tab-stops: no tab stop")
	}
	return stops, nil
}

func parseTabStop(s string, cur, root bag.ScaledPoint) (frontend.TabStop, error) {
	var ts frontend.TabStop
	var havePos, haveAlign, haveLeader bool
	for _, tok := range splitCSSList(s, ' ') {
		switch {
		case tok == "start" || tok == "left":
			ts.Align = node.TabAlignLeft
		case tok == "end" || tok == "right":
			ts.Align = node.TabAlignRight
		case tok == "center":
			ts.Align = node.TabAlignCenter
		case tok == "decimal":
			ts.Align = node.TabAlignDecimal
		case strings.HasPrefix(tok, "decimal(") && strings.HasSuffix(tok, ")"):
			sep, ok := unquoteCSSString(tok[len("decimal(") : len(tok)-1])
			if !ok || sep == "" {
				return ts, fmt.Errorf("%q: decimal() takes a non-empty string", s)
			}
			ts.Align = node.TabAlignDecimal
			ts.Separator = sep
		case strings.HasPrefix(tok, "leader(") && strings.HasSuffix(tok, ")"):
			if haveLeader {
				return ts, fmt.Errorf("%q: more than one leader", s)
			}
			pattern, ok := leaderPattern(tok[len("leader(") : len(tok)-1])
			if !ok {
				return ts, fmt.Errorf("%q: leader() takes a string, dotted, solid or space", s)
			}
			ts.Leader = pattern
			haveLeader = true
			continue
		default:
			if havePos {
				return ts, fmt.Errorf("%q: unexpected %q", s, tok)
			}
			if p, ok := strings.CutSuffix(tok, "%"); ok {
				f, err := strconv.ParseFloat(p, 64)
				if err != nil {
					return ts, fmt.Errorf("%q: invalid percentage %q", s, tok)
				}
				ts.Fraction = f / 100
			} else {
				if !isCSSLength(tok) {
					return ts, fmt.Errorf("%q: invalid position %q", s, tok)
				}
				ts.Position = ParseRelativeSize(tok, cur, root)
			}
			havePos = true
			continue
		}
		if haveAlign {
			return ts, fmt.Errorf("%q: more than one alignment", s)
		}
		haveAlign = true
	}
	if !havePos {
		return ts, fmt.Errorf("%q: a tab stop needs a position", s)
	}
	return ts, nil
}

// isCSSLength reports whether s is a number with a unit, or 0.
func isCSSLength(s string) bool {
	num := strings.TrimRight(s, "abcdefghijklmnopqrstuvwxyz")
	if num == "" {
		return false
	}
	if _, err := strconv.ParseFloat(num, 64); err != nil {
		return false
	}
	return num != s || num == "0"
}

// leaderPattern returns the pattern of a CSS GCPM leader() argument: a
// string or one of the keywords dotted (". "), solid ("_") and space (" ").
func leaderPattern(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	switch arg {
	case "dotted":
		return ". ", true
	case "solid":
		return "_", true
	case "space":
		return " ", true
	}
	s, ok := unquoteCSSString(arg)
	return s, ok && s != ""
}

// unquoteCSSString strips the quotes of a single or double quoted string.
func unquoteCSSString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1], true
	}
	return "", false
}

// splitCSSList splits s at sep outside of quotes and parentheses and drops
// empty parts. With sep ' ' any run of whitespace separates.
func splitCSSList(s string, sep rune) []string {
	var parts []string
	var cur strings.Builder
	var quote rune
	depth := 0
	flush := func() {
		if p := strings.TrimSpace(cur.String()); p != "" {
			parts = append(parts, p)
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && (r == sep || sep == ' ' && (r == '\t' || r == '\n')):
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return parts
}
