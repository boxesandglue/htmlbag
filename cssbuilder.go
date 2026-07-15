package htmlbag

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

var onecm = bag.MustSP("1cm")

// HeadingEntry records a heading (h1–h6) or a bookmarked element found
// during VList construction. Page and Y are filled later during OutputPages
// when the element is placed on a page. SE is filled at SE-construction time
// for tagged documents; consumers (PDF outline generator) use it to emit
// structure destinations as required by PDF/UA-2 §8.8.
//
// The bm* fields drive PDF outline (bookmark) generation. They are set from
// the element's heading level (h1→1 … h6→6) and/or the CSS -bag-bookmark
// property. An entry with bmLevel == 0 is recorded for the heading list / TOC
// but is omitted from the PDF outline (e.g. an h2 with -bag-bookmark: none, or
// a non-heading element without -bag-bookmark). Non-heading bookmarks carry an
// empty Level.
type HeadingEntry struct {
	Level string // "h1", "h2", etc.; "" for a non-heading bookmark
	Text  string
	Page  int                        // 1-based page number, 0 until assigned
	SE    *document.StructureElement // nil unless tagging is enabled and this heading was tagged

	Y       bag.ScaledPoint // top edge on the page (PDF user space), for an /XYZ outline destination
	bmLevel int             // resolved outline nesting level (1-based); 0 = not in the outline
	bmOpen  bool            // outline node shows its children expanded
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
	// GenerateOutline controls whether OutputPages / OutputPagesFromText
	// emit a PDF outline (bookmarks) from the collected headings and
	// -bag-bookmark elements. Defaults to true (set in New). Callers that
	// build their own outline (e.g. glu's Markdown pipeline) set it to
	// false to opt out.
	GenerateOutline bool
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
	// rootFontSize captures the font-size resolved on the root element
	// (<html>) during HTMLNodeToText. CSS Paged Media 3 §3.3: the page
	// context inherits from the root element, so margin-box em-based
	// declarations resolve against this value rather than against the
	// body or an internal default. Zero means the document never set a
	// root font-size; BeforeShipout falls back to the CSS initial value
	// (~16px ≈ 12pt) in that case.
	rootFontSize bag.ScaledPoint
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
		GenerateOutline:         true,
	}
	if err := LoadIncludedFonts(fd); err != nil {
		return nil, err
	}

	// Enable automatic structure tagging for PDF/UA (both UA-1 and UA-2)
	if fd.Doc.Format.IsPDFUA() {
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
		if fd.Doc.Format.IsPDFUA2() {
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
	base, hasBase := cb.css.Pages[""]
	pick := func(pseudo csshtml.Page) *csshtml.Page {
		if !hasBase {
			return &pseudo
		}
		merged := mergePageWithBase(pseudo, base)
		return &merged
	}
	if first, ok := cb.css.Pages[":first"]; ok && len(cb.frontend.Doc.Pages) == 0 {
		return pick(first)
	}
	isRight := len(cb.frontend.Doc.Pages)%2 == 0
	if right, ok := cb.css.Pages[":right"]; ok && isRight {
		return pick(right)
	}
	if left, ok := cb.css.Pages[":left"]; ok && !isRight {
		return pick(left)
	}
	if hasBase {
		return &base
	}
	return nil
}

// mergePageWithBase folds the generic @page rule into a pseudo-page
// selection so :first / :left / :right inherit any size, margins,
// declarations and margin boxes the pseudo didn't redeclare.
//
// CSS Paged Media 3 §3.2: pseudo-class page selectors cascade over the
// generic @page rule rather than replacing it. Scalar string fields
// fall back to base when the pseudo leaves them empty; Attributes are
// concatenated in cascade order (base first, pseudo second, so pseudo
// wins in ResolveAttributes); PageArea / PageAreaContent maps are
// unioned, with the pseudo's entry replacing the base's for any area
// declared in both. Without this merge a pseudo that omits "size" or
// "margin" propagates "" into bag.SP and aborts page setup with
// ErrConversion.
func mergePageWithBase(pseudo, base csshtml.Page) csshtml.Page {
	merged := pseudo
	if merged.Papersize == "" {
		merged.Papersize = base.Papersize
	}
	if merged.MarginTop == "" {
		merged.MarginTop = base.MarginTop
	}
	if merged.MarginBottom == "" {
		merged.MarginBottom = base.MarginBottom
	}
	if merged.MarginLeft == "" {
		merged.MarginLeft = base.MarginLeft
	}
	if merged.MarginRight == "" {
		merged.MarginRight = base.MarginRight
	}
	if len(base.Attributes) > 0 {
		combined := make([]html.Attribute, 0, len(base.Attributes)+len(pseudo.Attributes))
		combined = append(combined, base.Attributes...)
		combined = append(combined, pseudo.Attributes...)
		merged.Attributes = combined
	}
	if len(base.PageArea) > 0 {
		union := make(map[string]map[string]string, len(base.PageArea)+len(pseudo.PageArea))
		for k, v := range base.PageArea {
			union[k] = v
		}
		for k, v := range pseudo.PageArea {
			union[k] = v
		}
		merged.PageArea = union
	}
	if len(base.PageAreaContent) > 0 {
		union := make(map[string][]csshtml.ContentToken, len(base.PageAreaContent)+len(pseudo.PageAreaContent))
		for k, v := range base.PageAreaContent {
			union[k] = v
		}
		for k, v := range pseudo.PageAreaContent {
			union[k] = v
		}
		merged.PageAreaContent = union
	}
	return merged
}

// pageBoxMetrics carries the resolved @page border and padding widths so
// InitPage and NewPage can size the content area (PageArea* / Content*) and
// place the body identically. Without a shared source both paths drift: today
// InitPage folds border/padding into the content area but NewPage does not,
// which is one reason the @page border only shows on page 1.
type pageBoxMetrics struct {
	borderLeft, borderRight, borderTop, borderBottom     bag.ScaledPoint
	paddingLeft, paddingRight, paddingTop, paddingBottom bag.ScaledPoint
	// backgroundColor is the resolved @page background-color (nil if unset).
	// Intentionally painted on page 1 only (InitPage), so NewPage ignores it.
	backgroundColor *color.Color
}

// renderPageBorderBox builds an empty vlist the size of the @page content box
// and decorates it with the @page border/background/padding via HTMLBorder.
// The caller is expected to OutputAt(ml, ht-mt, vl) so the border-box outer
// edge coincides with the margin edge (for margin:0 that is the sheet edge,
// giving a full-height left bar). The returned metrics let the caller derive
// the padded content area. res is the already-resolved @page attribute map.
func (cb *CSSBuilder) renderPageBorderBox(res map[string]string, wd, ht, ml, mr, mt, mb bag.ScaledPoint) (*node.VList, pageBoxMetrics, error) {
	styles := cb.stylesStack.PushStyles()
	defer cb.stylesStack.PopStyles()
	if err := StylesToStyles(styles, res, cb.frontend, cb.stylesStack.CurrentStyle().Fontsize); err != nil {
		return nil, pageBoxMetrics{}, err
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
	m := pageBoxMetrics{
		borderLeft:    styles.BorderLeftWidth,
		borderRight:   styles.BorderRightWidth,
		borderTop:     styles.BorderTopWidth,
		borderBottom:  styles.BorderBottomWidth,
		paddingLeft:   styles.PaddingLeft,
		paddingRight:  styles.PaddingRight,
		paddingTop:      styles.PaddingTop,
		paddingBottom:   styles.PaddingBottom,
		backgroundColor: styles.BackgroundColor,
	}
	return vl, m, nil
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

		vl, m, err := cb.renderPageBorderBox(res, wd, ht, ml, mr, mt, mb)
		if err != nil {
			return err
		}

		// set page width / height
		cb.frontend.Doc.DefaultPageWidth = wd
		cb.frontend.Doc.DefaultPageHeight = ht
		cb.currentPageDimensions = PageDimensions{
			Width:         wd,
			Height:        ht,
			PageAreaLeft:  ml + m.borderLeft + m.paddingLeft,
			PageAreaTop:   mt + m.borderTop + m.paddingTop,
			ContentWidth:  wd - ml - mr - m.borderLeft - m.borderRight - m.paddingLeft - m.paddingRight,
			ContentHeight: ht - mt - mb - m.borderTop - m.borderBottom - m.paddingTop - m.paddingBottom,
			MarginTop:     mt,
			MarginBottom:  mb,
			MarginLeft:    ml,
			MarginRight:   mr,
			masterpage:    defaultPage,
		}
		cb.frontend.Doc.NewPage()
		if m.backgroundColor != nil {
			r := node.NewRule()
			x := pdfdraw.NewStandalone().ColorNonstroking(*m.backgroundColor).Rect(0, 0, wd, -ht).Fill()
			r.Pre = x.String()
			rvl := node.Vpack(r)
			rvl.Attributes = node.H{"origin": "page background color"}
			cb.frontend.Doc.CurrentPage.OutputAt(0, ht, rvl)
		}
		if err = cb.drawPageBackgroundImage(res, wd, ht); err != nil {
			return err
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
		// Re-resolve this page master's @page attributes so border/padding
		// (and background-image) apply per page, not only on page 1.
		bgRes, _ := csshtml.ResolveAttributes(pt.Attributes)
		vl, m, err := cb.renderPageBorderBox(bgRes, wd, ht, ml, mr, mt, mb)
		if err != nil {
			return err
		}
		cb.currentPageDimensions.PageAreaLeft = ml + m.borderLeft + m.paddingLeft
		cb.currentPageDimensions.PageAreaTop = mt + m.borderTop + m.paddingTop
		cb.currentPageDimensions.ContentWidth = wd - ml - mr - m.borderLeft - m.borderRight - m.paddingLeft - m.paddingRight
		cb.currentPageDimensions.ContentHeight = ht - mt - mb - m.borderTop - m.borderBottom - m.paddingTop - m.paddingBottom
		// Paint the @page background-image, then the border box, before the
		// body content lands on this page (both sit underneath the text).
		if err := cb.drawPageBackgroundImage(bgRes, wd, ht); err != nil {
			return err
		}
		cb.frontend.Doc.CurrentPage.OutputAt(ml, ht-mt, vl)
	}
	// Store page dimensions on the new page for callback access.
	if pd, err := cb.PageSize(); err == nil {
		storePageDimensions(cb, pd)
	}
	cb.firePageInit()
	return nil
}

// stripCSSURL unwraps a CSS url() token to its bare path, dropping the
// url(...) wrapper and any surrounding single or double quotes.
func stripCSSURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "url(") && strings.HasSuffix(s, ")") {
		s = s[len("url(") : len(s)-1]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

// drawPageBackgroundImage paints a CSS `@page { background-image: url(...) }`
// onto the current page, scaled to fill the whole sheet. The optional custom
// property `-bag-background-page: N` selects the source page of a multi-page
// PDF (default 1), so a two-page letterhead can drive page 1 vs. page 2+ via
// `@page :first` / `@page`. It is called for every page, so per-page @page
// selectors (:first/:left/:right) yield per-page backgrounds without the
// caller keeping its own page counter. A missing or unloadable file is logged
// and skipped rather than aborting the whole render.
func (cb *CSSBuilder) drawPageBackgroundImage(res map[string]string, wd, ht bag.ScaledPoint) error {
	raw, ok := res["background-image"]
	if !ok {
		return nil
	}
	filename := stripCSSURL(raw)
	if filename == "" || filename == "none" {
		return nil
	}
	// FindFile honours both CSS.FileFinder (xts route) and the dirstack
	// (glu/markdown route via PushDir(baseDir)); resolving relative to the
	// document is what the letterhead use case needs.
	if resolved, ferr := cb.css.FindFile(filename); ferr == nil && resolved != "" {
		filename = resolved
	}
	pageno := 1
	if p, ok := res["-bag-background-page"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 {
			pageno = n
		}
	}
	imgf, err := cb.frontend.Doc.LoadImageFileWithBox(filename, "/MediaBox", pageno)
	if err != nil {
		slog.Warn("page background-image could not be loaded", "filename", filename, "page", pageno, "error", err)
		return nil
	}
	imgNode := cb.frontend.Doc.CreateImageNodeFromImagefile(imgf, pageno, "/MediaBox")
	imgNode.Width = wd
	imgNode.Height = ht
	rvl := node.Vpack(imgNode)
	rvl.Attributes = node.H{"origin": "page background image"}
	cb.frontend.Doc.CurrentPage.OutputAt(0, ht, rvl)
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
	if cb.GenerateOutline {
		cb.appendOutline()
	}
	return nil
}

// headingLevel maps an HTML heading tag to its 1-based outline level
// (h1→1 … h6→6). Any other tag returns 0 (not an implicit bookmark).
func headingLevel(tag string) int {
	switch tag {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	case "h6":
		return 6
	}
	return 0
}

// parseBookmark interprets a CSS -bag-bookmark value (already lower-cased).
// Grammar: `none | [<integer>] [open | closed]` — tokens in any order. It
// returns the explicit level (level, hasLevel), the open/closed state
// (default open), and whether `none` was given. The caller combines this with
// the element's implicit heading level to decide the final outline level.
func parseBookmark(raw string) (level int, hasLevel bool, open bool, none bool) {
	open = true // bookmarks default to expanded; `closed` collapses them
	for _, tok := range strings.Fields(raw) {
		switch tok {
		case "none":
			none = true
		case "open":
			open = true
		case "closed":
			open = false
		default:
			if n, err := strconv.Atoi(tok); err == nil && n > 0 {
				level, hasLevel = n, true
			}
		}
	}
	return level, hasLevel, open, none
}

// appendOutline builds a nested PDF outline (bookmarks) from the collected
// heading/bookmark entries and assigns it to the PDF writer. Entries nest by
// their resolved bmLevel: a level-2 entry becomes a child of the most recent
// entry with a lower level, and so on. Level jumps (e.g. 1 → 3 with no 2 in
// between) attach to the nearest shallower ancestor, so a missing intermediate
// level can never orphan a child. Entries with bmLevel == 0 (TOC-only or
// -bag-bookmark: none) and entries without a page are skipped. Must run after
// every page has shipped out so page object numbers are assigned.
func (cb *CSSBuilder) appendOutline() {
	fe := cb.frontend
	ua2 := fe.Doc.Format.IsPDFUA2()
	type stackItem struct {
		level int
		ol    *pdf.Outline
	}
	var stack []stackItem
	for i := range cb.Headings {
		h := &cb.Headings[i]
		if h.bmLevel <= 0 || h.Page <= 0 || h.Page > len(fe.Doc.Pages) {
			continue
		}
		var dest string
		if ua2 && h.SE != nil {
			// PDF/UA-2 §8.8: intra-document destinations must be structure
			// destinations. Pre-allocate the SE object now so Finish() reuses
			// it; the outline /Dest then targets the StructElem directly.
			if h.SE.Obj == nil {
				h.SE.Obj = fe.Doc.PDFWriter.NewObject()
			}
			dest = fmt.Sprintf("[%s /Fit]", h.SE.Obj.ObjectNumber.Ref())
		} else {
			// /XYZ jumps to the heading's exact vertical position (its top
			// edge), keeping the current horizontal scroll and zoom (null).
			pg := fe.Doc.Pages[h.Page-1]
			dest = fmt.Sprintf("[%s /XYZ null %0.5g null]", pg.Objectnumber.Ref(), h.Y.ToPT())
		}
		o := &pdf.Outline{Title: h.Text, Dest: dest, Open: h.bmOpen}
		for len(stack) > 0 && stack[len(stack)-1].level >= h.bmLevel {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			fe.Doc.PDFWriter.Outlines = append(fe.Doc.PDFWriter.Outlines, o)
		} else {
			parent := stack[len(stack)-1].ol
			parent.Children = append(parent.Children, o)
		}
		stack = append(stack, stackItem{level: h.bmLevel, ol: o})
	}
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
	if cb.GenerateOutline {
		cb.appendOutline()
	}
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

// hasTableChild reports whether the node list nl contains a table VList
// (origin "table"), descending through transparent single-child wrapper VLists
// (a plain <div> around the table). It gates the wrapper flatten in
// outputGroupNodes: a transparent wrapper taller than the page is only
// unwrapped when it holds a table (the breakable part), so other tall
// transparent blocks keep their current handling.
func hasTableChild(nl node.Node) bool {
	for n := nl; n != nil; n = n.Next() {
		vl, ok := n.(*node.VList)
		if !ok || vl.Attributes == nil {
			continue
		}
		if o, _ := vl.Attributes["origin"].(string); o == "table" {
			return true
		}
		// Descend a transparent single-child wrapper (e.g. <div><table>).
		if spl, _ := vl.Attributes["_splittable"].(bool); !spl && vl.List != nil && vl.List.Next() == nil {
			if hasTableChild(vl.List) {
				return true
			}
		}
	}
	return false
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

		// A table taller than the page must break across pages, but such a
		// table is often nested inside a transparent wrapper (a plain
		// <div>, no bg/border) reached mid-flow after a
		// margin-top kern. The top-level unwrap only exposes a wrapper's
		// children when it is the wrapper's sole child, so a multi-child
		// wrapper (opening text + table + totals) stays monolithic and is
		// shipped whole, leaving page 1 empty under the margin-top. When such
		// a too-tall wrapper contains a too-tall table, splice its children
		// into the sibling chain so each child is processed at top level,
		// where the table reaches the repeating-header / row-splice paths
		// below. A styled wrapper (bg/border) is _splittable and handled by
		// outputBlockSplit instead, so it is left intact. Nested wrappers are
		// unwrapped progressively: each spliced child is re-examined here.
		if wrap, ok := cur.(*node.VList); ok && wrap.Attributes != nil && cb.pageBufHeight+h > contentArea {
			o, _ := wrap.Attributes["origin"].(string)
			spl, _ := wrap.Attributes["_splittable"].(bool)
			if o != "table" && !spl && wrap.List != nil && hasTableChild(wrap.List) {
				propagateInsertsAttr(wrap, wrap.List)
				first := wrap.List
				last := node.Tail(first)
				last.SetNext(next)
				if next != nil {
					next.SetPrev(last)
				}
				first.SetPrev(nil)
				cur = first
				continue
			}
		}

		// Special path: tables with repeating headers. outputTableRows
		// places rows directly via OutputAt, flowing them across page
		// breaks with the header repeated on every page. Take it whenever
		// the table does not fit in the space still free on this page
		// (already-buffered content included): a table taller than a full
		// page always splits, and a table that would fit on an empty page
		// but not in the remaining space starts here and breaks rather
		// than shipping whole to the next page and leaving a gap (e.g.
		// when a large top reservation eats most of page 1). Short tables
		// that do fit in the remaining space fall through to the normal
		// pageBuf path, which composes them with adjacent paragraphs.
		if tableVL, ok := cur.(*node.VList); ok && tableVL.Attributes != nil {
			if buildHeadersFn, tok := tableVL.Attributes["_buildHeaders"]; tok {
				tableIncoming := insertsOnNode(cur)
				tableInsertsH := cb.totalFloatTopHeight(filterInserts(tableIncoming, InsertFloatTop)) +
					cb.totalFloatBottomHeight(filterInserts(tableIncoming, InsertFloatBottom)) +
					cb.totalFootnoteHeight(filterInserts(tableIncoming, InsertFootnote))
				if cb.pageBufHeight+h+tableInsertsH > contentArea {
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
					// Siblings after the table continue on the table's
					// last page. The body buffer paints from the top of the
					// content area at flushInserts, so buffer a spacer
					// covering the height the directly-placed rows consumed;
					// following blocks then land right below the last row,
					// and the normal fit checks break to a new page only
					// when a block really doesn't fit anymore.
					if next != nil {
						usedH := pd.Height - pd.PageAreaTop - yLocal
						if usedH > 0 {
							k := node.NewKern()
							k.Kern = usedH
							spacer := node.Vpack(k)
							spacer.Attributes = node.H{"origin": "table continuation spacer"}
							cb.bufferBody(spacer, usedH, -1, nil)
						}
					}
					cur = next
					continue
				}
			}
		}

		// A table without repeating headers (no thead/tfoot) that is
		// taller than a full page must break across pages. When such a table
		// is the group's sole child, the unwrap loop above already exposes its
		// rows as top-level siblings, so they buffer and break one by one. But
		// a preceding sibling (e.g. a margin-top kern emitted by a wrapper
		// above the table) blocks that unwrap, leaving the table as
		// one monolithic VList taller than the page. `cur` was resolved to the
		// table above; splice its row HLists into the sibling chain so it
		// breaks like any other block sequence. (thead/tfoot tables took the
		// _buildHeaders path above and never reach here.)
		if tableVL, ok := cur.(*node.VList); ok && tableVL.Attributes != nil {
			o, _ := tableVL.Attributes["origin"].(string)
			_, hasHeaders := tableVL.Attributes["_buildHeaders"]
			if o == "table" && !hasHeaders && cb.pageBufHeight+h > contentArea && tableVL.List != nil {
				// Move any cell inserts onto the first row so they are still
				// reserved once the wrapper VList is dropped.
				propagateInsertsAttr(tableVL, tableVL.List)
				first := tableVL.List
				last := node.Tail(first)
				last.SetNext(next)
				if next != nil {
					next.SetPrev(last)
				}
				first.SetPrev(nil)
				cur = first
				continue
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

		// Widow / orphan protection: count content children. A splittable
		// block has two shapes: line-level children (a <pre> is HList lines
		// interleaved with Glue) and block-level children (a bordered card
		// is VList paragraphs/divs interleaved with margin Kerns). Both an
		// HList and a VList count as one unit of content here; only the
		// Glue/Kern fillers between them are skipped. Counting VLists is
		// load-bearing: without it a card whose children are all VLists
		// reports zero "lines", so the orphan branch below fires on every
		// page and shunts the whole card forward — orphaning a preceding
		// page-break-after:avoid heading (it stays put while its card jumps).
		// CSS Fragmentation 3 §4 spec defaults are widows: 2 and orphans: 2.
		const minLines = 2
		countHL := func(items []node.Node) int {
			n := 0
			for _, c := range items {
				switch c.(type) {
				case *node.HList, *node.VList:
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
