package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// CSS floats that text flows beside (CSS 2.1 §9.5), as opposed to the
// paged-media floats in insert.go, which are lifted out of the flow to a page
// edge.
//
// A float is taken out of its container's vertical stacking and painted at the
// point it appeared. What it leaves behind is a BAND: a vertical extent, as tall
// as the float, over which the content that follows has to keep clear of it.
// Every following child consumes part of the band and narrows itself by the
// float's width until the band is used up, after which content runs full width
// again. `clear` ends the band early.
//
// Two deliberate simplifications, both visible only with borders or backgrounds:
//
//   - A following block box is narrowed wholesale, as if it established a block
//     formatting context, rather than having only its line boxes shortened while
//     its border and background run on under the float. The narrowing is applied
//     as the paragraph indent, so a box's own border is drawn at the narrowed
//     width.
//   - The container is extended to hold a float taller than the content beside
//     it, rather than letting it overflow as a browser does. This engine does not
//     paint out-of-flow content over the following flow (see settingCSSHeight),
//     and a float hanging out of its container has nothing sensible to do at a
//     page break.
//
// Known limits, all of them narrowings rather than wrong answers:
//
//   - A float is recognised only as a direct child of a block container. One
//     written inside a paragraph's inline content is left in flow; lifting it to
//     the container is a separate change.
//   - One band at a time: a float opening while a band is live starts below it
//     rather than beside it, so two floats never overlap but neither do they sit
//     side by side as a browser would place them.
//   - A float needs a declared width (see buildFloat).
//   - A margin declared as zero gets the default gutter, because a margin is
//     stamped on every element whether or not it was written.
//   - A negative margin is not what CSS would do with it: on the text side it
//     falls back to the gutter (the margin has to beat zero to be used), and on
//     the far side it flows into the shift and the inset unfiltered, pulling the
//     float outside its container rather than overlapping the text beside it.
//     Below the float it shortens the band, down to nothing at all, but never
//     past the float box, which the container holds whatever the margin says.
//   - Pagination runs through a band: the float box reserves no vertical space,
//     so a float near the bottom of a page paints past the page edge and the
//     lines it shortened continue on the next page beside nothing.

// floatGutter is the space between a float and the text beside it when the float
// declares no margin of its own. CSS has no default here, but a picture butting
// against the text reads as a mistake rather than as a layout.
//
// A declared margin replaces it. Zero cannot: margins are stamped on every
// element whether or not they were written, so "margin: 0" and "no margin at
// all" arrive here as the same thing, and the second is much the commoner.
const floatGutter = 9 * bag.Factor

// floatMargins is what a float holds clear around itself.
type floatMargins struct{ left, right, top, bottom bag.ScaledPoint }

// marginsOf reads the margins declared on the float.
//
// Two places to read them from, because a float arrives as one of two things. An
// element is a frontend.Text and carries its margins in its settings. A replaced
// element never becomes one: it is a node, and the anonymous inline run it
// arrives in is not it — that run's margins are its own, which is to say zeros,
// so an image read through it holds no more space than its own box. Its margins
// are stamped on the node beside the side it floats to (see attrFloatMargins).
func marginsOf(itm any) floatMargins {
	if n, ok := itm.(node.Node); ok {
		m, _ := n.GetAttribute(attrFloatMargins)
		fm, _ := m.(floatMargins)
		return fm
	}
	t, ok := itm.(*frontend.Text)
	if !ok {
		return floatMargins{}
	}
	sp := func(key frontend.SettingType) bag.ScaledPoint {
		v, _ := t.Settings[key].(bag.ScaledPoint)
		return v
	}
	return floatMargins{
		left:   sp(frontend.SettingMarginLeft),
		right:  sp(frontend.SettingMarginRight),
		top:    sp(frontend.SettingMarginTop),
		bottom: sp(frontend.SettingMarginBottom),
	}
}

// floatMargins reads the margins the style resolution has already worked out, so
// a replaced element can carry them on its node.
func (fs *FormattingStyles) floatMargins() floatMargins {
	return floatMargins{
		left:   fs.marginLeft,
		right:  fs.marginRight,
		top:    fs.marginTop,
		bottom: fs.marginBottom,
	}
}

// gutter is the space between the float and the text: the margin on the side
// the text is on, or the default where none was declared.
func (m floatMargins) gutter(side string) bag.ScaledPoint {
	declared := m.right
	if side == "right" {
		declared = m.left
	}
	if declared > 0 {
		return declared
	}
	return floatGutter
}

// floatBand is the vertical extent a float still covers.
type floatBand struct {
	side      string          // "left" or "right"
	inset     bag.ScaledPoint // what content has to give up to clear the float
	remaining bag.ScaledPoint // band height not yet passed
	// boxRemaining is the float box's own extent not yet passed. It is the band
	// itself until a negative bottom margin shortens the band below the box: the
	// text then runs full width again sooner, but the container still has to hold
	// the box, since this engine does not paint out-of-flow content over the
	// following flow.
	boxRemaining bag.ScaledPoint
	// inherited marks a band that belongs to an ancestor: the float is painted
	// and extended by whoever opened it, and this container only narrows the
	// children the band still covers.
	inherited bool
}

// floatSideOf reports the float a container child is, if it is one. A replaced
// element arrives wrapped in the anonymous inline run its siblings share, so a
// run holding nothing but a floated node is unwrapped to the node itself —
// otherwise an `<img style="float:left">` beside a block would never be seen as
// a float at all.
func floatSideOf(itm any) (string, any, bool) {
	switch t := itm.(type) {
	case *frontend.Text:
		if side, ok := t.Settings[settingFloat].(string); ok {
			return side, t, true
		}
		if inner, ok := soleItem(t); ok {
			if n, isNode := inner.(node.Node); isNode {
				if side, ok := nodeFloatSide(n); ok {
					return side, n, true
				}
			}
		}
	case node.Node:
		if side, ok := nodeFloatSide(t); ok {
			return side, t, true
		}
	}
	return "", nil, false
}

func nodeFloatSide(n node.Node) (string, bool) {
	v, ok := n.GetAttribute(attrFloat)
	if !ok {
		return "", false
	}
	side, ok := v.(string)
	return side, ok
}

// soleItem returns a Text's only item, ignoring whitespace either side of it.
func soleItem(t *frontend.Text) (any, bool) {
	var found any
	for _, itm := range t.Items {
		if s, isStr := itm.(string); isStr && strings.TrimSpace(s) == "" {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = itm
	}
	return found, found != nil
}

func clearsBand(itm any, side string) bool {
	t, ok := itm.(*frontend.Text)
	if !ok {
		return false
	}
	clear, ok := t.Settings[settingClear].(string)
	if !ok {
		return false
	}
	return clear == "both" || clear == side
}

// buildFloat formats a float child. The result reserves no vertical space: it is
// painted from the cursor downwards and the band is what holds content clear.
func (cb *CSSBuilder) buildFloat(itm any, wd bag.ScaledPoint) (*node.VList, error) {
	switch t := itm.(type) {
	case *frontend.Text:
		// Captured and restored, not consumed: a second formatting pass (a table
		// cell's min/max/final measurements, a page-width reflow) walks the same
		// Text again, and a float stripped of what makes it a float renders in
		// flow the second time round.
		defer captureFloatSettings(t.Settings)()
		// The float is built at the container's width, so a float that declares
		// no width of its own fills the measure and leaves nothing beside it.
		// CSS 2.1 §10.3.5 shrinks it to fit its content instead; that needs a
		// measuring pass this does not do yet, so a float needs a width.
		return cb.CreateVlist(t, wd)
	case node.Node:
		// A replaced element whose size is a percentage of its containing block
		// has not been sized yet: the pass that resolves those runs on a
		// paragraph's items, and a float never reaches it. Left unresolved, an
		// image wider than the measure becomes a float wider than the page, with
		// nothing but negative space beside it.
		resolveDeferredSizing([]any{t}, wd)
		// A replaced element is sized by now. Unlink it first: Vpack packs from
		// the node it is given to the end of the list, which would take the rest
		// of the content into the float box with it.
		t.SetNext(nil)
		t.SetPrev(nil)
		return node.Vpack(t), nil
	}
	return nil, nil
}

func openBand(vls *node.VList, box *node.VList, side string, wd bag.ScaledPoint, m floatMargins) *floatBand {
	// The margin above the float is space the float itself takes: packed on top
	// of the box, it pushes the float down the page and the band with it.
	if m.top > 0 {
		k := node.NewKern()
		k.Kern = m.top
		k.Attributes = node.H{"origin": "float-margin"}
		// Built by hand rather than with Vpack: a kern reports its size as a
		// width whichever way it is packed, so a vertical one has to be added to
		// the height itself — as every other kern in this package is.
		wrapper := node.NewVList()
		wrapper.List = node.InsertAfter(k, k, box)
		wrapper.Width = box.Width
		wrapper.Height = box.Height + box.Depth + m.top
		// Everything the box carried moves to the wrapper: a footnote raised out
		// of the float, and the _splittable family that lets it break across a
		// page, are read off the one node in the list without recursing into it.
		// Copied rather than shared, so marking the wrapper does not mark the box
		// inside it as a second float.
		wrapper.Attributes = node.H{}
		for key, value := range box.Attributes {
			wrapper.Attributes[key] = value
		}
		box = wrapper
	}
	boxHeight := box.Height + box.Depth
	// A negative bottom margin pulls the text up beside the float: the band ends
	// that much sooner, and a margin deeper than the float ends it at once. It
	// does not pull the float up with it, so what the container still has to hold
	// is tracked apart from the band (see floatBand.boxRemaining). A band shorter
	// than nothing is simply spent: every reader of remaining tests it against
	// zero, and clamping it here would be a branch nothing can observe.
	height := boxHeight + m.bottom
	width := box.Width
	if side == "right" {
		box.ShiftX = wd - width - m.right
	} else {
		box.ShiftX = m.left
	}
	// Zero height is what takes the box out of the vertical flow: the parent
	// reserves nothing for it and the following content is held clear by the
	// band instead. A box with no height above its reference point already hangs
	// below it, so no shift is wanted on top of that.
	box.Height, box.Depth = 0, 0
	if box.Attributes == nil {
		box.Attributes = node.H{}
	}
	box.Attributes["origin"] = "float"
	vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), box)
	// What the content beside it has to give up: the float, the margin between
	// the two, and the margin on the far side, which is space the float holds
	// against the container edge rather than against the text.
	inset := width + m.gutter(side)
	if side == "right" {
		inset += m.right
	} else {
		inset += m.left
	}
	return &floatBand{side: side, inset: inset, remaining: height, boxRemaining: boxHeight}
}

// narrow marks a child as sitting in the band. The row count is left to the
// paragraph: a container's child does not yet carry the leading its lines will
// be set at, and the count is the band's height divided by exactly that.
func (b *floatBand) narrow(itm any) {
	t, ok := itm.(*frontend.Text)
	if !ok || b.remaining <= 0 {
		return
	}
	setFloatBand(t.Settings, b.inset, b.remaining, b.side)
}

// clearBandStamp wipes any band a previous pass stamped on a child, so that a
// child no longer covered by a float is not indented by a leftover.
func clearBandStamp(itm any) {
	if t, ok := itm.(*frontend.Text); ok {
		clearFloatBand(t.Settings)
	}
}

// gap is what still has to be left below the content beside the float: the band
// the text clears, or the float box itself where a negative bottom margin made
// the band the shorter of the two.
func (b *floatBand) gap() bag.ScaledPoint {
	if b.boxRemaining > b.remaining {
		return b.boxRemaining
	}
	return b.remaining
}

func (b *floatBand) consume(height bag.ScaledPoint) bool {
	b.remaining -= height
	b.boxRemaining -= height
	// The band outlives its text side: a float whose band a negative margin cut
	// short is still a box the container has to make room for.
	return b.gap() > 0
}

// floatIndentFor turns a band into the linebreaker's per-row inset, consuming
// the band as it goes: the band is derived afresh by the container on every
// formatting pass, so a stamp left behind would be a phantom indent the next
// time this paragraph is measured. The author's own float/clear settings are
// not touched here — see stripFloatSettings.
func floatIndentFor(settings frontend.TypesettingSettings) (inset bag.ScaledPoint, rows int, side string) {
	raw, height, side := floatBandOf(settings)
	if raw <= 0 {
		return 0, 0, ""
	}

	leading := paragraphLeading(settings)
	if leading <= 0 || height <= 0 {
		return 0, 0, ""
	}
	rows = int(height / leading)
	if height%leading != 0 {
		rows++
	}
	return raw, rows, side
}

// paragraphLeading is what a float's height is divided by to count the lines it
// covers.
func paragraphLeading(settings frontend.TypesettingSettings) bag.ScaledPoint {
	if l, ok := settings[frontend.SettingLeading].(bag.ScaledPoint); ok && l > 0 {
		return l
	}
	if fs, ok := settings[frontend.SettingSize].(bag.ScaledPoint); ok && fs > 0 {
		return fs * 12 / 10
	}
	return 0
}

func floatBandOf(settings frontend.TypesettingSettings) (inset bag.ScaledPoint, height bag.ScaledPoint, side string) {
	inset, _ = settings[settingFloatInset].(bag.ScaledPoint)
	height, _ = settings[settingFloatHeight].(bag.ScaledPoint)
	side, _ = settings[settingFloatSide].(string)
	delete(settings, settingFloatInset)
	delete(settings, settingFloatHeight)
	delete(settings, settingFloatSide)
	return inset, height, side
}

// captureFloatSettings removes the author-declared float and clear sentinels
// from one element's settings and returns the restore. They are captured rather
// than consumed because the same Text is formatted more than once — a table
// cell measures min/max/final, a page-width reflow rebuilds the page — and a
// float stripped of what makes it a float renders in flow the second time.
func captureFloatSettings(settings frontend.TypesettingSettings) func() {
	var saved []struct {
		key   frontend.SettingType
		value any
	}
	for _, key := range [...]frontend.SettingType{settingFloat, settingClear} {
		if v, ok := settings[key]; ok {
			saved = append(saved, struct {
				key   frontend.SettingType
				value any
			}{key, v})
			delete(settings, key)
		}
	}
	return func() {
		for _, s := range saved {
			settings[s.key] = s.value
		}
	}
}

// captureInlineFloatSettings does the same for a paragraph and everything
// inline inside it. A float is only recognised as a child of a block container,
// so one written on a <span> stays in flow — but ApplySettings has still stamped
// the sentinel on that span's Text, which goes straight into FormatParagraph's
// strict switch. Only for a paragraph: on a container this would reach the
// children, and the child that IS the float would stop being one.
func captureInlineFloatSettings(te *frontend.Text) func() {
	var restores []func()
	var walk func(t *frontend.Text)
	walk = func(t *frontend.Text) {
		if t == nil {
			return
		}
		restores = append(restores, captureFloatSettings(t.Settings))
		for _, itm := range t.Items {
			if inner, ok := itm.(*frontend.Text); ok {
				walk(inner)
			}
		}
	}
	walk(te)
	return func() {
		for _, restore := range restores {
			restore()
		}
	}
}

// restoreSettings snapshots the given keys and returns the restore, putting
// back what was there — including nothing at all.
func restoreSettings(settings frontend.TypesettingSettings, keys []frontend.SettingType) func() {
	saved := make([]any, len(keys))
	had := make([]bool, len(keys))
	for i, key := range keys {
		saved[i], had[i] = settings[key]
	}
	return func() {
		for i, key := range keys {
			if had[i] {
				settings[key] = saved[i]
			} else {
				delete(settings, key)
			}
		}
	}
}

// clearFloatBand drops a band stamped by an earlier formatting pass. The stamp
// is derived from where the float actually landed, so it has to be re-derived
// rather than carried: at another page width the same child may be clear of the
// float altogether.
func clearFloatBand(settings frontend.TypesettingSettings) {
	delete(settings, settingFloatInset)
	delete(settings, settingFloatHeight)
	delete(settings, settingFloatSide)
}

func setFloatBand(settings frontend.TypesettingSettings, inset, height bag.ScaledPoint, side string) {
	settings[settingFloatInset] = inset
	settings[settingFloatHeight] = height
	settings[settingFloatSide] = side
}
