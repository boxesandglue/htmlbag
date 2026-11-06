package htmlbag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/lang"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// hyphenLimits is the value of hyphenate-limit-chars (CSS Text 4): the
// characters a word needs at least, and those before and after a hyphen. A
// negative number stands for auto, which leaves the language's own minimum.
type hyphenLimits struct {
	word, before, after int
}

// parseHyphenLimits reads a value of hyphenate-limit-chars and reports whether
// it is valid. The limits are nil for auto. An invalid value leaves the
// inherited one in place, which covers inherit, unset and revert.
func parseHyphenLimits(v string) (*hyphenLimits, bool) {
	fields := strings.Fields(strings.ToLower(v))
	if len(fields) == 0 || len(fields) > 3 {
		return nil, false
	}
	if len(fields) == 1 && fields[0] == "initial" {
		return nil, true
	}
	values := [3]int{-1, -1, -1}
	for i, f := range fields {
		if f == "auto" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, false
		}
		values[i] = n
	}
	if len(fields) == 2 {
		values[2] = values[1]
	}
	if values == [3]int{-1, -1, -1} {
		return nil, true
	}
	return &hyphenLimits{values[0], values[1], values[2]}, true
}

// parseHyphenCharacter reads a value of hyphenate-character and reports whether
// it is valid: one CSS string, or auto for the hyphen of the font, which is nil.
// An invalid value leaves the inherited one in place, which covers inherit,
// unset and revert.
func parseHyphenCharacter(v string) (*string, bool) {
	v = strings.TrimSpace(v)
	if keyword := strings.ToLower(v); keyword == "auto" || keyword == "initial" {
		return nil, true
	}
	inner, ok := unquoteCSSString(v)
	if !ok {
		return nil, false
	}
	var text strings.Builder
	for i := 0; i < len(inner); i++ {
		switch c := inner[i]; {
		case c == '\\' && i+1 < len(inner):
			i++
			text.WriteByte(inner[i])
		case c == v[0]:
			// Two strings: "a" "b".
			return nil, false
		default:
			text.WriteByte(c)
		}
	}
	s := text.String()
	return &s, true
}

// limitedLanguage returns l with the limits applied. The document holds one
// language per language and limits, so that a change of limits within a
// document keeps its word caches apart.
func limitedLanguage(df *frontend.Document, l *lang.Lang, limits *hyphenLimits) *lang.Lang {
	if limits == nil || l == nil {
		return l
	}
	key := fmt.Sprintf("%s\x00%d %d %d", l.Name, limits.word, limits.before, limits.after)
	if limited, ok := df.Doc.Languages[key]; ok {
		return limited
	}
	limited := l.Copy()
	if limits.word >= 0 {
		limited.Wordhyphenmin = limits.word
	}
	if limits.before >= 0 {
		limited.Lefthyphenmin = limits.before
	}
	if limits.after >= 0 {
		limited.Righthyphenmin = limits.after
	}
	if df.Doc.Languages == nil {
		df.Doc.Languages = map[string]*lang.Lang{}
	}
	df.Doc.Languages[key] = limited
	return limited
}
