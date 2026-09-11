package htmlbag

import (
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
//   - One band at a time: a second float opening while a band is live replaces
//     it rather than stacking beside it, so two floats on the same side overlap.
//   - A float needs a declared width (see buildFloat).

// floatGutter is the space between a float and the text beside it when the float
// declares no margin of its own. CSS has no default here, but a picture butting
// against the text reads as a mistake rather than as a layout.
const floatGutter = 9 * bag.Factor

// floatBand is the vertical extent a float still covers.
type floatBand struct {
	side      string          // "left" or "right"
	inset     bag.ScaledPoint // what content has to give up to clear the float
	remaining bag.ScaledPoint // float height not yet passed
}

func floatSideOf(itm any) (string, bool) {
	switch t := itm.(type) {
	case *frontend.Text:
		side, ok := t.Settings[settingFloat].(string)
		return side, ok
	case node.Node:
		v, ok := t.GetAttribute(attrFloat)
		if !ok {
			return "", false
		}
		side, ok := v.(string)
		return side, ok
	}
	return "", false
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
		delete(t.Settings, settingFloat)
		// The float is built at the container's width, so a float that declares
		// no width of its own fills the measure and leaves nothing beside it.
		// CSS 2.1 §10.3.5 shrinks it to fit its content instead; that needs a
		// measuring pass this does not do yet, so a float needs a width.
		return cb.CreateVlist(t, wd)
	case node.Node:
		// A replaced element (an image) is already sized. Unlink it first:
		// Vpack packs from the node it is given to the end of the list, which
		// would take the rest of the content into the float box with it.
		t.SetNext(nil)
		t.SetPrev(nil)
		return node.Vpack(t), nil
	}
	return nil, nil
}

func openBand(vls *node.VList, box *node.VList, side string, wd bag.ScaledPoint) *floatBand {
	height := box.Height + box.Depth
	if side == "right" {
		box.ShiftX = wd - box.Width
	}
	// Shift is a pure rendering offset, so the box paints downward from the
	// cursor without the parent reserving anything for it. The band is what
	// keeps the following content clear.
	box.Shift = -height
	box.Height, box.Depth = 0, 0
	if box.Attributes == nil {
		box.Attributes = node.H{}
	}
	box.Attributes["origin"] = "float"
	vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), box)
	return &floatBand{side: side, inset: box.Width + floatGutter, remaining: height}
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

func (b *floatBand) consume(height bag.ScaledPoint) bool {
	b.remaining -= height
	return b.remaining > 0
}

// floatIndentFor turns a band into the linebreaker's per-row inset. It consumes
// the sentinels, which FormatParagraph would otherwise reject as unknown.
func floatIndentFor(settings frontend.TypesettingSettings) (inset bag.ScaledPoint, rows int, side string) {
	// float and clear describe the element itself, not its lines. They are
	// stamped by ApplySettings on every element that declares them, so they have
	// to be dropped here whether or not this paragraph is inside a band —
	// FormatParagraph rejects settings it does not know.
	delete(settings, settingFloat)
	delete(settings, settingClear)

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

func setFloatBand(settings frontend.TypesettingSettings, inset, height bag.ScaledPoint, side string) {
	settings[settingFloatInset] = inset
	settings[settingFloatHeight] = height
	settings[settingFloatSide] = side
}
