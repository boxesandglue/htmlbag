package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// textBoxTrim is CSS Inline 3 text-box-trim: whether a block's first line
// is trimmed at its start and its last line at its end.
type textBoxTrim struct{ start, end bool }

// parseTextBoxTrim reads a text-box-trim value; an unknown one keeps cur.
func parseTextBoxTrim(v string, cur textBoxTrim) textBoxTrim {
	switch strings.TrimSpace(v) {
	case "none":
		return textBoxTrim{}
	case "trim-start":
		return textBoxTrim{start: true}
	case "trim-end":
		return textBoxTrim{end: true}
	case "trim-both":
		return textBoxTrim{start: true, end: true}
	}
	bag.Logger.Warn("unknown text-box-trim value", "value", v)
	return cur
}

// parseTextBox reads the text-box shorthand: normal, or a text-box-trim and
// a text-box-edge value in either order, a missing trim being trim-both.
func parseTextBox(v string, cur textBoxTrim) textBoxTrim {
	fields := strings.Fields(v)
	if len(fields) == 1 && fields[0] == "normal" {
		return textBoxTrim{}
	}
	trim := textBoxTrim{start: true, end: true}
	var edge []string
	for _, f := range fields {
		if strings.HasPrefix(f, "trim-") || f == "none" {
			trim = parseTextBoxTrim(f, cur)
		} else {
			edge = append(edge, f)
		}
	}
	if len(edge) > 0 {
		checkTextBoxEdge(strings.Join(edge, " "))
	}
	return trim
}

// checkTextBoxEdge warns about a text-box-edge other than text (or auto,
// which is text here): the lines record their trims at the text edges.
func checkTextBoxEdge(v string) {
	switch strings.Join(strings.Fields(v), " ") {
	case "auto", "text", "text text":
		return
	}
	bag.Logger.Warn("text-box-edge: only text is supported, the text edges are used", "value", v)
}

// trimLines takes the trims bag records on the lines (node.LineTrimStart
// and node.LineTrimEnd) off a block's first line at its start and its last
// line at its end, as trim says, so the block's box ends at its text (CSS
// Inline 3 text-box-trim with text-box-edge: text). bag records them only
// for a Text with frontend.SettingRecordLineTrims, which Output sets. A
// negative trim, lines set tighter than the font's content area, moves the
// edge out to the text, so the block grows.
func trimLines(vl *node.VList, trim textBoxTrim) {
	if vl == nil || trim == (textBoxTrim{}) {
		return
	}
	var first, last *node.HList
	for n := vl.List; n != nil; n = n.Next() {
		if hl, ok := n.(*node.HList); ok {
			if first == nil {
				first = hl
			}
			last = hl
		}
	}
	if first == nil {
		return
	}
	changed := false
	if t, ok := first.Attributes[node.LineTrimStart].(bag.ScaledPoint); ok && trim.start {
		first.Height -= t
		changed = true
	}
	if t, ok := last.Attributes[node.LineTrimEnd].(bag.ScaledPoint); ok && trim.end {
		last.Depth -= t
		changed = true
	}
	if changed {
		packed := node.Vpack(vl.List)
		vl.Height, vl.Depth = packed.Height, packed.Depth
	}
}

// passTrimDown hands the text-box-trim of a block container on to the block
// that holds its first or last formatted line (CSS Inline 3): its first or
// last in-flow child, and from a container on through the same call when
// that one is built. Padding or a border on the child's side lies between
// the container and the line and ends the trim's reach there. A table or a
// pre-rendered box holds no formatted line of the container: it takes the
// trim, which has no effect on it, and the trim reaches no further. Floats
// are not in flow and are passed over.
func (cb *CSSBuilder) passTrimDown(te *frontend.Text) {
	trim := cb.trims[te]
	if trim.start {
		if c := edgeChild(te.Items, false); c != nil && !hasEdgeSpace(c.Settings, frontend.SettingPaddingTop, frontend.SettingBorderTopWidth) {
			cb.addTrim(c, textBoxTrim{start: true})
		}
	}
	if trim.end {
		if c := edgeChild(te.Items, true); c != nil && !hasEdgeSpace(c.Settings, frontend.SettingPaddingBottom, frontend.SettingBorderBottomWidth) {
			cb.addTrim(c, textBoxTrim{end: true})
		}
	}
}

// edgeChild returns the first in-flow block among items, the last one when
// last is set, nil when there is none.
func edgeChild(items []any, last bool) *frontend.Text {
	for i := range items {
		if last {
			i = len(items) - 1 - i
		}
		t, ok := items[i].(*frontend.Text)
		if !ok {
			continue
		}
		if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
			continue
		}
		if _, _, isFloat := floatSideOf(t); isFloat {
			continue
		}
		return t
	}
	return nil
}

// hasEdgeSpace reports whether settings give a block padding or a border on
// one side, named by its padding and border width keys.
func hasEdgeSpace(settings frontend.TypesettingSettings, padding, border frontend.SettingType) bool {
	for _, k := range []frontend.SettingType{padding, border} {
		if v, ok := settings[k].(bag.ScaledPoint); ok && v != 0 {
			return true
		}
	}
	return false
}

// addTrim adds trim to what te trims and has bag record the trims on its
// lines.
func (cb *CSSBuilder) addTrim(te *frontend.Text, trim textBoxTrim) {
	if cb.trims == nil {
		cb.trims = map[*frontend.Text]textBoxTrim{}
	}
	cur := cb.trims[te]
	cb.trims[te] = textBoxTrim{start: cur.start || trim.start, end: cur.end || trim.end}
	te.Settings[frontend.SettingRecordLineTrims] = true
}
