package htmlbag

import (
	"slices"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	scanner "github.com/speedata/css"
)

// A named string (CSS GCPM) copies text of the document into the page margin.
// string-set gives a name its value at an element, string() in a margin box
// reads it. Which value a page shows depends on the assignments on that page,
// so they are collected as the page is filled. The margin boxes come after
// that, at shipout, so a named string needs no second pass.

// pageString is one string-set assignment.
type pageString struct {
	name, value string
	// atTop is set for an assignment at the very top of its page, which
	// string(name, start) counts as the page's own.
	atTop bool
}

// splitAtCommas splits a value at its top-level commas, a function call stays
// in one piece.
func splitAtCommas(toks tokenstream) []tokenstream {
	var parts []tokenstream
	var current tokenstream
	depth := 0
	for _, tok := range toks {
		switch {
		case tok.Type == scanner.Function:
			depth++
		case tok.Type == scanner.Delim && tok.Value == ")" && depth > 0:
			depth--
		case tok.Type == scanner.Delim && tok.Value == "," && depth == 0:
			parts = append(parts, current)
			current = nil
			continue
		}
		current = append(current, tok)
	}
	return append(parts, current)
}

// stringAssignments evaluates the string-set of item. The value is fixed here,
// where the counters and the attributes of the element are at hand.
func (cb *CSSBuilder) stringAssignments(item *HTMLItem, ss StylesStack, anchorPages map[string]int) []pageString {
	v, ok := item.Styles["string-set"]
	if !ok || strings.TrimSpace(v.String()) == "none" || item.Styles.Get("display") == "none" {
		return nil
	}
	attrLookup := func(name string) string { return item.Attributes[name] }
	var assigned []pageString
	for _, part := range splitAtCommas(v.tokens()) {
		components := splitComponents(part)
		if len(components) < 2 {
			bag.Logger.Warn("string-set needs a name and a value, ignoring it", "value", part.String())
			continue
		}
		var rest tokenstream
		for _, c := range components[1:] {
			rest = append(rest, c...)
		}
		tokens := parseContentTokens(rest)
		for i, tok := range tokens {
			if tok.Type != ContentContent {
				continue
			}
			if tok.Value != "text" {
				bag.Logger.Warn("string-set: only content() and content(text) are supported, ignoring it", "value", tok.Value)
			}
			text := ""
			if tok.Value == "text" {
				text = strings.Join(strings.Fields(extractTextFromHTMLItem(item)), " ")
			}
			tokens[i] = ContentToken{Type: ContentString, Value: text}
		}
		assigned = append(assigned, pageString{
			name:  components[0].String(),
			value: evaluateContentWithStack(tokens, ss, anchorPages, cb.anchorTexts, cb.anchorCounters, attrLookup),
		})
	}
	return assigned
}

// setStrings registers the string-set of item and returns the marker that
// takes it to the page of the element, or false if it has none. The marker
// travels as an anchor does, through line breaking, splits and rebuilds.
func (cb *CSSBuilder) setStrings(item *HTMLItem, ss StylesStack, anchorPages map[string]int) (anchorMarker, bool) {
	assigned := cb.stringAssignments(item, ss, anchorPages)
	if len(assigned) == 0 {
		return anchorMarker{}, false
	}
	cb.Anchors = append(cb.Anchors, AnchorEntry{assigned: assigned})
	m := anchorMarker{Idx: cb.anchorCount}
	cb.anchorCount++
	return m, true
}

// tableParts are the elements of a table that hold other elements, not text.
var tableParts = []string{"table", "thead", "tbody", "tfoot", "tr", "td", "th"}

// plantMarker puts m in the first paragraph inside te, where the paragraph
// builder finds it. A block that holds blocks has no use for it, and the parts
// of a table take elements only, so a table without text has no place for it.
func plantMarker(te *frontend.Text, m anchorMarker) {
	for {
		box, _ := te.Settings[frontend.SettingBox].(bool)
		tag, _ := te.Settings[frontend.SettingDebug].(string)
		inTable := slices.Contains(tableParts, tag)
		if !box && !inTable {
			break
		}
		var child *frontend.Text
		for _, itm := range te.Items {
			if t, ok := itm.(*frontend.Text); ok && !isWhitespaceOnly(t) {
				child = t
				break
			}
		}
		if child == nil {
			if inTable {
				bag.Logger.Warn("string-set on a table without text, ignoring it")
				return
			}
			break
		}
		te = child
	}
	te.Items = slices.Insert(te.Items, 0, any(m))
}

// recordStrings files the assignments of anchor idx under their page. An
// element split across pages assigns once, on the page it starts on.
func (cb *CSSBuilder) recordStrings(idx, pageNum int) {
	strs := cb.Anchors[idx].assigned
	if len(strs) == 0 {
		return
	}
	cb.Anchors[idx].assigned = nil
	if cb.pageStrings == nil {
		cb.pageStrings = map[int][]pageString{}
	}
	for _, s := range strs {
		s.atTop = cb.markAtPageTop
		cb.pageStrings[pageNum] = append(cb.pageStrings[pageNum], s)
	}
}

// namedString is the value of string(name, position) on page: first, the
// default, is the first assignment on the page, start one at the very top of
// it, last the last one, and first-except none on a page with an assignment.
// Without one on the page the value carries over from the pages before.
func (cb *CSSBuilder) namedString(name, position string, page int) string {
	var own []string
	atTop := false
	for _, s := range cb.pageStrings[page] {
		if s.name == name {
			if len(own) == 0 {
				atTop = s.atTop
			}
			own = append(own, s.value)
		}
	}
	entry := ""
find:
	for p := page - 1; p > 0; p-- {
		strs := cb.pageStrings[p]
		for i := len(strs) - 1; i >= 0; i-- {
			if strs[i].name == name {
				entry = strs[i].value
				break find
			}
		}
	}
	if len(own) == 0 {
		return entry
	}
	switch position {
	case "start":
		if atTop {
			return own[0]
		}
		return entry
	case "last":
		return own[len(own)-1]
	case "first-except":
		return ""
	}
	return own[0]
}
