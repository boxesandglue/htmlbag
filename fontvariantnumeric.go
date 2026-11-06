package htmlbag

import "strings"

// numericKeywords are the keywords of font-variant-numeric (CSS Fonts 4) with
// their OpenType features. Keywords of one group exclude each other.
var numericKeywords = map[string]struct{ feature, group string }{
	"lining-nums":        {"lnum", "figure"},
	"oldstyle-nums":      {"onum", "figure"},
	"proportional-nums":  {"pnum", "spacing"},
	"tabular-nums":       {"tnum", "spacing"},
	"diagonal-fractions": {"frac", "fraction"},
	"stacked-fractions":  {"afrc", "fraction"},
	"ordinal":            {"ordn", "ordinal"},
	"slashed-zero":       {"zero", "zero"},
}

// parseFontVariantNumeric returns the features of a value of
// font-variant-numeric and whether the value is valid. An invalid value leaves
// the inherited one in place, which covers inherit, unset and revert.
func parseFontVariantNumeric(v string) ([]string, bool) {
	fields := strings.Fields(strings.ToLower(v))
	if len(fields) == 1 && (fields[0] == "normal" || fields[0] == "initial") {
		return nil, true
	}
	var features []string
	seen := map[string]bool{}
	for _, f := range fields {
		keyword, ok := numericKeywords[f]
		if !ok || seen[keyword.group] {
			return nil, false
		}
		seen[keyword.group] = true
		features = append(features, keyword.feature+"=1")
	}
	return features, len(features) > 0
}
