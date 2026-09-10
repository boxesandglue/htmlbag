package htmlbag

import (
	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
)

// Box model trace overlay (CSSBuilder.TraceBoxModel): translucent fills in
// the devtools color scheme that visualise margin, border, padding and
// content box of block-level elements. Each layer is painted as the ring
// between two neighbouring boxes, so within one element no area is tinted
// twice. Overlapping rings of *different* elements (collapsed margins of
// vertical neighbours) tint twice and show up darker — alpha compositing
// is sublinear (n layers give 1-(1-a)^n coverage), so stacking never
// saturates to full opacity.

// traceRect is an axis-aligned rectangle in the coordinate system of the
// rule node that paints the overlay (PDF user space, y up).
type traceRect struct {
	llx, lly, urx, ury bag.ScaledPoint
}

func (r traceRect) valid() bool { return r.urx > r.llx && r.ury > r.lly }

// grow returns the rectangle enlarged by the given amounts per side.
// Negative values shrink; a degenerate result is caught by valid().
func (r traceRect) grow(top, right, bottom, left bag.ScaledPoint) traceRect {
	return traceRect{llx: r.llx - left, lly: r.lly - bottom, urx: r.urx + right, ury: r.ury + top}
}

// Devtools-like layer colors.
var (
	traceMarginColor  = color.Color{Space: color.ColorRGB, R: 0.965, G: 0.698, B: 0.420} // #f6b26b
	traceBorderColor  = color.Color{Space: color.ColorRGB, R: 0.992, G: 0.867, B: 0.486} // #fddd7c
	tracePaddingColor = color.Color{Space: color.ColorRGB, R: 0.576, G: 0.769, B: 0.490} // #93c47d
	traceContentColor = color.Color{Space: color.ColorRGB, R: 0.435, G: 0.659, B: 0.863} // #6fa8dc
)

// traceBoxModelAlpha is the constant fill alpha of the overlay.
const traceBoxModelAlpha = 0.4

func traceDrawRect(draw *pdfdraw.Object, rc traceRect) {
	draw.Rect(rc.llx, rc.lly, rc.urx-rc.llx, rc.ury-rc.lly)
}

// boxModelOverlayRule builds the hidden rule that paints the overlay. The
// four rectangles must be nested (margin ⊇ border ⊇ padding ⊇ content).
// When transparent is false (the document format forbids transparency,
// e.g. PDF/A-1 or PDF/X-3) the overlay falls back to thin colored
// outlines instead of translucent fills.
func boxModelOverlayRule(margin, border, padding, content traceRect, transparent bool) *node.Rule {
	r := node.NewRule()
	r.Hide = true
	draw := pdfdraw.NewStandalone()
	if transparent {
		alpha := float64(traceBoxModelAlpha)
		gs := pdf.ExtGState{FillAlpha: &alpha}
		draw.GState(string(gs.ResourceName()))
		// ring fills the area of outer not covered by inner (even-odd
		// rule). Equal rectangles produce nothing; an empty inner fills
		// outer completely.
		ring := func(outer, inner traceRect, col color.Color) {
			if !outer.valid() || inner == outer {
				return
			}
			draw.ColorNonstroking(col)
			traceDrawRect(draw, outer)
			if inner.valid() {
				traceDrawRect(draw, inner)
				draw.FillEvenOdd()
			} else {
				draw.Fill()
			}
		}
		ring(margin, border, traceMarginColor)
		ring(border, padding, traceBorderColor)
		ring(padding, content, tracePaddingColor)
		ring(content, traceRect{}, traceContentColor)
		r.Attributes = node.H{
			"origin":     "boxmodel trace",
			"extgstates": []pdf.ExtGState{gs},
		}
	} else {
		draw.LineWidth(bag.ScaledPointFromFloat(0.5))
		outline := func(rc, inner traceRect, col color.Color) {
			if !rc.valid() || rc == inner {
				return
			}
			draw.ColorStroking(col)
			traceDrawRect(draw, rc)
			draw.Stroke()
		}
		outline(margin, border, traceMarginColor)
		outline(border, padding, traceBorderColor)
		outline(padding, content, tracePaddingColor)
		outline(content, traceRect{}, traceContentColor)
		r.Attributes = node.H{"origin": "boxmodel trace"}
	}
	r.Pre = "q " + draw.String() + " Q"
	return r
}

// traceBoxModel prepends the box model overlay to vl's node list. The
// overlay rule paints at the top-left corner of vl's content, so all
// rectangles are expressed relative to that origin: the content box spans
// vl's own dimensions, padding/border/margin grow outward from it per hv.
// Called from HTMLBorder (bordered/background blocks, before the padding
// and border wrapping changes vl) and from the borderless leaf path in
// buildVListInternal.
//
// paddingInside marks padding that is already part of vl's dimensions
// (the borderless leaf reserves vertical padding as kerns inside the
// list): those sides shrink the content box inward instead of growing the
// padding box outward.
func (cb *CSSBuilder) traceBoxModel(vl *node.VList, hv HTMLValues, paddingInside bool) {
	content := traceRect{llx: 0, lly: -(vl.Height + vl.Depth), urx: vl.Width, ury: 0}
	padding := content
	if paddingInside {
		content = content.grow(-hv.PaddingTop, 0, -hv.PaddingBottom, 0)
	} else {
		padding = content.grow(hv.PaddingTop, hv.PaddingRight, hv.PaddingBottom, hv.PaddingLeft)
	}
	border := padding.grow(hv.BorderTopWidth, hv.BorderRightWidth, hv.BorderBottomWidth, hv.BorderLeftWidth)
	margin := border.grow(hv.MarginTop, hv.MarginRight, hv.MarginBottom, hv.MarginLeft)

	transparent := cb.frontend.Doc.Format.AllowsTransparency()
	tr := boxModelOverlayRule(margin, border, padding, content, transparent)
	if vl.List == nil {
		vl.List = tr
		return
	}
	// Keep an existing background rule (inserted by HTMLBorder) below the
	// overlay so the overlay tint stays visible on colored blocks.
	if first, ok := vl.List.(*node.Rule); ok && first.Attributes != nil {
		if origin, ok := first.Attributes["origin"].(string); ok && origin == "html background color" {
			vl.List = node.InsertAfter(vl.List, first, tr)
			return
		}
	}
	vl.List = node.InsertBefore(vl.List, vl.List, tr)
}
