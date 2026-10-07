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
		first.Attributes[attrTrimmedStart] = true
		changed = true
	}
	if t, ok := last.Attributes[node.LineTrimEnd].(bag.ScaledPoint); ok && trim.end {
		last.Depth -= t
		last.Attributes[attrTrimmedEnd] = true
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
	// A table built as a container, as one in a table cell is, passes on
	// nothing either.
	if dbg, _ := te.Settings[frontend.SettingDebug].(string); dbg == "table" {
		return
	}
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

// fragmentTrim is the text-box-trim every fragment of the split leaf block
// vl takes at a break: under box-decoration-break: clone each fragment is
// trimmed, its last line at its end and its first at its start. Under slice
// it is none, as only the block's own first and last line are (trimLines).
// A trim handed down by a container is the container's first or last line
// only, not every fragment's.
func (cb *CSSBuilder) fragmentTrim(vl *node.VList) textBoxTrim {
	if _, _, clone := decorationClone(vl); !clone {
		return textBoxTrim{}
	}
	te, _ := vl.Attributes["_splittableTe"].(*frontend.Text)
	return cb.ownTrims[te]
}

// lineTrimEnd is the trim bag recorded at the end of n, a line, or 0 when
// it is not a line or its end is trimmed already.
func lineTrimEnd(n node.Node) bag.ScaledPoint {
	hl, ok := n.(*node.HList)
	if !ok {
		return 0
	}
	if done, _ := hl.Attributes[attrTrimmedEnd].(bool); done {
		return 0
	}
	t, _ := hl.Attributes[node.LineTrimEnd].(bag.ScaledPoint)
	return t
}

// attrTrimmedStart and attrTrimmedEnd mark a line whose start or end is
// trimmed already, so it is not trimmed twice.
const (
	attrTrimmedStart = "_trimmedStart"
	attrTrimmedEnd   = "_trimmedEnd"
)

// trimFragmentStart trims the start of the first line of items, the start
// of a fragment after a break, once.
func trimFragmentStart(items []node.Node) {
	for _, n := range items {
		hl, ok := n.(*node.HList)
		if !ok {
			if isContentNode(n) {
				return
			}
			continue
		}
		if done, _ := hl.Attributes[attrTrimmedStart].(bool); done {
			return
		}
		if t, ok := hl.Attributes[node.LineTrimStart].(bag.ScaledPoint); ok {
			hl.Height -= t
			hl.Attributes[attrTrimmedStart] = true
		}
		return
	}
}

// trimFragmentEnd trims the end of the last line of items, the end of a
// fragment before a break.
func trimFragmentEnd(items []node.Node) {
	for j := len(items) - 1; j >= 0; j-- {
		hl, ok := items[j].(*node.HList)
		if !ok {
			if isContentNode(items[j]) {
				return
			}
			continue
		}
		if t := lineTrimEnd(hl); t != 0 {
			hl.Depth -= t
			hl.Attributes[attrTrimmedEnd] = true
		}
		return
	}
}

// pullBackGrownLine moves the break before the last line of batch while that
// line, trimmed at the break by a negative amount (lines set tighter than
// the font's content area), grows past room.
func pullBackGrownLine(children, batch []node.Node, next int, room bag.ScaledPoint) ([]node.Node, int) {
	for countContent(batch) > 1 {
		j := len(batch) - 1
		for j >= 0 && !isContentNode(batch[j]) {
			j--
		}
		t := lineTrimEnd(batch[j])
		if t >= 0 || childrenHeight(batch[:j+1])-t <= room {
			break
		}
		next -= len(batch) - j
		batch = batch[:j]
	}
	return batch, next
}
