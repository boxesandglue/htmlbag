package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
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
