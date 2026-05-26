package htmlbag

import (
	"sort"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// PositionedInsert is one CSS-positioned element fully resolved to PDF
// page coordinates. Stored in cb.positionedItems for the current page;
// painted in flushInserts between the buffered body and the bottom-
// floats so positioned content overlays in-flow content per CSS 2.1
// Appendix E.
//
// X/Y is the PDF top-left anchor expected by Page.OutputAt (y measured
// from the page bottom upward). Width/Height are the resolved content
// box dimensions. ZIndex/SourceOrder determine the paint order.
type PositionedInsert struct {
	Body        *node.VList
	X           bag.ScaledPoint
	Y           bag.ScaledPoint
	Width       bag.ScaledPoint
	Height      bag.ScaledPoint
	ZIndex      int
	SourceOrder int
}

// isPositionedElement reports whether an HTMLItem is taken out of flow
// by the CSS positioning pipeline. Only `position: absolute` is
// handled in v1; `relative` stays in flow (Phase 4 will treat that
// with a different mechanism), and `fixed`/`sticky` are intentionally
// unimplemented per the positioning plan.
func isPositionedElement(item *HTMLItem) bool {
	if item == nil {
		return false
	}
	switch item.Styles["position"] {
	case "absolute":
		return true
	}
	return false
}

// resolvePositionedRect runs the simplified CSS 2.1 §10.3.7 /
// §10.6.4 width/height/offset resolution for a position: absolute
// element. The full spec algorithm allows shrink-to-fit and circular
// dependencies; v1 requires an explicit width: declaration and, if
// absent, falls back to using the available containing-block width
// minus the left/right offsets — covering the lesson-positioning and
// DIN-5008 cases without invoking the cycle-prone shrink-to-fit
// pass.
//
// The returned struct carries the resolved content-box geometry in
// PDF coordinates (y measured from page bottom upward). Height is 0
// when neither an explicit height nor both top+bottom offsets are
// set — the caller must overwrite it with the formatted body's
// natural height before painting.
func (cb *CSSBuilder) resolvePositionedRect(styles *FormattingStyles) positioningContext {
	parent := cb.currentContainingBlock()
	var rect positioningContext
	// Width.
	switch {
	case styles.width != "":
		rect.width = ParseRelativeSize(styles.width, styles.Fontsize, styles.DefaultFontSize)
	case styles.leftOffset != nil && styles.rightOffset != nil:
		rect.width = parent.width - *styles.leftOffset - *styles.rightOffset
	case styles.leftOffset != nil:
		rect.width = parent.width - *styles.leftOffset
	case styles.rightOffset != nil:
		rect.width = parent.width - *styles.rightOffset
	default:
		rect.width = parent.width
	}
	// Height (resolvable here if explicit or both top+bottom). If
	// only an offset pair without explicit height, we leave height 0
	// and let the caller fill it in from the formatted body.
	switch {
	case styles.height != "":
		rect.height = ParseRelativeSize(styles.height, styles.Fontsize, styles.DefaultFontSize)
	case styles.topOffset != nil && styles.bottomOffset != nil:
		rect.height = parent.height - *styles.topOffset - *styles.bottomOffset
	}
	// X.
	switch {
	case styles.leftOffset != nil:
		rect.x = parent.x + *styles.leftOffset
	case styles.rightOffset != nil:
		rect.x = parent.x + parent.width - *styles.rightOffset - rect.width
	default:
		rect.x = parent.x
	}
	// Y. parent.y is PDF-y of the CB top edge; CSS top counts downward
	// from there. PDF anchor for OutputAt is the top of the box.
	switch {
	case styles.topOffset != nil:
		rect.y = parent.y - *styles.topOffset
	case styles.bottomOffset != nil:
		// rect.height may still be unknown here; caller patches y
		// after the body has been formatted (see handlePositioned).
		rect.y = parent.y - parent.height + *styles.bottomOffset + rect.height
	default:
		rect.y = parent.y
	}
	return rect
}

// handlePositioned takes an HTMLItem with position: absolute, resolves
// its geometry against the current containing block, formats the body
// into a VList, and appends a PositionedInsert to cb.positionedItems
// for painting at flush time. The element is out of flow — the caller
// must NOT add anything to its parent's Items list for this child.
//
// The containing-block stack is pushed with the element's own
// resolved rectangle for the duration of the recursive Output() call
// so any nested positioned descendants resolve against the right
// box. The push uses width as resolved and height=0 (or the explicit
// override) since the body's natural height is not yet known —
// percentage heights on grand-descendants resolve to 0 in that case,
// which is the documented v1 limitation.
func (cb *CSSBuilder) handlePositioned(item *HTMLItem, ss StylesStack, df *frontend.Document, anchorPages map[string]int) error {
	// HTMLToText runs before OutputPagesFromText calls InitPage, so
	// in the HTML pipeline the page dimensions are still zero-valued
	// at marker-emit time. PageSize() is idempotent and primes
	// cb.currentPageDimensions plus DefaultPageWidth/Height; without
	// this call the initial containing block resolves to an empty
	// rectangle and offsets land off-page.
	if _, err := cb.PageSize(); err != nil {
		return err
	}
	// Probe the styles without polluting the visible stack: clone the
	// current top, apply, throw away. We push the real styles below
	// via Output()'s own machinery.
	probe := ss.CurrentStyle().Clone()
	if err := StylesToStyles(probe, item.Styles, df, ss.CurrentStyle().Fontsize); err != nil {
		return err
	}
	rect := cb.resolvePositionedRect(probe)
	// Push CB so nested positioned descendants resolve against this
	// element's own box. Height is 0 here; descendants needing
	// percentage heights will get 0 (v1 limitation).
	cb.pushPositioningContext(positioningContext{
		x:      rect.x,
		y:      rect.y,
		width:  rect.width,
		height: rect.height,
	})
	bodyText, err := Output(cb, item, ss, df, anchorPages)
	cb.popPositioningContext()
	if err != nil {
		return err
	}
	body, err := cb.CreateVlist(bodyText, rect.width)
	if err != nil {
		return err
	}
	// Natural height = body height + depth. If an explicit height
	// was set (via `height:` or via top+bottom offsets), keep that;
	// the painter still places the top edge at rect.y so the
	// explicit height defines the box but the VList content may
	// overflow (v1: no clipping, matches Prince/AntennaHouse).
	naturalHeight := body.Height + body.Depth
	heightWasUnresolved := rect.height == 0
	height := rect.height
	if heightWasUnresolved {
		height = naturalHeight
	}
	// If the y was computed from bottom without an explicit height,
	// resolvePositionedRect could only contribute *bottom (the
	// rect.height term in its bottom branch was 0). Patch now that
	// the natural height is known. When height was explicit the y
	// value already reflects it — nothing to do.
	if probe.bottomOffset != nil && probe.topOffset == nil && heightWasUnresolved {
		parent := cb.currentContainingBlock()
		rect.y = parent.y - parent.height + *probe.bottomOffset + height
	}
	z := 0
	if probe.zIndex != nil {
		z = *probe.zIndex
	}
	cb.positionedItems = append(cb.positionedItems, &PositionedInsert{
		Body:        body,
		X:           rect.x,
		Y:           rect.y,
		Width:       rect.width,
		Height:      height,
		ZIndex:      z,
		SourceOrder: len(cb.positionedItems),
	})
	return nil
}

// paintPositionedItems sorts the page's pending positioned inserts by
// z-index (then source order as tiebreaker) and paints them on the
// current page. CSS 2.1 Appendix E: positioned descendants paint
// above in-flow non-positioned descendants and floats, so the caller
// (flushInserts) must invoke this after the buffered body and
// bottom-floats have been laid down but before the page is shipped.
func (cb *CSSBuilder) paintPositionedItems() {
	if len(cb.positionedItems) == 0 {
		return
	}
	sort.SliceStable(cb.positionedItems, func(i, j int) bool {
		a, b := cb.positionedItems[i], cb.positionedItems[j]
		if a.ZIndex != b.ZIndex {
			return a.ZIndex < b.ZIndex
		}
		return a.SourceOrder < b.SourceOrder
	})
	for _, pi := range cb.positionedItems {
		cb.frontend.Doc.CurrentPage.OutputAt(pi.X, pi.Y, pi.Body)
	}
	cb.positionedItems = nil
}

// positioningContext represents one entry in the CSS containing-block
// stack (CSS 2.1 §10.1). The X/Y are PDF coordinates of the top-left
// corner; Width/Height define the content box. A position: absolute
// element resolves top/right/bottom/left against the topmost entry.
//
// PDF y-coordinates grow upward; CSS top grows downward. The painter
// must convert when resolving offsets — see resolvePositionedRect.
type positioningContext struct {
	x          bag.ScaledPoint
	y          bag.ScaledPoint
	width      bag.ScaledPoint
	height     bag.ScaledPoint
	isPageRoot bool
}

// resetPositioningContextForPage replaces the positioning stack with a
// single entry for the current page area. Called from InitPage and
// NewPage so the initial containing block is always present and any
// stale per-element entries from the previous page are discarded.
func (cb *CSSBuilder) resetPositioningContextForPage() {
	pd := cb.currentPageDimensions
	cb.positioningContext = cb.positioningContext[:0]
	cb.positioningContext = append(cb.positioningContext, positioningContext{
		x:          pd.PageAreaLeft,
		y:          cb.frontend.Doc.DefaultPageHeight - pd.PageAreaTop,
		width:      pd.ContentWidth,
		height:     pd.ContentHeight,
		isPageRoot: true,
	})
}

// currentContainingBlock returns the topmost positioning context — the
// containing block a position: absolute element would resolve against.
// Falls back to a synthesized page-area entry if the stack is empty
// (defensive: callers should only invoke this after InitPage has run).
func (cb *CSSBuilder) currentContainingBlock() positioningContext {
	if len(cb.positioningContext) == 0 {
		pd := cb.currentPageDimensions
		return positioningContext{
			x:      pd.PageAreaLeft,
			y:      cb.frontend.Doc.DefaultPageHeight - pd.PageAreaTop,
			width:  pd.ContentWidth,
			height: pd.ContentHeight,
		}
	}
	return cb.positioningContext[len(cb.positioningContext)-1]
}

// pushPositioningContext extends the containing-block stack with a new
// entry. Called by Output() on entering an element whose computed
// position is anything but static. The pushed rectangle is the
// element's own content box (the element becomes the containing block
// for its absolutely-positioned descendants).
func (cb *CSSBuilder) pushPositioningContext(ctx positioningContext) {
	cb.positioningContext = append(cb.positioningContext, ctx)
}

// popPositioningContext drops the top entry. Must mirror every
// pushPositioningContext call. A no-op on an already-empty stack
// (defensive: the page-root entry is never popped explicitly — it is
// replaced wholesale by resetPositioningContextForPage on page
// transitions).
func (cb *CSSBuilder) popPositioningContext() {
	n := len(cb.positioningContext)
	if n == 0 {
		return
	}
	cb.positioningContext = cb.positioningContext[:n-1]
}
