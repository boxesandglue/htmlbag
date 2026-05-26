package htmlbag

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/PuerkitoBio/goquery"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

var onecm = bag.MustSP("1cm")

// HeadingEntry records a heading found during VList construction.
// The Page field is filled later during OutputPages when the heading
// is placed on a page. SE is filled at SE-construction time for tagged
// documents; consumers (PDF outline generator) use it to emit
// structure destinations as required by PDF/UA-2 §8.8.
type HeadingEntry struct {
	Level string // "h1", "h2", etc.
	Text  string
	Page  int                         // 1-based page number, 0 until assigned
	SE    *document.StructureElement // nil unless tagging is enabled and this heading was tagged
}

// AnchorEntry records an element with an id attribute (block or
// inline). Used as the target side of CSS target-counter() and
// target-text() cross-references. The Page field is filled during
// shipout, mirroring HeadingEntry; Text is filled at collection time
// from the element's contents (capped at 200 characters to keep the
// aux file bounded — long block anchors get a trailing "…").
type AnchorEntry struct {
	ID   string
	Text string
	Page int // 1-based page number, 0 until assigned
}

// anchorTextCap is the character budget for AnchorEntry.Text. Block
// anchors over this length are truncated with a U+2026 marker. Inline
// anchors are almost always shorter than this.
const anchorTextCap = 200

// truncateAnchorText shortens s to at most anchorTextCap characters,
// appending "…" when truncation happens.
func truncateAnchorText(s string) string {
	if len([]rune(s)) <= anchorTextCap {
		return s
	}
	runes := []rune(s)
	return string(runes[:anchorTextCap-1]) + "…"
}

// ElementEvent holds information about a processed block element.
type ElementEvent struct {
	TagName     string
	TextContent string
	VList       *node.VList
}

// ElementCallbackFunc is called after a block element's VList is built.
type ElementCallbackFunc func(event ElementEvent)

// PageInitCallbackFunc is called after a new page has been initialized.
type PageInitCallbackFunc func()

// CSSBuilder handles HTML chunks and CSS instructions.
type CSSBuilder struct {
	pagebox               []node.Node
	currentPageDimensions PageDimensions
	frontend              *frontend.Document
	css                   *csshtml.CSS
	stylesStack           StylesStack
	structureRoot         *document.StructureElement
	structureCurrent      *document.StructureElement
	enableTagging         bool
	ElementCallback       ElementCallbackFunc
	PageInitCallback      PageInitCallbackFunc
	// Counters holds named counter values used when evaluating CSS content
	// properties (e.g. "page" for the current page, "pages" for the total).
	// The "page" counter is set automatically during shipout; other counters
	// (like "pages") should be set by the caller.
	Counters     map[string]int
	headingCount int
	// Headings collects all h1–h6 headings encountered during VList
	// construction. Page numbers are assigned during OutputPages.
	Headings []HeadingEntry
	// Anchors collects every block-level element with an id attribute
	// encountered during VList construction. Page numbers are assigned
	// during shipout, just like Headings. Read by the multi-pass aux
	// loop to feed target-counter() resolution on the following pass.
	Anchors     []AnchorEntry
	anchorCount int
	// anchorPages maps anchor id → page number from the *previous*
	// render pass. Populated via SetAnchorPages before HTMLToText runs.
	// Nil on the first pass; the evaluator renders "?" for unresolved
	// references until the next pass fills the map in.
	anchorPages map[string]int
	// anchorTexts maps anchor id → captured text from the *previous*
	// render pass (CSS target-text()). Same lifecycle as anchorPages:
	// nil on first pass, populated via SetAnchorTexts before render.
	anchorTexts map[string]string
	// PendingVLists stores pre-rendered VLists keyed by a unique ID.
	// Used to pass already-rendered content (e.g. group contents) through
	// the HTML/CSS pipeline into table cells.
	PendingVLists map[string]*node.VList
	// pageInserts accumulates inserts (per class) whose marks have been
	// placed on the current page. Flushed by flushInserts, which is called
	// automatically from cb.NewPage() before shipout, and must also be
	// called once for the last page before its final shipout.
	pageInserts map[InsertClass][]*Insert
	// pageInsertHeight tracks the result of the per-class height summary
	// (e.g. totalFootnoteHeight for InsertFootnote), kept in sync to avoid
	// recomputing it on every overflow check.
	pageInsertHeight map[InsertClass]bag.ScaledPoint
	// tableInserts accumulates inserts encountered while building the
	// currently in-flight table. Saved/restored across nested buildTable
	// calls. Drained into the table VList's "inserts" attribute at the
	// end of buildTable.
	tableInserts []*Insert
	// tableInsertWidth is the width to format insert bodies inside a
	// table cell. Set by buildTable at entry, read by buildTD.
	tableInsertWidth bag.ScaledPoint
	// pageBuf collects body content for the current page that has been
	// committed by the page builder but not yet painted. flushInserts
	// drains it at shipout time, *after* the float reservation at the top
	// is known, so the body cursor can start below the (final) float
	// stack. Phase 3: two-pass page assembly enables multiple floats per
	// page without forcing page breaks.
	pageBuf []pageBufEntry
	// pageBufHeight is the running sum of pageBuf entry heights, kept in
	// sync to avoid recomputing it on every fit check.
	pageBufHeight bag.ScaledPoint
	// positioningContext is a stack of CSS containing blocks
	// (CSS 2.1 §10.1). The top entry is the nearest positioned
	// ancestor's content box (or, at the bottom of the stack, the
	// initial containing block = the current page area). A
	// position: absolute element resolves top/right/bottom/left
	// against the top entry. Pushed/popped by Output() on entering
	// and leaving every element whose computed position is anything
	// but static; primed by NewPage() with the page-area entry so
	// the initial containing block is always available.
	positioningContext []positioningContext
	// positionedItems collects PositionedInsert entries for the
	// current page. Filled by handlePositioned when an out-of-flow
	// `position: absolute` element is encountered; drained and
	// painted by paintPositionedItems from inside flushInserts
	// (after the buffered body, before bottom-floats — see CSS 2.1
	// App. E painting order). Kept as a parallel buffer (not in
	// pageInserts) because positioned items carry resolved pixel
	// coordinates and must not influence pageInsertHeight or the
	// flow's trial-fit calculations.
	positionedItems []*PositionedInsert
	// FootnoteSeparatorHeight overrides the default footnote rule thickness.
	// Zero falls back to the package default (0.4pt).
	FootnoteSeparatorHeight bag.ScaledPoint
	// FootnoteSeparatorSkip overrides the default skip between content area
	// and the rule. Zero falls back to the package default (6pt).
	FootnoteSeparatorSkip bag.ScaledPoint
	// FootnoteInterSkip overrides the default skip between consecutive
	// footnote bodies. Zero falls back to the package default (2pt).
	FootnoteInterSkip bag.ScaledPoint
	// FootnoteCallSizeRatio overrides the marker-call font-size relative to
	// the surrounding text. Zero falls back to 0.7.
	FootnoteCallSizeRatio float64
	// FootnoteCallRiseRatio overrides the marker-call rise (PDF Ts operator)
	// relative to the surrounding font size. Zero falls back to 0.4.
	FootnoteCallRiseRatio float64
	// FloatTopInterSkip overrides the default skip between consecutive
	// top-floats and below the stack (separating it from body content).
	// Zero falls back to the package default (6pt).
	FloatTopInterSkip bag.ScaledPoint
	// FloatBottomInterSkip overrides the default skip between consecutive
	// bottom-floats and above the stack (separating it from body content).
	// Zero falls back to the package default (6pt).
	FloatBottomInterSkip bag.ScaledPoint
}

// New creates an instance of the CSSBuilder.
func New(fd *frontend.Document, c *csshtml.CSS) (*CSSBuilder, error) {
	cb := CSSBuilder{
		css:                     c,
		frontend:                fd,
		stylesStack:             make(StylesStack, 0),
		pagebox:                 []node.Node{},
		Counters:                map[string]int{},
		PendingVLists:           map[string]*node.VList{},
		pageInserts:             map[InsertClass][]*Insert{},
		pageInsertHeight:        map[InsertClass]bag.ScaledPoint{},
		FootnoteSeparatorHeight: defaultFootnoteSeparatorHeight,
		FootnoteSeparatorSkip:   defaultFootnoteSeparatorSkip,
		FootnoteInterSkip:       defaultFootnoteInterSkip,
		FootnoteCallSizeRatio:   defaultFootnoteCallSizeRatio,
		FootnoteCallRiseRatio:   defaultFootnoteCallRiseRatio,
		FloatTopInterSkip:       defaultFloatTopInterSkip,
		FloatBottomInterSkip:    defaultFloatBottomInterSkip,
	}
	if err := LoadIncludedFonts(fd); err != nil {
		return nil, err
	}

	// Enable automatic structure tagging for PDF/UA (both UA-1 and UA-2)
	if fd.Doc.Format == document.FormatPDFUA || fd.Doc.Format == document.FormatPDFUA2 {
		cb.enableTagging = true
		cb.structureRoot = fd.Doc.RootStructureElement
		if cb.structureRoot == nil {
			cb.structureRoot = newSE("Document", fd.Doc.Format)
			fd.Doc.RootStructureElement = cb.structureRoot
		}
		cb.structureCurrent = cb.structureRoot
		// PDF/UA-2 (ISO 14289-2 §8.2.4): every structure role must
		// belong to or be role-mapped to one of the standard namespaces
		// (PDF 1.7 SSN / PDF 2.0 SSN / MathML). For the HTML5 namespace
		// we install a RoleMapNS that targets the PDF 2.0 SSN equivalent
		// for each canonical role we emit. Without this, veraPDF flags
		// every HTML5-tagged element as SENonStandard.
		if fd.Doc.Format == document.FormatPDFUA2 {
			fd.Doc.DeclareNamespace(document.NamespacePDF20SSN)
			fd.Doc.SetNamespaceRoleMap(document.NamespaceHTML5, html5RoleMap())
		}
	}

	return &cb, nil
}

// PageDimensions contains the page size and the margins of the page.
type PageDimensions struct {
	Width         bag.ScaledPoint
	Height        bag.ScaledPoint
	MarginLeft    bag.ScaledPoint
	MarginRight   bag.ScaledPoint
	MarginTop     bag.ScaledPoint
	MarginBottom  bag.ScaledPoint
	PageAreaLeft  bag.ScaledPoint
	PageAreaTop   bag.ScaledPoint
	ContentWidth  bag.ScaledPoint
	ContentHeight bag.ScaledPoint
	masterpage    *csshtml.Page
}

// PageAreas returns the CSS page margin box areas (e.g. "@top-center")
// for the current page type, or nil if no @page rule is active.
func (pd PageDimensions) PageAreas() map[string]map[string]string {
	if pd.masterpage == nil {
		return nil
	}
	return pd.masterpage.PageArea
}

// CSS returns the underlying CSS parser.
func (cb *CSSBuilder) CSS() *csshtml.CSS {
	return cb.css
}

// SetAnchorPages installs the id → page map collected on the previous
// render pass. The CSS evaluator reads this when resolving
// target-counter() references. Pass nil to clear.
func (cb *CSSBuilder) SetAnchorPages(m map[string]int) {
	cb.anchorPages = m
}

// SetAnchorTexts installs the id → text map collected on the previous
// render pass. The CSS evaluator reads this when resolving
// target-text() references. Pass nil to clear.
func (cb *CSSBuilder) SetAnchorTexts(m map[string]string) {
	cb.anchorTexts = m
}

func (cb *CSSBuilder) getPageType() *csshtml.Page {
	if first, ok := cb.css.Pages[":first"]; ok && len(cb.frontend.Doc.Pages) == 0 {
		return &first
	}
	isRight := len(cb.frontend.Doc.Pages)%2 == 0
	if right, ok := cb.css.Pages[":right"]; ok && isRight {
		return &right
	}
	if left, ok := cb.css.Pages[":left"]; ok && !isRight {
		return &left
	}
	if allPages, ok := cb.css.Pages[""]; ok {
		return &allPages
	}
	return nil
}

// InitPage makes sure that there is a valid page in the frontend.
func (cb *CSSBuilder) InitPage() error {
	if cb.frontend.Doc.CurrentPage != nil {
		return nil
	}
	if err := AddFontFamiliesFromCSS(cb.css, cb.frontend); err != nil {
		return err
	}
	var err error
	if defaultPage := cb.getPageType(); defaultPage != nil {
		wdStr, htStr := csshtml.PapersizeWidthHeight(defaultPage.Papersize)
		var wd, ht, mt, mb, ml, mr bag.ScaledPoint
		if wd, err = bag.SP(wdStr); err != nil {
			return err
		}
		if ht, err = bag.SP(htStr); err != nil {
			return err
		}
		if str := defaultPage.MarginTop; str == "" {
			mt = onecm
		} else {
			if mt, err = bag.SP(str); err != nil {
				return err
			}
		}
		if str := defaultPage.MarginBottom; str == "" {
			mb = onecm
		} else {
			if mb, err = bag.SP(str); err != nil {
				return err
			}
		}
		if str := defaultPage.MarginLeft; str == "" {
			ml = onecm
		} else {
			if ml, err = bag.SP(str); err != nil {
				return err
			}
		}
		if str := defaultPage.MarginRight; str == "" {
			mr = onecm
		} else {
			if mr, err = bag.SP(str); err != nil {
				return err
			}
		}
		var res map[string]string
		res, defaultPage.Attributes = csshtml.ResolveAttributes(defaultPage.Attributes)

		styles := cb.stylesStack.PushStyles()
		if err = StylesToStyles(styles, res, cb.frontend, cb.stylesStack.CurrentStyle().Fontsize); err != nil {
			return err
		}
		vl := node.NewVList()
		vl.Width = wd - ml - mr - styles.BorderLeftWidth - styles.BorderRightWidth - styles.PaddingLeft - styles.PaddingRight
		vl.Height = ht - mt - mb - styles.PaddingTop - styles.PaddingBottom - styles.BorderTopWidth - styles.BorderBottomWidth
		hv := HTMLValues{
			BorderLeftWidth:         styles.BorderLeftWidth,
			BorderRightWidth:        styles.BorderRightWidth,
			BorderTopWidth:          styles.BorderTopWidth,
			BorderBottomWidth:       styles.BorderBottomWidth,
			BorderTopStyle:          styles.BorderTopStyle,
			BorderLeftStyle:         styles.BorderLeftStyle,
			BorderRightStyle:        styles.BorderRightStyle,
			BorderBottomStyle:       styles.BorderBottomStyle,
			BorderTopColor:          styles.BorderTopColor,
			BorderLeftColor:         styles.BorderLeftColor,
			BorderRightColor:        styles.BorderRightColor,
			BorderBottomColor:       styles.BorderBottomColor,
			PaddingLeft:             styles.PaddingLeft,
			PaddingRight:            styles.PaddingRight,
			PaddingBottom:           styles.PaddingBottom,
			PaddingTop:              styles.PaddingTop,
			BorderTopLeftRadius:     styles.BorderTopLeftRadius,
			BorderTopRightRadius:    styles.BorderTopRightRadius,
			BorderBottomLeftRadius:  styles.BorderBottomLeftRadius,
			BorderBottomRightRadius: styles.BorderBottomRightRadius,
		}
		vl = cb.HTMLBorder(vl, hv)
		cb.stylesStack.PopStyles()

		// set page width / height
		cb.frontend.Doc.DefaultPageWidth = wd
		cb.frontend.Doc.DefaultPageHeight = ht
		cb.currentPageDimensions = PageDimensions{
			Width:         wd,
			Height:        ht,
			PageAreaLeft:  ml + styles.BorderLeftWidth + styles.PaddingLeft,
			PageAreaTop:   mt - styles.BorderTopWidth - styles.PaddingTop,
			ContentWidth:  wd - styles.BorderRightWidth - styles.PaddingRight - ml - mr - styles.BorderLeftWidth - styles.PaddingLeft,
			ContentHeight: ht - styles.BorderBottomWidth - styles.PaddingBottom - mt - mb - styles.BorderTopWidth - styles.PaddingTop,
			MarginTop:     mt,
			MarginBottom:  mb,
			MarginLeft:    ml,
			MarginRight:   mr,
			masterpage:    defaultPage,
		}
		cb.frontend.Doc.NewPage()
		if styles.BackgroundColor != nil {
			r := node.NewRule()
			x := pdfdraw.NewStandalone().ColorNonstroking(*styles.BackgroundColor).Rect(0, 0, wd, -ht).Fill()
			r.Pre = x.String()
			rvl := node.Vpack(r)
			rvl.Attributes = node.H{"origin": "page background color"}
			cb.frontend.Doc.CurrentPage.OutputAt(0, ht, rvl)
		}
		cb.frontend.Doc.CurrentPage.OutputAt(ml, ht-mt, vl)
		cb.firePageInit()
		return nil
	}
	// no page master found
	cb.frontend.Doc.DefaultPageWidth = bag.MustSP("210mm")
	cb.frontend.Doc.DefaultPageHeight = bag.MustSP("297mm")

	cb.currentPageDimensions = PageDimensions{
		Width:         cb.frontend.Doc.DefaultPageWidth,
		Height:        cb.frontend.Doc.DefaultPageHeight,
		ContentWidth:  cb.frontend.Doc.DefaultPageWidth - 2*onecm,
		ContentHeight: cb.frontend.Doc.DefaultPageHeight - 2*onecm,
		PageAreaLeft:  onecm,
		PageAreaTop:   onecm,
		MarginTop:     onecm,
		MarginBottom:  onecm,
		MarginLeft:    onecm,
		MarginRight:   onecm,
	}
	cb.frontend.Doc.NewPage()
	cb.firePageInit()
	return nil
}

// PageSize returns a struct with the dimensions of the current page.
func (cb *CSSBuilder) PageSize() (PageDimensions, error) {
	err := cb.InitPage()
	if err != nil {
		return PageDimensions{}, err
	}
	return cb.currentPageDimensions, nil
}

// ParseCSSString reads CSS instructions from a string.
func (cb *CSSBuilder) ParseCSSString(css string) error {
	var err error
	if err = cb.css.AddCSSText(css); err != nil {
		return err
	}
	return nil
}

// NewPage puts the current page into the PDF document and starts with a new page.
func (cb *CSSBuilder) NewPage() error {
	if err := cb.InitPage(); err != nil {
		return err
	}
	// Flush accumulated inserts onto this page before it ships out.
	if err := cb.flushInserts(); err != nil {
		return err
	}
	if err := cb.BeforeShipout(); err != nil {
		return err
	}
	cb.frontend.Doc.CurrentPage.Shipout()
	cb.frontend.Doc.NewPage()
	// Update page dimensions for the new page (different @page selector may apply).
	if pt := cb.getPageType(); pt != nil {
		cb.currentPageDimensions.masterpage = pt
		// Recalculate margins from the new page type.
		if str := pt.MarginTop; str != "" {
			if v, err := bag.SP(str); err == nil {
				cb.currentPageDimensions.MarginTop = v
			}
		}
		if str := pt.MarginBottom; str != "" {
			if v, err := bag.SP(str); err == nil {
				cb.currentPageDimensions.MarginBottom = v
			}
		}
		if str := pt.MarginLeft; str != "" {
			if v, err := bag.SP(str); err == nil {
				cb.currentPageDimensions.MarginLeft = v
			}
		}
		if str := pt.MarginRight; str != "" {
			if v, err := bag.SP(str); err == nil {
				cb.currentPageDimensions.MarginRight = v
			}
		}
		mt := cb.currentPageDimensions.MarginTop
		mb := cb.currentPageDimensions.MarginBottom
		ml := cb.currentPageDimensions.MarginLeft
		mr := cb.currentPageDimensions.MarginRight
		wd := cb.currentPageDimensions.Width
		ht := cb.currentPageDimensions.Height
		cb.currentPageDimensions.PageAreaLeft = ml
		cb.currentPageDimensions.PageAreaTop = mt
		cb.currentPageDimensions.ContentWidth = wd - ml - mr
		cb.currentPageDimensions.ContentHeight = ht - mt - mb
	}
	// Store page dimensions on the new page for callback access.
	if pd, err := cb.PageSize(); err == nil {
		storePageDimensions(cb, pd)
	}
	cb.firePageInit()
	return nil
}

func (cb *CSSBuilder) firePageInit() {
	cb.resetPositioningContextForPage()
	if cb.PageInitCallback != nil {
		cb.PageInitCallback()
	}
}

// PageDimensionsKey is the key used to store PageDimensions in Page.Userdata.
const PageDimensionsKey = "htmlbag.PageDimensions"

// storePageDimensions saves the current PageDimensions in the page's Userdata map.
func storePageDimensions(cb *CSSBuilder, pd PageDimensions) {
	page := cb.frontend.Doc.CurrentPage
	if page.Userdata == nil {
		page.Userdata = make(map[any]any)
	}
	page.Userdata[PageDimensionsKey] = pd
}

// OutputPages distributes the content of a VList across pages, breaking
// between child nodes whenever the next node would exceed the content height.
// It ships out each page automatically and starts new pages as needed.
// The final page is shipped out before returning.
func (cb *CSSBuilder) OutputPages(vl *node.VList) error {
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}

	// Unwrap nested single-child VLists (html > body > content). Each unwrap
	// step strips one VList; if it carried an inserts attribute, propagate
	// it onto the next inner node so the page builder can still see it.
	contentList := vl.List
	contentWidth := vl.Width
	if vl.Attributes != nil {
		propagateInsertsAttr(vl, contentList)
	}
	for {
		inner, ok := contentList.(*node.VList)
		if !ok || inner.Next() != nil {
			break
		}
		propagateInsertsAttr(inner, inner.List)
		contentList = inner.List
		if inner.Width > 0 {
			contentWidth = inner.Width
		}
	}

	// Store page dimensions as Userdata on the current page so callbacks
	// can access margins.
	storePageDimensions(cb, pd)

	cur := contentList

	// refreshPage re-reads page dimensions after a NewPage advanced the
	// document. Phase 3 doesn't track a y-cursor here — the body cursor's
	// position is computed at flushInserts time from the final float
	// reservation.
	refreshPage := func() error {
		var err error
		if pd, err = cb.PageSize(); err != nil {
			return err
		}
		return nil
	}

	// trialPageHeight estimates what the current page's total content
	// footprint would be if `incoming` inserts were committed and a body
	// box of height addBodyH were buffered: top-float stack + body buffer
	// + addBodyH + footnote stack. The page builder uses this to decide
	// whether the next node still fits.
	trialPageHeight := func(incoming []*Insert, addBodyH bag.ScaledPoint) bag.ScaledPoint {
		topFloatTrial := append([]*Insert{}, cb.pageInserts[InsertFloatTop]...)
		topFloatTrial = append(topFloatTrial, filterInserts(incoming, InsertFloatTop)...)
		bottomFloatTrial := append([]*Insert{}, cb.pageInserts[InsertFloatBottom]...)
		bottomFloatTrial = append(bottomFloatTrial, filterInserts(incoming, InsertFloatBottom)...)
		footnoteTrial := append([]*Insert{}, cb.pageInserts[InsertFootnote]...)
		footnoteTrial = append(footnoteTrial, filterInserts(incoming, InsertFootnote)...)
		return cb.totalFloatTopHeight(topFloatTrial) +
			cb.pageBufHeight + addBodyH +
			cb.totalFloatBottomHeight(bottomFloatTrial) +
			cb.totalFootnoteHeight(footnoteTrial)
	}

	for cur != nil {
		next := cur.Next()
		h := vlistNodeHeight(cur)
		incoming := insertsOnNode(cur)
		contentArea := pd.Height - pd.MarginTop - pd.MarginBottom

		// page-break-before: always — only fires if the page has any
		// buffered body (else it would create a leading blank page).
		if forceBreakBefore(cur) && cb.pageBufHeight > 0 {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}

		// page-break-after: avoid — if the next ~2 nodes wouldn't fit on
		// the current page either, break before cur instead.
		if avoidBreakAfter(cur) && next != nil {
			peekH := h + vlistNodeHeight(next)
			nn := next.Next()
			if nn != nil {
				peekH += vlistNodeHeight(nn)
			}
			fits := trialPageHeight(incoming, peekH) <= contentArea
			// Relaxation for splittable nn (e.g. <pre>): the orphan-heading
			// rule only requires the heading + at least one line of the
			// following block on the same page. The rest can split off via
			// outputBlockSplit. Without this, a long <pre> after a heading
			// pushes the heading to the next page even though splitting
			// would let it stay in place.
			if !fits && nn != nil {
				if reduced, ok := splittablePeekHeight(nn); ok {
					relaxedH := h + vlistNodeHeight(next) + reduced
					if trialPageHeight(incoming, relaxedH) <= contentArea {
						fits = true
					}
				}
			}
			if !fits && cb.pageBufHeight > 0 {
				if err := cb.NewPage(); err != nil {
					return err
				}
				if err := refreshPage(); err != nil {
					return err
				}
			}
		}

		// Overflow: cur (with its inserts) doesn't fit on the current
		// page. Ship what's buffered and start fresh. Skip the break if
		// the buffer is empty — cur is forcibly placed on the empty page
		// (single-node-too-tall case, accept truncation).
		//
		// page-break-inside: avoid relaxes the "buffer empty" guard so
		// an avoid-block lands on a fresh page even in edge cases where
		// the body buffer is empty but inserts (footnotes, floats) have
		// already eaten into the page. The fresh-page-fits gate
		// (h <= contentArea) prevents infinite loops for blocks taller
		// than a full page.
		avoidForcesBreak := avoidBreakInside(cur) &&
			trialPageHeight(incoming, h) > contentArea &&
			cb.pageBufHeight == 0 &&
			h <= contentArea
		if (trialPageHeight(incoming, h) > contentArea && cb.pageBufHeight > 0) || avoidForcesBreak {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}

		// Commit cur's inserts (both classes) to the current page's
		// accumulators. Heights are kept in sync for trialPageHeight.
		if len(incoming) > 0 {
			for _, ins := range incoming {
				cb.pageInserts[ins.Class] = append(cb.pageInserts[ins.Class], ins)
			}
			cb.pageInsertHeight[InsertFloatTop] = cb.totalFloatTopHeight(cb.pageInserts[InsertFloatTop])
			cb.pageInsertHeight[InsertFloatBottom] = cb.totalFloatBottomHeight(cb.pageInserts[InsertFloatBottom])
			cb.pageInsertHeight[InsertFootnote] = cb.totalFootnoteHeight(cb.pageInserts[InsertFootnote])
		}

		// Detach and wrap.
		cur.SetPrev(nil)
		cur.SetNext(nil)
		box := node.NewVList()
		box.List = cur
		box.Width = contentWidth
		box.Height = h

		// Heading and anchor indices for page-number tracking happen at
		// flush time, not here, so the page number reflects the page
		// actually painted.
		headingIdx := -1
		var anchorIndices []int
		if vl, ok := cur.(*node.VList); ok && vl.Attributes != nil {
			if idx, ok := vl.Attributes["_heading_idx"].(int); ok {
				headingIdx = idx
			}
			if idx, ok := vl.Attributes["_anchor_idx"].(int); ok {
				anchorIndices = append(anchorIndices, idx)
			}
			if list, ok := vl.Attributes["_anchor_indices"].([]int); ok {
				anchorIndices = append(anchorIndices, list...)
			}
		}

		cb.bufferBody(box, h, headingIdx, anchorIndices)

		// page-break-after: always — ship the page now if more content
		// follows.
		if forceBreakAfter(cur) && next != nil {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}

		cur = next
	}

	// Flush any inserts accumulated on the final page before its shipout.
	if err := cb.flushInserts(); err != nil {
		return err
	}
	if err := cb.BeforeShipout(); err != nil {
		return err
	}
	cb.frontend.Doc.CurrentPage.Shipout()
	return nil
}

// OutputPagesFromText takes a Text tree (from HTMLToText), splits it at forced
// page breaks, and formats each group with the content width of its target page.
// This ensures that different @page margins produce different text widths.
func (cb *CSSBuilder) OutputPagesFromText(te *frontend.Text) error {
	// Find the body-level Text element (unwrap html > body wrappers).
	body := findBody(te)

	// Split body items into groups at pageBreakBefore boundaries.
	groups := splitTextAtPageBreaks(body)

	for i, group := range groups {
		if i > 0 {
			if err := cb.NewPage(); err != nil {
				return err
			}
		}

		pd, err := cb.PageSize()
		if err != nil {
			return err
		}

		// Create a wrapper Text with the body's settings for this group.
		wrapper := &frontend.Text{
			Settings: body.Settings,
			Items:    group,
		}

		vl, err := cb.CreateVlist(wrapper, pd.ContentWidth)
		if err != nil {
			return err
		}

		// Place nodes from this group's vlist onto pages.
		// Within a group there are no forced page breaks, but content may
		// overflow and require automatic page breaks.
		if err := cb.outputGroupNodes(vl, pd); err != nil {
			return err
		}
	}

	// Flush any inserts accumulated on the final page before its shipout.
	if err := cb.flushInserts(); err != nil {
		return err
	}
	if err := cb.BeforeShipout(); err != nil {
		return err
	}
	cb.frontend.Doc.CurrentPage.Shipout()
	return nil
}

// findBody descends through the root → <html> → <body> wrapper chain to reach
// the Text whose Items are the page-level content. It only descends through
// untagged or <html>-tagged wrappers; once a deeper tag is encountered it
// stops, so structural elements like <table> still reach their dedicated
// builders. Previously this function descended through every single-child
// Text, which silently unwrapped <body>/<table>/<tbody> when each layer had
// only one child and broke the table layout.
func findBody(te *frontend.Text) *frontend.Text {
	for {
		dbg, _ := te.Settings[frontend.SettingDebug].(string)
		if dbg != "" && dbg != "html" {
			return te
		}
		if len(te.Items) != 1 {
			return te
		}
		child, ok := te.Items[0].(*frontend.Text)
		if !ok {
			return te
		}
		te = child
	}
}

// splitTextAtPageBreaks splits the Items of a body-level Text into groups.
// A new group starts whenever a child Text carries a CSS forced break-before
// keyword (`always`, `page`, `left`, `right`, `recto`, `verso`, `all`).
func splitTextAtPageBreaks(body *frontend.Text) [][]any {
	var groups [][]any
	var current []any

	for _, itm := range body.Items {
		if t, ok := itm.(*frontend.Text); ok {
			if pbb, ok := t.Settings[frontend.SettingPageBreakBefore]; ok && isForcedBreakValue(pbb) {
				if len(current) > 0 {
					groups = append(groups, current)
				}
				current = []any{itm}
				continue
			}
		}
		current = append(current, itm)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// outputGroupNodes places the nodes from a single vlist onto the current page,
// breaking to new pages if the content overflows (no forced breaks expected).
func (cb *CSSBuilder) outputGroupNodes(vl *node.VList, pd PageDimensions) error {
	// Unwrap nested single-child VLists. Each unwrap step strips one VList;
	// if it carried an inserts attribute, propagate it onto the next inner
	// node so the page builder can still see it.
	contentList := vl.List
	contentWidth := vl.Width
	if vl.Attributes != nil {
		propagateInsertsAttr(vl, contentList)
	}
	for {
		inner, ok := contentList.(*node.VList)
		if !ok || inner.Next() != nil {
			break
		}
		propagateInsertsAttr(inner, inner.List)
		contentList = inner.List
		if inner.Width > 0 {
			contentWidth = inner.Width
		}
	}

	storePageDimensions(cb, pd)

	cur := contentList

	refreshPage := func() error {
		var err error
		if pd, err = cb.PageSize(); err != nil {
			return err
		}
		return nil
	}

	trialPageHeight := func(incoming []*Insert, addBodyH bag.ScaledPoint) bag.ScaledPoint {
		topFloatTrial := append([]*Insert{}, cb.pageInserts[InsertFloatTop]...)
		topFloatTrial = append(topFloatTrial, filterInserts(incoming, InsertFloatTop)...)
		bottomFloatTrial := append([]*Insert{}, cb.pageInserts[InsertFloatBottom]...)
		bottomFloatTrial = append(bottomFloatTrial, filterInserts(incoming, InsertFloatBottom)...)
		footnoteTrial := append([]*Insert{}, cb.pageInserts[InsertFootnote]...)
		footnoteTrial = append(footnoteTrial, filterInserts(incoming, InsertFootnote)...)
		return cb.totalFloatTopHeight(topFloatTrial) +
			cb.pageBufHeight + addBodyH +
			cb.totalFloatBottomHeight(bottomFloatTrial) +
			cb.totalFootnoteHeight(footnoteTrial)
	}

	for cur != nil {
		next := cur.Next()
		h := vlistNodeHeight(cur)
		contentArea := pd.Height - pd.MarginTop - pd.MarginBottom

		// Special path: tables with repeating headers. outputTableRows
		// uses direct OutputAt and is boundary-isolated — it forces a
		// page break before AND after, so any surrounding body content
		// lands on its own page. Take it only when the table actually
		// spans more than one page. Short tables (the typical Markdown
		// case — GFM tables always carry a thead, so _buildHeaders is
		// set unconditionally) go through the normal pageBuf path
		// below, which composes them with adjacent paragraphs without
		// spurious page breaks.
		if tableVL, ok := cur.(*node.VList); ok && tableVL.Attributes != nil {
			if buildHeadersFn, tok := tableVL.Attributes["_buildHeaders"]; tok {
				tableIncoming := insertsOnNode(cur)
				tableInsertsH := cb.totalFloatTopHeight(filterInserts(tableIncoming, InsertFloatTop)) +
					cb.totalFloatBottomHeight(filterInserts(tableIncoming, InsertFloatBottom)) +
					cb.totalFootnoteHeight(filterInserts(tableIncoming, InsertFootnote))
				if h+tableInsertsH > contentArea {
					// Anything already buffered on this page (e.g. a heading
					// that introduces the table) should be painted FIRST so
					// the table can start directly below it, rather than
					// being shipped off to its own short page. flushInserts
					// paints the body and top-floats but does NOT advance
					// to a new page, so outputTableRows can take over the
					// current page with the correct y cursor.
					flushedBodyH := cb.pageBufHeight
					topFloatH := cb.pageInsertHeight[InsertFloatTop]
					if err := cb.flushInserts(); err != nil {
						return err
					}
					yLocal := pd.Height - pd.MarginTop - topFloatH - flushedBodyH
					yLimitLocal := pd.MarginBottom
					phc := flushedBodyH > 0 || topFloatH > 0
					if err := cb.outputTableRows(tableVL, buildHeadersFn, &yLocal, &yLimitLocal, &phc, &pd); err != nil {
						return err
					}
					// Commit the table's own inserts (typically footnotes
					// from cells) to the page that holds the table's last
					// rows; they paint at the next flushInserts.
					if len(tableIncoming) > 0 {
						for _, ins := range tableIncoming {
							cb.pageInserts[ins.Class] = append(cb.pageInserts[ins.Class], ins)
						}
						cb.pageInsertHeight[InsertFloatTop] = cb.totalFloatTopHeight(cb.pageInserts[InsertFloatTop])
						cb.pageInsertHeight[InsertFootnote] = cb.totalFootnoteHeight(cb.pageInserts[InsertFootnote])
					}
					// Force a fresh page for whatever follows so the body
					// buffer doesn't paint over the directly-placed rows.
					// Skip if no more content — the caller's final flush
					// will ship the table's last page cleanly.
					if next != nil {
						if err := cb.NewPage(); err != nil {
							return err
						}
						if err := refreshPage(); err != nil {
							return err
						}
					}
					cur = next
					continue
				}
			}
		}

		incoming := insertsOnNode(cur)

		// Splittable block (<pre>, block container with bg/border) that's
		// taller than what fits even on an empty page: fragment it across
		// pages instead of letting the wrapped vlist run off the bottom.
		// Short splittable blocks fall through to the normal pageBuf path.
		if vlS, ok := cur.(*node.VList); ok && vlS.Attributes != nil {
			if isSplittable, _ := vlS.Attributes["_splittable"].(bool); isSplittable {
				if trialPageHeight(incoming, h) > contentArea {
					// Commit incoming inserts so outputBlockSplit's
					// availOnPage sees the correct float/footnote
					// reservations. Don't ship pageBuf here — the splitter
					// appends its first fragment after whatever's already
					// buffered (e.g. a heading just placed via the
					// avoidBreakAfter relaxation), and only calls NewPage
					// between fragments.
					if len(incoming) > 0 {
						for _, ins := range incoming {
							cb.pageInserts[ins.Class] = append(cb.pageInserts[ins.Class], ins)
						}
						cb.pageInsertHeight[InsertFloatTop] = cb.totalFloatTopHeight(cb.pageInserts[InsertFloatTop])
						cb.pageInsertHeight[InsertFloatBottom] = cb.totalFloatBottomHeight(cb.pageInserts[InsertFloatBottom])
						cb.pageInsertHeight[InsertFootnote] = cb.totalFootnoteHeight(cb.pageInserts[InsertFootnote])
					}
					if err := cb.outputBlockSplit(vlS, &pd, refreshPage); err != nil {
						return err
					}
					if forceBreakAfter(cur) && next != nil {
						if err := cb.NewPage(); err != nil {
							return err
						}
						if err := refreshPage(); err != nil {
							return err
						}
					}
					cur = next
					continue
				}
			}
		}

		if avoidBreakAfter(cur) && next != nil {
			peekH := h + vlistNodeHeight(next)
			nn := next.Next()
			if nn != nil {
				peekH += vlistNodeHeight(nn)
			}
			fits := trialPageHeight(incoming, peekH) <= contentArea
			if !fits && nn != nil {
				if reduced, ok := splittablePeekHeight(nn); ok {
					relaxedH := h + vlistNodeHeight(next) + reduced
					if trialPageHeight(incoming, relaxedH) <= contentArea {
						fits = true
					}
				}
			}
			if !fits && cb.pageBufHeight > 0 {
				if err := cb.NewPage(); err != nil {
					return err
				}
				if err := refreshPage(); err != nil {
					return err
				}
			}
		}

		if trialPageHeight(incoming, h) > contentArea && cb.pageBufHeight > 0 {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}

		if len(incoming) > 0 {
			for _, ins := range incoming {
				cb.pageInserts[ins.Class] = append(cb.pageInserts[ins.Class], ins)
			}
			cb.pageInsertHeight[InsertFloatTop] = cb.totalFloatTopHeight(cb.pageInserts[InsertFloatTop])
			cb.pageInsertHeight[InsertFloatBottom] = cb.totalFloatBottomHeight(cb.pageInserts[InsertFloatBottom])
			cb.pageInsertHeight[InsertFootnote] = cb.totalFootnoteHeight(cb.pageInserts[InsertFootnote])
		}

		cur.SetPrev(nil)
		cur.SetNext(nil)
		box := node.NewVList()
		box.List = cur
		box.Width = contentWidth
		box.Height = h

		headingIdx := -1
		var anchorIndices []int
		if vl, ok := cur.(*node.VList); ok && vl.Attributes != nil {
			if idx, ok := vl.Attributes["_heading_idx"].(int); ok {
				headingIdx = idx
			}
			if idx, ok := vl.Attributes["_anchor_idx"].(int); ok {
				anchorIndices = append(anchorIndices, idx)
			}
			if list, ok := vl.Attributes["_anchor_indices"].([]int); ok {
				anchorIndices = append(anchorIndices, list...)
			}
		}

		cb.bufferBody(box, h, headingIdx, anchorIndices)

		if forceBreakAfter(cur) && next != nil {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}

		cur = next
	}

	return nil
}

// outputBlockSplit fragments a splittable block (e.g. <pre>) across pages
// when its wrapped height exceeds the available content area. The block was
// marked in vlistbuilder.go with `_splittableInner` (slice of inner children),
// `_splittableHv` (HTMLValues), and `_splittableInnerWidth`.
//
// Per-fragment wrapping rules (CSS-style fragmentation):
//   - top fragment    keeps padding-top + border-top, drops bottom side
//   - middle fragment drops both top and bottom sides
//   - bottom fragment drops top side, keeps padding-bottom + border-bottom
//
// Padding-left/right and side-borders are emitted on every fragment.
//
// Each fragment is buffered via bufferBody so it composes correctly with
// surrounding paragraphs in the page buffer; NewPage is called between
// fragments to ship the partial page.
func (cb *CSSBuilder) outputBlockSplit(blockVL *node.VList, pd *PageDimensions, refreshPage func() error) error {
	children, _ := blockVL.Attributes["_splittableInner"].([]node.Node)
	hv, _ := blockVL.Attributes["_splittableHv"].(HTMLValues)
	innerWidth, _ := blockVL.Attributes["_splittableInnerWidth"].(bag.ScaledPoint)

	if len(children) == 0 {
		return nil
	}

	// Nackter Absatz (kein Border/Background): HTMLBorder-Wrapper überspringen.
	// vlistbuilder.go markiert solche Blöcke mit hv == HTMLValues{}.
	noWrapper := !hv.hasBorder() && hv.BackgroundColor == nil

	// Bookmarks/Anchors auf dem Original-VList müssen aufs erste Fragment
	// wandern, sonst landet die Seitenreferenz auf der falschen Seite.
	var firstHeadingIdx int = -1
	var firstAnchorIndices []int
	if blockVL.Attributes != nil {
		if idx, ok := blockVL.Attributes["_heading_idx"].(int); ok {
			firstHeadingIdx = idx
		}
		if idx, ok := blockVL.Attributes["_anchor_idx"].(int); ok {
			firstAnchorIndices = append(firstAnchorIndices, idx)
		}
		if list, ok := blockVL.Attributes["_anchor_indices"].([]int); ok {
			firstAnchorIndices = append(firstAnchorIndices, list...)
		}
	}

	// Detach so children can be re-linked into per-fragment vlists.
	for _, c := range children {
		c.SetPrev(nil)
		c.SetNext(nil)
	}

	const (
		fragTop = iota
		fragMiddle
		fragBottom
		fragOnly
	)

	// buildFragment wraps a slice of inner children with HTMLBorder using a
	// per-fragment HTMLValues that drops paddings/borders on the cut sides.
	// Bei noWrapper bleibt der innere VList unverpackt — kein extra Box-Frame.
	buildFragment := func(items []node.Node, kind int) (*node.VList, bag.ScaledPoint) {
		innerVL := node.NewVList()
		innerVL.Width = innerWidth
		var totalH bag.ScaledPoint
		for i, n := range items {
			if i == 0 {
				innerVL.List = n
			} else {
				innerVL.List = node.InsertAfter(innerVL.List, node.Tail(innerVL.List), n)
			}
			totalH += vlistNodeHeight(n)
		}
		innerVL.Height = totalH
		// Carry the PDF/UA StructureElement linkage onto the first
		// fragment. tagVList stamps it on blockVL before splitting;
		// without this copy the wrapped fragment would have no /K MCR
		// entry and the StructElem would be structurally empty.
		// Only the first fragment (Only / Top) gets the tag — later
		// fragments would create duplicate MCID references.
		if kind == fragOnly || kind == fragTop {
			if blockVL.Attributes != nil {
				if tag, ok := blockVL.Attributes["tag"]; ok {
					if innerVL.Attributes == nil {
						innerVL.Attributes = node.H{}
					}
					innerVL.Attributes["tag"] = tag
				}
			}
		}
		if noWrapper {
			return innerVL, vlistNodeHeight(innerVL)
		}
		fragHv := hv
		if kind != fragTop && kind != fragOnly {
			fragHv.PaddingTop = 0
			fragHv.BorderTopWidth = 0
		}
		if kind != fragBottom && kind != fragOnly {
			fragHv.PaddingBottom = 0
			fragHv.BorderBottomWidth = 0
		}
		wrapped := cb.HTMLBorder(innerVL, fragHv)
		// Same rationale for the bordered path: HTMLBorder produced a
		// fresh outer VList, so the tag would otherwise be dropped.
		if kind == fragOnly || kind == fragTop {
			if blockVL.Attributes != nil {
				if tag, ok := blockVL.Attributes["tag"]; ok {
					if wrapped.Attributes == nil {
						wrapped.Attributes = node.H{}
					}
					wrapped.Attributes["tag"] = tag
				}
			}
		}
		return wrapped, vlistNodeHeight(wrapped)
	}

	availOnPage := func() bag.ScaledPoint {
		contentArea := pd.Height - pd.MarginTop - pd.MarginBottom
		used := cb.pageBufHeight +
			cb.pageInsertHeight[InsertFloatTop] +
			cb.pageInsertHeight[InsertFloatBottom] +
			cb.pageInsertHeight[InsertFootnote]
		return contentArea - used
	}

	totalH := func(items []node.Node) bag.ScaledPoint {
		var s bag.ScaledPoint
		for _, n := range items {
			s += vlistNodeHeight(n)
		}
		return s
	}

	i := 0
	isFirst := true
	for i < len(children) {
		avail := availOnPage()

		// Try to fit all remaining children with bottom-fragment overhead.
		topOverhead := bag.ScaledPoint(0)
		if isFirst {
			topOverhead = hv.PaddingTop + hv.BorderTopWidth
		}
		bottomOverhead := hv.PaddingBottom + hv.BorderBottomWidth
		remaining := totalH(children[i:])

		if topOverhead+remaining+bottomOverhead <= avail {
			kind := fragBottom
			if isFirst {
				kind = fragOnly
			}
			wrapped, h := buildFragment(children[i:], kind)
			hIdx, aIdx := firstHeadingIdx, firstAnchorIndices
			if !isFirst {
				hIdx, aIdx = -1, nil
			}
			cb.bufferBody(wrapped, h, hIdx, aIdx)
			return nil
		}

		// Doesn't all fit: collect a top/middle fragment that does fit.
		var batch []node.Node
		batchH := bag.ScaledPoint(0)
		for ; i < len(children); i++ {
			ch := vlistNodeHeight(children[i])
			if topOverhead+batchH+ch > avail && len(batch) > 0 {
				break
			}
			batch = append(batch, children[i])
			batchH += ch
		}
		if len(batch) == 0 {
			// One child is taller than a full empty page. Place it anyway —
			// truncation is unavoidable. Advance so the loop terminates.
			batch = append(batch, children[i])
			i++
		}

		// Widow / orphan protection: count actual lines (HList nodes) —
		// children alternate HList,Glue,HList,Glue so raw count doubles.
		// CSS Fragmentation 3 §4 spec defaults are widows: 2 and orphans: 2.
		const minLines = 2
		countHL := func(items []node.Node) int {
			n := 0
			for _, c := range items {
				if _, ok := c.(*node.HList); ok {
					n++
				}
			}
			return n
		}

		// Orphan protection: if the first fragment of the block would leave
		// fewer than minLines on the current page, force a NewPage first so
		// the block restarts on a fresh page with full available space. Only
		// applies when there's something already on the page — on an empty
		// page even a single line has to land here.
		if isFirst && cb.pageBufHeight > 0 && countHL(batch) < minLines && i < len(children) {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
			i = 0
			continue
		}

		// Widow protection: the next page must carry at least minLines HLists;
		// otherwise pull items back from this batch until it does, while
		// leaving at least minLines in the current batch (don't trade a
		// widow for an orphan).
		if i < len(children) {
			remainingLines := countHL(children[i:])
			for remainingLines < minLines && countHL(batch) > minLines {
				last := batch[len(batch)-1]
				batchH -= vlistNodeHeight(last)
				batch = batch[:len(batch)-1]
				i--
				if _, ok := last.(*node.HList); ok {
					remainingLines++
				}
			}
		}

		kind := fragTop
		if !isFirst {
			kind = fragMiddle
		}
		wrapped, h := buildFragment(batch, kind)
		hIdx, aIdx := firstHeadingIdx, firstAnchorIndices
		if !isFirst {
			hIdx, aIdx = -1, nil
		}
		cb.bufferBody(wrapped, h, hIdx, aIdx)
		isFirst = false

		// More fragments to come: ship this page and start fresh.
		if i < len(children) {
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}
		}
	}
	return nil
}

// outputTableRows unpacks a table VList into individual rows and places them
// on pages, repeating header rows after each page break.
func (cb *CSSBuilder) outputTableRows(tableVL *node.VList, buildHeadersFn any, y *bag.ScaledPoint, yLimit *bag.ScaledPoint, pageHasContent *bool, pd *PageDimensions) error {
	buildHeaders := buildHeadersFn.(func() ([]*node.HList, error))
	headerCount := tableVL.Attributes["_headerCount"].(int)
	tableWidth := tableVL.Width

	// Footer support: tables with <tfoot> repeat the footer at the
	// bottom of every page they span (HTML semantics, CSS Tables 3 §11.1).
	var footerCount int
	var footerHeight bag.ScaledPoint
	var buildFooters func() ([]*node.HList, error)
	if fc, ok := tableVL.Attributes["_footerCount"].(int); ok {
		footerCount = fc
	}
	if fh, ok := tableVL.Attributes["_footerHeight"].(bag.ScaledPoint); ok {
		footerHeight = fh
	}
	if bf, ok := tableVL.Attributes["_buildFooters"].(func() ([]*node.HList, error)); ok {
		buildFooters = bf
	}

	// Collect all row nodes from the table VList. The trailing
	// footerCount rows are pulled out of the normal stream and placed
	// explicitly at the end of each page.
	var rows []node.Node
	for n := tableVL.List; n != nil; n = n.Next() {
		rows = append(rows, n)
	}
	dataEnd := len(rows) - footerCount

	placeFooters := func() error {
		if footerCount == 0 {
			return nil
		}
		footers, err := buildFooters()
		if err != nil {
			return err
		}
		for _, ft := range footers {
			h := ft.Height + ft.Depth
			ft.SetPrev(nil)
			ft.SetNext(nil)
			box := node.NewVList()
			box.List = ft
			box.Width = tableWidth
			box.Height = h
			cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, *y, box)
			*y -= h
		}
		*pageHasContent = true
		return nil
	}

	refreshPage := func() error {
		var err error
		*pd, err = cb.PageSize()
		if err != nil {
			return err
		}
		*y = pd.Height - pd.MarginTop
		*yLimit = pd.MarginBottom
		*pageHasContent = false
		return nil
	}

	for i := 0; i < dataEnd; i++ {
		row := rows[i]
		h := vlistNodeHeight(row)

		// Check if row fits on current page. The footer reserves space
		// at the bottom on every page, so the effective limit is
		// yLimit + footerHeight.
		// page-break-inside: avoid tightens the fit check so an avoid-row
		// is pushed onto a fresh page rather than placed partially off
		// the page box. The existing pageHasContent guard is dropped for
		// avoid rows, but only when a fresh page would actually fit the
		// row — otherwise the loop is pointless and risks infinite breaks
		// for rows taller than a full page.
		pageContent := pd.Height - pd.MarginTop - pd.MarginBottom
		effectiveLimit := *yLimit + footerHeight
		avoidForcesBreak := avoidBreakInside(row) && *y-h < effectiveLimit && !*pageHasContent && h+footerHeight <= pageContent
		if (*y-h < effectiveLimit && *pageHasContent) || avoidForcesBreak {
			// Place footer at the bottom of the current page before
			// breaking so it appears on every spanned page.
			if err := placeFooters(); err != nil {
				return err
			}
			if err := cb.NewPage(); err != nil {
				return err
			}
			if err := refreshPage(); err != nil {
				return err
			}

			// Repeat header rows on the new page (skip if this IS a header row).
			if i >= headerCount {
				headers, err := buildHeaders()
				if err != nil {
					return err
				}
				for _, hdr := range headers {
					hdrH := hdr.Height + hdr.Depth
					hdr.SetPrev(nil)
					hdr.SetNext(nil)
					box := node.NewVList()
					box.List = hdr
					box.Width = tableWidth
					box.Height = hdrH
					cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, *y, box)
					*y -= hdrH
				}
				*pageHasContent = true
			}
		}

		// Detach row from linked list and place it.
		row.SetPrev(nil)
		row.SetNext(nil)

		box := node.NewVList()
		box.List = row
		box.Width = tableWidth
		box.Height = h

		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, *y, box)
		*y -= h
		*pageHasContent = true
	}

	// Footer on the last page.
	return placeFooters()
}

// avoidBreakAfter checks if a node has the page-break-after: avoid attribute.
func avoidBreakAfter(n node.Node) bool {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakAfter"]; ok {
			return v == "avoid"
		}
	}
	return false
}

// avoidBreakInside reports whether a node carries the CSS
// page-break-inside: avoid (or break-inside: avoid) directive. Both VList
// and HList nodes are supported because table rows materialize as HLists
// via frontend.BuildTable, while generic block nodes materialize as
// VLists in vlistbuilder.go.
func avoidBreakInside(n node.Node) bool {
	switch t := n.(type) {
	case *node.VList:
		if t.Attributes != nil {
			if v, ok := t.Attributes["pageBreakInside"]; ok {
				return v == "avoid"
			}
		}
	case *node.HList:
		if t.Attributes != nil {
			if v, ok := t.Attributes["pageBreakInside"]; ok {
				return v == "avoid"
			}
		}
	}
	return false
}

// isForcedBreakValue reports whether a CSS break-after / break-before
// keyword forces a page break. CSS Fragmentation 3 §3.1 lists `always`,
// `all`, `page`, `left`, `right`, `recto`, and `verso` as forced-break
// values (the legacy `page-break-*: always` and the modern `break-*: page`
// are synonymous). Page-area variants (`left`/`right`/`recto`/`verso`)
// degrade to a plain page break here since the page-area / named-page
// machinery is not yet wired up.
func isForcedBreakValue(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	switch s {
	case "always", "all", "page", "left", "right", "recto", "verso":
		return true
	}
	return false
}

// forceBreakAfter checks if a node has a forced break-after keyword.
func forceBreakAfter(n node.Node) bool {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakAfter"]; ok {
			return isForcedBreakValue(v)
		}
	}
	return false
}

// forceBreakBefore checks if a node has a forced break-before keyword.
func forceBreakBefore(n node.Node) bool {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakBefore"]; ok {
			return isForcedBreakValue(v)
		}
	}
	return false
}

// splittablePeekHeight returns the minimum vertical extent of a splittable
// block that must travel with a preceding break-after:avoid heading on the
// same page. That's the top padding/border plus the first inner child
// (typically a single code line for <pre>). Returns ok=false for nodes that
// can't be split — callers fall back to the full vlistNodeHeight.
func splittablePeekHeight(n node.Node) (bag.ScaledPoint, bool) {
	vl, ok := n.(*node.VList)
	if !ok || vl.Attributes == nil {
		return 0, false
	}
	if isSplittable, _ := vl.Attributes["_splittable"].(bool); !isSplittable {
		return 0, false
	}
	children, _ := vl.Attributes["_splittableInner"].([]node.Node)
	if len(children) == 0 {
		return 0, false
	}
	hv, _ := vl.Attributes["_splittableHv"].(HTMLValues)
	return hv.PaddingTop + hv.BorderTopWidth + vlistNodeHeight(children[0]), true
}

// vlistNodeHeight returns the vertical extent of a node in a vertical list.
func vlistNodeHeight(n node.Node) bag.ScaledPoint {
	switch t := n.(type) {
	case *node.VList:
		return t.Height + t.Depth
	case *node.HList:
		return t.Height + t.Depth
	case *node.Kern:
		return t.Kern
	case *node.Glue:
		return t.Width
	case *node.Rule:
		return t.Height + t.Depth
	default:
		return 0
	}
}

// ParseHTMLFromNode interprets the HTML structure and applies all previously read CSS data.
func (cb *CSSBuilder) ParseHTMLFromNode(input *html.Node) (*frontend.Text, error) {
	doc := goquery.NewDocumentFromNode(input)
	gq, err := cb.css.ApplyCSS(doc)
	if err != nil {
		return nil, err
	}
	var te *frontend.Text
	n := gq.Nodes[0]
	if te, err = HTMLNodeToText(cb, n, cb.stylesStack, cb.frontend, cb.anchorPages); err != nil {
		return nil, err
	}

	return te, nil
}

// HTMLToText interprets the HTML string and applies all previously read CSS data.
func (cb *CSSBuilder) HTMLToText(html string) (*frontend.Text, error) {
	doc, err := cb.css.ProcessHTMLChunk(html)
	if err != nil {
		return nil, err
	}
	n := doc.Nodes[0]

	// Register @font-face declarations now — ProcessHTMLChunk has just parsed
	// any embedded <style> blocks into cb.css.FontFaces, and the upcoming
	// HTMLNodeToText pass needs the font families resolved to honour
	// font-family lookups against in-document fonts. AddMember is idempotent
	// for repeat (weight, style) keys, so the InitPage call later in the
	// pipeline re-registering the same set is harmless.
	if err := AddFontFamiliesFromCSS(cb.css, cb.frontend); err != nil {
		return nil, err
	}

	var te *frontend.Text
	if te, err = HTMLNodeToText(cb, n, cb.stylesStack, cb.frontend, cb.anchorPages); err != nil {
		return nil, err
	}

	return te, nil
}

// AddCSS reads the CSS instructions in css.
func (cb *CSSBuilder) AddCSS(css string) error {
	curwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cb.css.PushDir(curwd)
	return cb.css.AddCSSText(css)
}

type info struct {
	vl           *node.VList
	hsize        bag.ScaledPoint
	x            bag.ScaledPoint
	marginTop    bag.ScaledPoint
	marginBottom bag.ScaledPoint
	pagebox      []node.Node
	height       bag.ScaledPoint
	hv           HTMLValues
	debug        string
}

func (inf *info) String() string {
	return fmt.Sprintf("mt: %s mb: %s len(pb): %d vl: %v", inf.marginTop, inf.marginBottom, len(inf.pagebox), inf.vl)
}

func hasContents(areaAttributes map[string]string, contentTokens []csshtml.ContentToken) bool {
	if len(contentTokens) > 0 {
		return true
	}
	return areaAttributes["content"] != "none" && areaAttributes["content"] != "normal"
}

type pageMarginBox struct {
	minWidth    bag.ScaledPoint
	maxWidth    bag.ScaledPoint
	areaWidth   bag.ScaledPoint
	areaHeight  bag.ScaledPoint
	hasContents bool
	widthAuto   bool
	halign      frontend.HorizontalAlignment
	x           bag.ScaledPoint
	y           bag.ScaledPoint
	wd          bag.ScaledPoint
	ht          bag.ScaledPoint
}

// ReadCSSFile reads the given file name and tries to parse the CSS contents
// from the file.
func (cb *CSSBuilder) ReadCSSFile(filename string) error {
	slog.Debug("Read file", "filename", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return err
	}
	cb.css.PushDir(abs)
	return cb.css.AddCSSText(string(data))
}
