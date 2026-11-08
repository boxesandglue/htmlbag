package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// replacedAlign places an image or inline svg in its line by CSS
// vertical-align (CSS 2.1 §10.8.1). The box stands on the baseline: its
// bottom edge at the baseline, the height above. apply moves it by
// splitting its height into a part above and a part below the baseline.
// The line sees the moved box and grows with it, as a line box does in a
// browser.
type replacedAlign struct {
	// keyword is the vertical-align of the element itself: top,
	// text-top, middle, bottom, text-bottom, or empty for the baseline.
	keyword string
	// shift raises the box, negative lowers it: a length or percentage,
	// sub or super, and the shift of the inline elements around it.
	shift bag.ScaledPoint
	// ascent, descent and xHeight are the font of the parent, against
	// which the keywords place the box. Read only with a keyword.
	ascent, descent, xHeight bag.ScaledPoint
}

// newReplacedAlign reads the vertical-align of item. own is the style of
// the element itself, parent that of the element around it. A float or a
// block is not in a line, vertical-align does not apply to it.
func (cb *CSSBuilder) newReplacedAlign(item *HTMLItem, own, parent *FormattingStyles) replacedAlign {
	if own.floatSide != "" || item.Styles.Get("display") == "block" {
		return replacedAlign{}
	}
	a := replacedAlign{shift: own.yoffset}
	switch kw := strings.ToLower(strings.TrimSpace(item.Styles.Get("vertical-align"))); kw {
	case "top", "text-top", "middle", "bottom", "text-bottom":
		// htmlbag has no line box of its own, top and bottom take the
		// font's content area as text-top and text-bottom do.
		a.keyword = kw
		if fnt := cb.strutFont(parent, nil); fnt != nil {
			a.ascent, a.descent = fnt.ContentAscent, fnt.ContentDescent
			if f := fnt.Face.OTFace(); f != nil && f.Upem() > 0 {
				a.xHeight = bag.ScaledPoint(float64(fnt.Size) * float64(f.XHeight()) / float64(f.Upem()))
			}
		} else {
			// Without a font, the proportions of a typical one.
			a.ascent, a.descent, a.xHeight = parent.Fontsize*4/5, parent.Fontsize/5, parent.Fontsize/2
		}
	}
	return a
}

// isBaseline reports whether the box stays where it is.
func (a replacedAlign) isBaseline() bool {
	return a.keyword == "" && a.shift == 0
}

// apply moves a box standing on the baseline, height and depth are its
// extent above and below the baseline.
func (a replacedAlign) apply(height, depth *bag.ScaledPoint) {
	if a.isBaseline() {
		return
	}
	total := *height + *depth
	var above bag.ScaledPoint
	switch a.keyword {
	case "top", "text-top":
		above = a.ascent
	case "bottom", "text-bottom":
		above = total - a.descent
	case "middle":
		// The middle of the box at half the x-height of the parent.
		above = a.xHeight/2 + total/2
	default:
		above = *height
	}
	above += a.shift
	*height, *depth = above, total-above
}

// formatter wraps the deferred formatter of a box so the box it makes is
// moved as well.
func (a replacedAlign) formatter(ftv frontend.FormatToVList) frontend.FormatToVList {
	if a.isBaseline() {
		return ftv
	}
	return func(containerWidth bag.ScaledPoint) (*node.VList, error) {
		vl, err := ftv(containerWidth)
		if err == nil && vl != nil {
			a.apply(&vl.Height, &vl.Depth)
		}
		return vl, err
	}
}
