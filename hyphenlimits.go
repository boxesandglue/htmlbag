package htmlbag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/lang"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// hyphenLimits is the value of hyphenate-limit-chars (CSS Text 4): the
// characters a word needs at least, and those before and after a hyphen. A
// negative number stands for auto, which leaves the language's own minimum.
type hyphenLimits struct {
	word, before, after int
}

// parseHyphenLimits reads a value of hyphenate-limit-chars and returns nil for
// auto or an invalid value.
func parseHyphenLimits(v string) *hyphenLimits {
	fields := strings.Fields(strings.ToLower(v))
	if len(fields) == 0 || len(fields) > 3 {
		bag.Logger.Warn("hyphenate-limit-chars takes one to three values, ignoring it", "value", v)
		return nil
	}
	values := [3]int{-1, -1, -1}
	for i, f := range fields {
		if f == "auto" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			bag.Logger.Warn("hyphenate-limit-chars: not auto or an integer of zero or more, ignoring it", "value", v)
			return nil
		}
		values[i] = n
	}
	if len(fields) == 2 {
		values[2] = values[1]
	}
	if values == [3]int{-1, -1, -1} {
		return nil
	}
	return &hyphenLimits{values[0], values[1], values[2]}
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
