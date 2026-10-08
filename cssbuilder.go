package htmlbag

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/font"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
	"golang.org/x/net/html"
)

var onecm = bag.MustSP("1cm")

// fragLines holds the CSS widows and orphans of a block: the number of
// content children (HList lines or VList blocks) a fragment must carry at the
// top of a page and leave at the bottom of one. outputBlockSplit enforces
// them, and splittablePeekHeight must predict them — the two would otherwise
// disagree about whether a block can start on the current page.
type fragLines struct{ widows, orphans int }

// defaultFragLines is the initial value of both properties (CSS
// Fragmentation 3 §4.3).
var defaultFragLines = fragLines{widows: 2, orphans: 2}

const attrFragLines = "_fragLines"

// setFragLines records the widows and orphans of an element's block Text
// when they differ from the initial value.
func (cb *CSSBuilder) setFragLines(te *frontend.Text, fl fragLines) {
	if cb == nil || fl == defaultFragLines {
		return
	}
	if cb.fragLines == nil {
		cb.fragLines = map[*frontend.Text]fragLines{}
	}
	cb.fragLines[te] = fl
}

// stampFragLines puts the widows and orphans of te on the splittable node
// built from it.
func (cb *CSSBuilder) stampFragLines(attrs node.H, te *frontend.Text) {
	if fl, ok := cb.fragLines[te]; ok {
		attrs[attrFragLines] = fl
	}
}

// fragLinesOf returns the widows and orphans of a splittable block.
func fragLinesOf(vl *node.VList) fragLines {
	if fl, ok := vl.Attributes[attrFragLines].(fragLines); ok {
		return fl
	}
	return defaultFragLines
}

// HeadingEntry records a heading (h1–h6) or a bookmarked element found
// during VList construction. Page and Y are filled later during OutputPagesFromText
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
	// Counters holds the CSS counter state at the anchor, keyed by
	// counter name. Each value is the root-first chain of nested
	// counter values (cf. StylesStack.CounterValues): the last element
	// feeds target-counter(), the whole chain target-counters(). The
	// "page" counter is not part of this map; it lives in Page because
	// its value is only known at shipout.
	Counters map[string][]int
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

// LineModelStyles is what a LineModelFunc is told about the paragraph whose
// -bag-leading-model names it. Fields may be added.
type LineModelStyles struct {
	// Name is the -bag-leading-model name, lower case.
	Name       string
	FontSize   bag.ScaledPoint
	LineHeight bag.ScaledPoint
	// Language is the paragraph's BCP 47 language tag, "" when unset.
	Language string
	// Font is the paragraph's own font at FontSize, with the face's
	// vertical metrics as a glyph's font carries them, for the strut of a
	// line (CSS 2.1 §10.8.1): a line without glyphs, such as one between two
	// <br>, has no font of its own to take its height from. nil when the
	// paragraph's font cannot be loaded.
	Font *font.Font
}

// LineModelFunc makes the line model (node.LineModel) for a paragraph whose
// -bag-leading-model names it; see CSSBuilder.RegisterLineModel. A nil model
// keeps the built-in leading. It may be called more than once per paragraph.
type LineModelFunc func(LineModelStyles) node.LineModel

// BreakerStyles is what a BreakerFunc is told about the paragraph whose
// -bag-line-breaker names it. Fields may be added.
type BreakerStyles struct {
	// Name is the -bag-line-breaker name, lower case.
	Name     string
	FontSize bag.ScaledPoint
	// Language is the paragraph's BCP 47 language tag, "" when unset.
	Language string
}

// BreakerFunc makes the breaker (node.Breaker) for a paragraph whose
// -bag-line-breaker names it; see CSSBuilder.RegisterBreaker. A nil breaker
// keeps Knuth-Plass. It may be called more than once per paragraph.
type BreakerFunc func(BreakerStyles) node.Breaker

// CSSBuilder handles HTML chunks and CSS instructions.
type CSSBuilder struct {
	pagebox               []node.Node
	currentPageDimensions PageDimensions
	frontend              *frontend.Document
	css                   *CSS
	stylesStack           StylesStack
	smcpFaces             map[*frontend.FontSource]bool
	structureRoot         *document.StructureElement
	structureCurrent      *document.StructureElement
	enableTagging         bool
	lineModels            map[string]LineModelFunc
	breakers              map[string]BreakerFunc
	// strutFonts caches LineModelStyles.Font.
	strutFonts       map[strutKey]*font.Font
	warnedLineModels map[string]bool
	warnedBreakers   map[string]bool
	// warnedColumns is set once column-count has been warned about.
	warnedColumns bool
	// multicols holds the column properties of a body with column-count,
	// gap resolved.
	multicols map[*frontend.Text]multicol
	// inlineNodes are the nodes InlineNode stands in for, by element.
	inlineNodes      map[*html.Node]node.Node
	ElementCallback  ElementCallbackFunc
	PageInitCallback PageInitCallbackFunc
	// Counters holds named counter values used when evaluating CSS content
	// properties (e.g. "page" for the current page, "pages" for the total).
	// The "page" counter is set automatically during shipout; other counters
	// (like "pages") should be set by the caller.
	Counters     map[string]int
	headingCount int
	// Headings collects all h1–h6 headings encountered during VList
	// construction. Page numbers are assigned during OutputPagesFromText.
	Headings []HeadingEntry
	// GenerateOutline controls whether OutputPagesFromText
	// emits a PDF outline (bookmarks) from the collected headings and
	// -bag-bookmark elements. Defaults to true (set in New). Callers that
	// build their own outline (e.g. glu's Markdown pipeline) set it to
	// false to opt out.
	GenerateOutline bool
	// TraceBoxModel paints a translucent overlay visualizing the CSS box
	// model of block-level elements (margin / border / padding / content,
	// devtools color scheme). Overlapping margins of neighboring blocks
	// tint twice and show up darker. Debug aid; toggled by consumers such
	// as xts' <Trace boxmodel="yes">. When the document format forbids
	// transparency (PDF/A-1, PDF/X-3) the overlay falls back to thin
	// colored outlines.
	TraceBoxModel bool
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
	// anchorCounters maps anchor id → counter snapshot from the
	// *previous* render pass (CSS target-counter() / target-counters()
	// with counters other than "page"). Same lifecycle as anchorPages.
	anchorCounters map[string]map[string][]int
	// previousPassReads collects what this pass read of anchorPages,
	// anchorTexts, anchorCounters and Counters["pages"].
	previousPassReads PreviousPassReads
	// anchorSnapshots carries counter snapshots for block-level anchors
	// within the *current* pass, from the HTML walk (where the styles
	// stack with its counters is live) to the VList builder (which runs
	// after formatting, when the stack is gone, and creates the
	// AnchorEntry). Keyed by element id; ids are document-unique.
	anchorSnapshots map[string]map[string][]int
	// PendingVLists stores pre-rendered VLists keyed by a unique ID.
	// Used to pass already-rendered content (e.g. group contents) through
	// the HTML/CSS pipeline: an element with data-vlist-id="ID" stands for
	// the VList, in a table cell or as a block of its own.
	PendingVLists map[string]*node.VList
	// placeholderIDs holds the data-vlist-id values the HTML walk has met,
	// each of which stands for one box.
	placeholderIDs map[string]bool
	// sourceNodes holds the node of the document each Text of the last
	// HTMLToText comes from (noteSource).
	sourceNodes map[*frontend.Text]*html.Node
	// autoMargins holds the Texts of blocks with an auto side margin.
	autoMargins map[*frontend.Text]autoMargin
	// trims holds the Texts of blocks with text-box-trim (trimLines).
	trims map[*frontend.Text]textBoxTrim
	// ownTrims holds the text-box-trim a block sets itself, without what a
	// container hands on to it (passTrimDown); fragmentTrim reads it.
	ownTrims map[*frontend.Text]textBoxTrim
	// clones holds the Texts of blocks with box-decoration-break: clone.
	clones map[*frontend.Text]bool
	// breakTrims holds the Texts of blocks with -bag-text-box-trim-at-break.
	breakTrims map[*frontend.Text]bool
	// liftedFloats maps a float lifted from the start of a paragraph
	// (liftLeadingFloats) to that paragraph.
	liftedFloats map[any]*frontend.Text
	// floatGutters holds the -bag-float-gutter of each floated element.
	floatGutters map[*frontend.Text]bag.ScaledPoint
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
	// tableRestores puts back the float/clear sentinels and the inline-id
	// markers stripped from the cell content that reaches frontend.BuildTable
	// unbuilt. They cannot be restored where they are taken: the cell's Text
	// is formatted by BuildTable much later, so the restore belongs at the end
	// of buildTable.
	tableRestores []func()
	tableInserts  []*Insert
	// tableRowAnchors holds, per row of the in-flight table in tbl.Rows
	// order, the AnchorEntry indices of the inline ids in its cells.
	tableRowAnchors [][]int
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
	// initial containing block = the page box, i.e. the physical
	// sheet from the page edges, independent of the @page margins).
	// A position: absolute element resolves top/right/bottom/left
	// against the top entry. Pushed/popped by Output() on entering
	// and leaving every element whose computed position is anything
	// but static; primed by NewPage() with the page-box entry so
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
	// runningElements holds the body Text of every element removed from
	// the normal flow via `position: running(name)` (CSS GCPM running
	// elements), keyed by name. Filled during HTMLNodeToText; consumed by
	// BeforeShipout when a page margin box declares
	// `content: element(name)`. The Text is re-formatted per page at the
	// margin box width (Mknodes/FormatParagraph are idempotent), so the
	// same footer can repeat on every page. When the same name is
	// captured more than once, the first occurrence wins (GCPM `first`).
	runningElements map[string]*frontend.Text
	// textRunning holds the running-element names the last HTMLNodeToText met,
	// true for those it added to runningElements, for FlowText to drop.
	textRunning map[string]bool
	// fragLines holds widows and orphans off the Settings: they inherit to
	// every block, and a private setting would have to be stripped on each
	// path that hands a Text to FormatParagraph (cells, footnotes, floats).
	fragLines map[*frontend.Text]fragLines
	// flowing is set while OutputPagesFromText or FlowText runs.
	flowing bool
	// callerFlow is FlowText's cursor while it runs: the build takes the
	// page parity from its region, as the paginator checks it there.
	callerFlow *flowCursor
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
	// reflowRebuild is true while OutputPagesFromText rebuilds the not yet
	// placed rest of a page-break group because an automatic page break
	// switched to a page with a different content width. The VList builder
	// then skips heading/anchor registration and the ElementCallback — the
	// entries from the first build stay valid and their indices are
	// transferred onto the rebuilt nodes.
	reflowRebuild bool
}

// New creates an instance of the CSSBuilder.
func New(fd *frontend.Document, c *CSS) (*CSSBuilder, error) {
	cb := CSSBuilder{
		css:                     c,
		frontend:                fd,
		stylesStack:             make(StylesStack, 0),
		pagebox:                 []node.Node{},
		Counters:                map[string]int{},
		PendingVLists:           map[string]*node.VList{},
		pageInserts:             map[InsertClass][]*Insert{},
		runningElements:         map[string]*frontend.Text{},
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
	masterpage    *Page
}

// pageAreaBottom returns the offset of the content area's bottom edge from
// the sheet's bottom edge: margin + @page border + @page padding. Without
// @page border/padding this equals MarginBottom.
func (pd PageDimensions) pageAreaBottom() bag.ScaledPoint {
	return pd.Height - pd.PageAreaTop - pd.ContentHeight
}

// PageAreas returns the CSS page margin box areas (e.g. "@top-center")
// for the current page type, or nil if no @page rule is active.
func (pd PageDimensions) PageAreas() map[string]StyleMap {
	if pd.masterpage == nil {
		return nil
	}
	return pd.masterpage.PageArea
}

// CSS returns the underlying CSS parser.
func (cb *CSSBuilder) CSS() *CSS {
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

// SetAnchorCounters installs the id → counter snapshot map collected on
// the previous render pass. The CSS evaluator reads this when resolving
// target-counter() / target-counters() references to counters other
// than "page". Pass nil to clear.
func (cb *CSSBuilder) SetAnchorCounters(m map[string]map[string][]int) {
	cb.anchorCounters = m
}

// PreviousPassReads is what a render pass read of the data a caller
// installs from the previous pass.
type PreviousPassReads struct {
	// Pages is set when a page margin box evaluated counter(pages), which
	// the caller sets in Counters.
	Pages bool
	// Anchors holds the ids whose page, text or counters a
	// target-counter(), target-counters() or target-text() looked up, also
	// when the lookup found nothing.
	Anchors map[string]bool
}

// PreviousPassReads returns what the CSSBuilder read so far of the data
// installed with SetAnchorPages, SetAnchorTexts, SetAnchorCounters and the
// "pages" counter. A multi-pass caller needs another pass only when one of
// these values changed; a document that reads none of them comes out the
// same in every pass.
func (cb *CSSBuilder) PreviousPassReads() PreviousPassReads {
	return cb.previousPassReads
}

// notePreviousPassReads records the anchors that the target functions in
// tokens look up (see PreviousPassReads).
func (cb *CSSBuilder) notePreviousPassReads(tokens []ContentToken, attrLookup func(string) string) {
	for _, tok := range tokens {
		switch tok.Type {
		case ContentTargetCounter, ContentTargetCounters, ContentTargetText:
			if id := resolveTargetID(tok, attrLookup); id != "" {
				if cb.previousPassReads.Anchors == nil {
					cb.previousPassReads.Anchors = make(map[string]bool)
				}
				cb.previousPassReads.Anchors[id] = true
			}
		}
	}
}

// recordAnchorSnapshot stores the current counter state for a block
// element carrying an id. Called during the HTML walk (Output), read
// back by the VList builder when it turns the id into an AnchorEntry.
func (cb *CSSBuilder) recordAnchorSnapshot(id string, ss StylesStack) {
	snap := ss.CounterSnapshot()
	if snap == nil {
		return
	}
	if cb.anchorSnapshots == nil {
		cb.anchorSnapshots = map[string]map[string][]int{}
	}
	cb.anchorSnapshots[id] = snap
}

// pageIsRight reports whether the page being laid out is a right (recto)
// page: the first page is, and the parity alternates from there. An open page
// is already in the document's page list; before the first one exists, the
// page about to be made is page one. Counting the list alone made every even
// page a right one from page two on. In FlowText it is the current region's
// page.
func (cb *CSSBuilder) pageIsRight() bool {
	if cb.callerFlow != nil {
		return cb.callerFlow.cur.isRight()
	}
	n := len(cb.frontend.Doc.Pages)
	if cb.frontend.Doc.CurrentPage == nil {
		n++
	}
	return n%2 == 1
}

func (cb *CSSBuilder) getPageType() *Page {
	base, hasBase := cb.css.Pages[""]
	pick := func(pseudo Page) *Page {
		if !hasBase {
			return &pseudo
		}
		merged := mergePageWithBase(pseudo, base)
		return &merged
	}
	if first, ok := cb.css.Pages[":first"]; ok && len(cb.frontend.Doc.Pages) == 0 {
		return pick(first)
	}
	isRight := cb.pageIsRight()
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
// wins in resolveDeclarations); PageArea / PageAreaContent maps are
// unioned, with the pseudo's entry replacing the base's for any area
// declared in both. Without this merge a pseudo that omits "size" or
// "margin" propagates "" into bag.SP and aborts page setup with
// ErrConversion.
func mergePageWithBase(pseudo, base Page) Page {
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
		combined := make([]declaration, 0, len(base.Attributes)+len(pseudo.Attributes))
		combined = append(combined, base.Attributes...)
		combined = append(combined, pseudo.Attributes...)
		merged.Attributes = combined
	}
	if len(base.PageArea) > 0 {
		union := make(map[string]StyleMap, len(base.PageArea)+len(pseudo.PageArea))
		for k, v := range base.PageArea {
			union[k] = v
		}
		for k, v := range pseudo.PageArea {
			union[k] = v
		}
		merged.PageArea = union
	}
	if len(base.PageAreaContent) > 0 {
		union := make(map[string][]ContentToken, len(base.PageAreaContent)+len(pseudo.PageAreaContent))
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
// the padded content area. res is the already-resolved @page style map.
func (cb *CSSBuilder) renderPageBorderBox(res StyleMap, wd, ht, ml, mr, mt, mb bag.ScaledPoint) (*node.VList, pageBoxMetrics, error) {
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
		borderLeft:      styles.BorderLeftWidth,
		borderRight:     styles.BorderRightWidth,
		borderTop:       styles.BorderTopWidth,
		borderBottom:    styles.BorderBottomWidth,
		paddingLeft:     styles.PaddingLeft,
		paddingRight:    styles.PaddingRight,
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
	if err := AddColorsFromCSS(cb.css, cb.frontend); err != nil {
		return err
	}
	var err error
	if defaultPage := cb.getPageType(); defaultPage != nil {
		wdStr, htStr := PapersizeWidthHeight(defaultPage.Papersize)
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
		res := resolveDeclarations(defaultPage.Attributes)

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

// NewPage puts the current page into the PDF document and starts with a new page.
func (cb *CSSBuilder) NewPage() error {
	if err := cb.InitPage(); err != nil {
		return err
	}
	// Flush accumulated inserts onto this page before it ships out.
	if err := cb.flushInserts(); err != nil {
		return err
	}
	return cb.shipoutAndStartPage()
}

// shipoutAndStartPage is NewPage after the flush: it ships the current page
// out and starts the next one.
func (cb *CSSBuilder) shipoutAndStartPage() error {
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
		bgRes := resolveDeclarations(pt.Attributes)
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

// drawPageBackgroundImage paints a CSS `@page { background-image: url(...) }`
// onto the current page, scaled to fill the whole sheet. The optional custom
// property `-bag-background-page: N` selects the source page of a multi-page
// PDF (default 1), so a two-page letterhead can drive page 1 vs. page 2+ via
// `@page :first` / `@page`. It is called for every page, so per-page @page
// selectors (:first/:left/:right) yield per-page backgrounds without the
// caller keeping its own page counter. A missing or unloadable file is logged
// and skipped rather than aborting the whole render.
func (cb *CSSBuilder) drawPageBackgroundImage(res StyleMap, wd, ht bag.ScaledPoint) error {
	// The url() token carries the path the CSS parser already resolved against
	// the declaring stylesheet (issue #3). Nothing to unwrap, nothing to look
	// up a second time.
	filename, ok := res["background-image"].uri()
	if !ok || filename == "" || filename == "none" {
		return nil
	}
	pageno := 1
	if p, ok := res["-bag-background-page"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(p.String())); err == nil && n > 0 {
			pageno = n
		}
	}
	imgf, err := cb.frontend.Doc.LoadImageFileWithBox(filename, "/MediaBox", pageno)
	if err != nil {
		bag.Logger.Warn("page background-image could not be loaded", "filename", filename, "page", pageno, "error", err)
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
//
// When an automatic page break switches to a page whose @page rule yields a
// different content width (e.g. @page :first vs. @page with other horizontal
// margins), the not-yet-placed rest of the group is rebuilt at the new width
// so its lines flow to the new measure (incremental relayout). The rebuild
// happens on the item/Text level: outputGroupNodes reports the first whole
// body item that has not been placed, and the loop below re-runs CreateVlist
// on the remaining items. Recorded heading/anchor indices and extracted
// inserts are transferred from the discarded nodes onto the rebuilt ones
// (see reflowRebuild). Groups whose pages share one content width — the
// common case — never restart and take the unchanged fast path.
//
// OutputPagesFromText returns an error when it is called while it or
// FlowText is running on the same builder, such as from a PageInitCallback.
func (cb *CSSBuilder) OutputPagesFromText(te *frontend.Text) error {
	done, err := cb.startFlow()
	if err != nil {
		return err
	}
	defer done()
	// Page-width rebuilds reuse the item Texts, so the map is needed until
	// the last group is placed and no longer.
	defer func() { cb.fragLines = nil }()

	fc := &flowCursor{regions: &pageRegions{cb: cb}}
	if count, gap := cb.columns(findBody(te)); count > 1 {
		fc.regions, fc.columns = &columnRegions{cb: cb, count: count, gap: gap}, true
	}
	if _, err := cb.flowText(te, fc); err != nil {
		return err
	}
	if err := fc.regions.filled(filled{}); err != nil {
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

// flowText pours the body of te into the regions of fc, all but handing the
// last region back. For a caller's regions it returns the margin-bottom that
// ends the flow.
func (cb *CSSBuilder) flowText(te *frontend.Text, fc *flowCursor) (bag.ScaledPoint, error) {
	// Find the body-level Text element (unwrap html > body wrappers).
	body := findBody(te)

	// Split body items into groups at pageBreakBefore boundaries.
	groups := splitTextAtPageBreaks(body, fc.forcedKeyword)

	var marginAfter bag.ScaledPoint
	var children *flowChildren
	if fc.caller {
		children = newFlowChildren(body, cb.sourceNodes)
	}
	groupStart := 0
	for i, group := range groups {
		if i > 0 {
			groupStart += len(groups[i-1])
		}
		var brk string
		if t, ok := group[0].(*frontend.Text); ok {
			brk = textBreakBefore(t, fc.forcedKeyword)
		}
		if i == 0 {
			if err := fc.start(brk); err != nil {
				return 0, err
			}
		} else if err := fc.breakTo(brk, nil); err != nil {
			return 0, err
		}

		items := group
		rebuild := false
		fc.rebuiltIn = 0
		var carry map[int]node.H
		for {
			// Create a wrapper Text with the body's settings for this group.
			wrapper := &frontend.Text{
				Settings: body.Settings,
				Items:    items,
			}
			if fc.caller {
				// The build collapses the last child's margin-bottom into
				// the wrapper's, which is read below: a copy keeps it off
				// the body and apart from the other groups.
				wrapper.Settings = maps.Clone(body.Settings)
			}

			cb.reflowRebuild = rebuild
			vl, err := cb.CreateVlist(wrapper, fc.cur.width)
			cb.reflowRebuild = false
			if err != nil {
				return 0, err
			}
			stampGroupItemIndices(wrapper, vl)
			if children != nil {
				children.stamp(vl, groupStart+len(group)-len(items))
			}
			if rebuild {
				// The collapsed margin kern preceding the restart item was
				// already buffered by the previous pass; the rebuilt chain
				// must not add it a second time.
				dropLeadingMarginKern(vl)
				applyReflowCarry(vl, carry)
			}
			marginAfter, _ = wrapper.Settings[frontend.SettingMarginBottom].(bag.ScaledPoint)

			// Place nodes from this group's vlist onto pages.
			// Within a group there are no forced page breaks, but content may
			// overflow and require automatic page breaks.
			restart, c, err := cb.outputGroupNodes(vl, fc)
			if err != nil {
				return 0, err
			}
			if restart < 0 {
				break
			}
			// Rebuild the remaining items at the changed content width. The
			// carried attributes are keyed by the item index relative to the
			// slice the next CreateVlist call will see.
			carry = make(map[int]node.H, len(c))
			for k, v := range c {
				carry[k-restart] = v
			}
			items = items[restart:]
			rebuild = true
			fc.rebuiltIn = fc.serial
		}
	}
	return marginAfter, nil
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

// textBreakBefore is the forced break-before keyword of t, as forced reports
// it, or of its first block, which passes it on to t (CSS Fragmentation 3
// §3.1), or "".
func textBreakBefore(t *frontend.Text, forced func(any) string) string {
	for depth := 0; depth < 32 && t != nil; depth++ {
		if k := forced(t.Settings[frontend.SettingPageBreakBefore]); k != "" {
			return k
		}
		var first *frontend.Text
	items:
		for _, itm := range t.Items {
			switch v := itm.(type) {
			case *frontend.Text:
				first = v
				break items
			case string:
				if strings.TrimSpace(v) != "" {
					break items
				}
			default:
				break items
			}
		}
		t = first
	}
	return ""
}

// splitTextAtPageBreaks splits the Items of a body-level Text into groups.
// A new group starts whenever a child Text carries a CSS forced break-before
// keyword (`always`, `page`, `left`, `right`, `recto`, `verso`, `all`, and
// `column` in FlowText), as forced reports it.
func splitTextAtPageBreaks(body *frontend.Text, forced func(any) string) [][]any {
	var groups [][]any
	var current []any

	for _, itm := range body.Items {
		if t, ok := itm.(*frontend.Text); ok {
			if pbb, ok := t.Settings[frontend.SettingPageBreakBefore]; ok && forced(pbb) != "" {
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

// reflowCarryKeys are the node attributes that must survive a width-change
// rebuild: they were recorded (headings/anchors) or extracted (inserts) on
// the first build and cannot be re-created by the rebuild, whose Text tree
// has already been consumed once.
var reflowCarryKeys = []string{"_heading_idx", "_anchor_idx", "_anchor_indices", "inserts"}

// stampGroupItemIndices marks each direct VList child of a group vlist with
// the index of the wrapper item that produced it. buildVlistInternal's box
// branch emits exactly one VList per non-whitespace *frontend.Text item (all
// other direct children are margin/padding kerns), so items and VList
// children correspond 1:1 in order. The index lets the paginator restart the
// group at a whole-item boundary when an automatic page break changes the
// content width. Wrappers that did not take the box branch (or were wrapped
// by HTMLBorder) are left unstamped — pagination then keeps the old
// single-width behavior.
func stampGroupItemIndices(wrapper *frontend.Text, vl *node.VList) {
	stampItemIndices(wrapper, vl, "_groupItemIdx")
}

// containerItemIdxKey is the stamp on the children of a splittable block
// container. A key of its own, because outputGroupNodes unwraps a body whose
// only child is a container and then walks that container's children as the
// group's chain: a _groupItemIdx on them would be read against the body's
// items.
const containerItemIdxKey = "_containerItemIdx"

// stampItemIndices is stampGroupItemIndices for any container built by the
// box branch, with the stamp under key.
func stampItemIndices(wrapper *frontend.Text, vl *node.VList, key string) {
	if isBox, ok := wrapper.Settings[frontend.SettingBox].(bool); !ok || !isBox {
		return
	}
	if vl.Attributes == nil {
		return
	}
	if o, _ := vl.Attributes["origin"].(string); o != "buildVListInternal" {
		return
	}
	items := wrapper.Items
	itemIdx := 0
	nextItemIdx := func() int {
		for itemIdx < len(items) {
			t, ok := items[itemIdx].(*frontend.Text)
			if !ok {
				itemIdx++
				continue
			}
			// Mirror the box branch's skip condition for whitespace-only
			// text between elements.
			if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
				itemIdx++
				continue
			}
			idx := itemIdx
			itemIdx++
			return idx
		}
		return -1
	}
	for n := vl.List; n != nil; n = n.Next() {
		child, ok := n.(*node.VList)
		if !ok {
			continue
		}
		idx := nextItemIdx()
		if idx < 0 {
			return
		}
		if child.Attributes == nil {
			child.Attributes = node.H{}
		}
		child.Attributes[key] = idx
		if built, ok := narrowingFloatParity(child); ok {
			child.Attributes["_floatParity"] = built
		}
	}
}

// narrowingFloatParity reports the page parity an item's inside/outside float
// was resolved against, if the item holds one that narrows the text beside it.
// Every float in one build pass assumed the same page, so the first one found
// speaks for the item.
func narrowingFloatParity(vl *node.VList) (bool, bool) {
	if narrows, _ := vl.Attributes[attrFloatNarrows].(bool); narrows {
		built, _ := vl.Attributes[attrFloatBuiltRight].(bool)
		return built, true
	}
	for n := vl.List; n != nil; n = n.Next() {
		if child, ok := n.(*node.VList); ok {
			if built, ok := narrowingFloatParity(child); ok {
				return built, true
			}
		}
	}
	return false, false
}

// dropLeadingMarginKern removes a leading collapsed-margin kern from a
// rebuilt group vlist. The kern belongs between the last placed item and the
// restart item and was already buffered by the pass that detected the width
// change; keeping it would double the vertical gap.
func dropLeadingMarginKern(vl *node.VList) {
	k, ok := vl.List.(*node.Kern)
	if !ok || k.Attributes == nil {
		return
	}
	if o, _ := k.Attributes["origin"].(string); o != "margin" {
		return
	}
	next := k.Next()
	if next == nil {
		return
	}
	next.SetPrev(nil)
	vl.List = next
	vl.Height -= k.Kern
}

// applyReflowCarry merges the carried attributes (see reflowCarryKeys) onto
// the rebuilt items, matched by their stamped group item index.
func applyReflowCarry(vl *node.VList, carry map[int]node.H) {
	if len(carry) == 0 {
		return
	}
	for n := vl.List; n != nil; n = n.Next() {
		child, ok := n.(*node.VList)
		if !ok || child.Attributes == nil {
			continue
		}
		idx, ok := child.Attributes["_groupItemIdx"].(int)
		if !ok {
			continue
		}
		if c, ok := carry[idx]; ok {
			for k, v := range c {
				child.Attributes[k] = v
			}
		}
	}
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
//
// Return value: (-1, nil, err) when the whole vlist has been placed. When an
// automatic page break switches to a page with a different content width, it
// stops at the next whole body item (identified by the _groupItemIdx stamp)
// and returns that item's index plus the attributes to carry onto the rebuilt
// nodes — OutputPagesFromText then re-breaks the remaining items at the new
// width. Nodes that do not correspond to a whole item (fragment lines from an
// unwrapped paragraph, spliced table rows) are placed at the old width.
func (cb *CSSBuilder) outputGroupNodes(vl *node.VList, fc *flowCursor) (int, map[int]node.H, error) {
	// builtWidth is the content width the vlist was formatted at. Pages
	// whose @page rule yields the same width slice this vlist by height
	// only (fast path); a differing width triggers the item-level restart.
	builtWidth := fc.cur.width

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
		// Do not descend into a table: a table that does not fit breaks
		// in outputTableRows, which needs the table VList intact for the
		// repeated header, rows that break inside or that a rowspan
		// joins, and the width reflow. Unwrapped, its bare rows would be
		// placed one by one.
		if inner.Attributes != nil {
			if o, _ := inner.Attributes["origin"].(string); o == "table" {
				break
			}
			// A box with a border or background splits in outputBlockSplit,
			// which needs its _splittable VList; unwrapped, the HTMLBorder
			// parts would be placed whole (#35). Transparent boxes unwrap.
			if hv, ok := inner.Attributes["_splittableHv"].(HTMLValues); ok && (hv.hasBorder() || hv.BackgroundColor != nil) {
				break
			}
			// The page and position of a heading, an anchor and an element
			// id are taken from the box they are stamped on, so a box that
			// fits is kept. A taller one must split: its marks move onto its
			// first child.
			if hasBoxMarks(inner) && vlistNodeHeight(inner) <= fc.cur.height {
				break
			}
			// Unwrapped, a paragraph's lines would be placed one by one,
			// without widows at its breaks and without the re-break of its
			// rest in a region of another width (#84).
			if _, leaf := inner.Attributes["_splittableTe"]; leaf {
				break
			}
			// A pre-rendered box is placed whole.
			if isPlaceholderBox(inner) {
				break
			}
			// A box that repeats its decorations on every fragment splits in
			// outputBlockSplit.
			if _, _, clone := decorationClone(inner); clone || cb.fragmentTrim(inner) != (breakTrim{}) {
				break
			}
		}
		shiftChildren(inner)
		propagateInsertsAttr(inner, inner.List)
		propagateAnchorIndices(inner, inner.List)
		carryBoxMarks(inner, inner.List)
		propagateFlowChild(inner, inner.List)
		contentList = inner.List
		if inner.Width > 0 {
			contentWidth = inner.Width
		}
	}

	cur := contentList

	// chained holds the blocks after the head of the break-after: avoid
	// chain being placed.
	chained := map[node.Node]bool{}

	// floatRegion is the region the last float box of this chain was
	// buffered for, 0 before one. A sibling built beside that float
	// (attrInFloatBand) that ends up in a later region is beside nothing
	// there and is rebuilt at full width; the rebuilt chain starts after the
	// float, so it carries no band.
	floatRegion := 0

	// restartIdx reports whether pagination must hand control back to
	// OutputPagesFromText, n being the (not yet placed) node of a whole body
	// item: the current page's content width differs from the width the
	// vlist was built at, the item narrows text beside an inside/outside
	// float that was resolved for a page of the other parity, or the item
	// was set beside a float that stayed on an earlier page. Margin kerns
	// in between are placed normally: they are width-independent and the
	// rebuilt chain drops its leading kern.
	restartIdx := func(n node.Node) (int, bool) {
		nvl, ok := n.(*node.VList)
		if !ok || nvl.Attributes == nil {
			return 0, false
		}
		idx, ok := nvl.Attributes["_groupItemIdx"].(int)
		if !ok {
			return 0, false
		}
		// The first item of a rebuild was built for this very region. Should
		// it still not fit the checks below, it is placed as it is: another
		// rebuild would come out the same and never end.
		if idx == 0 && fc.rebuiltIn == fc.serial {
			return 0, false
		}
		if fc.cur.width != builtWidth {
			return idx, true
		}
		if built, ok := nvl.Attributes["_floatParity"].(bool); ok && built != fc.cur.isRight() {
			return idx, true
		}
		if inFloatBand(n) && floatRegion != 0 && floatRegion != fc.serial {
			return idx, true
		}
		return 0, false
	}

	// collectReflowCarry gathers the attributes listed in reflowCarryKeys
	// from every stamped item node in the not-yet-placed rest of the chain.
	collectReflowCarry := func(from node.Node) map[int]node.H {
		carry := map[int]node.H{}
		for n := from; n != nil; n = n.Next() {
			nvl, ok := n.(*node.VList)
			if !ok || nvl.Attributes == nil {
				continue
			}
			idx, ok := nvl.Attributes["_groupItemIdx"].(int)
			if !ok {
				continue
			}
			c := node.H{}
			for _, key := range reflowCarryKeys {
				if v, ok := nvl.Attributes[key]; ok {
					c[key] = v
				}
			}
			if len(c) > 0 {
				carry[idx] = c
			}
		}
		return carry
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
		// A page break in a previous iteration (or inside a split path)
		// switched to a different content width: restart at the next whole
		// item so OutputPagesFromText re-breaks it at the new width.
		if idx, ok := restartIdx(cur); ok {
			return idx, collectReflowCarry(cur), nil
		}
		if fc.truncated(cb, cur) {
			cur = cur.Next()
			continue
		}
		cur = fc.marginBefore(cb, cur)

		// A forced break-before on a block, or on the first block inside
		// it, starts a new region unless nothing is placed in this one yet.
		// The blocks of the body were split into groups at their own; this
		// catches the ones further down.
		if brk := fc.breakBefore(cur); brk != "" && !cb.marginsOnly() {
			if err := fc.breakTo(brk, cur); err != nil {
				return -1, nil, err
			}
			if idx, ok := restartIdx(cur); ok {
				return idx, collectReflowCarry(cur), nil
			}
		}

		next := cur.Next()
		h := vlistNodeHeight(cur)
		contentArea := fc.cur.height

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
			if o != "table" && !spl && !isPlaceholderBox(wrap) && wrap.List != nil && hasTableChild(wrap.List) {
				propagateInsertsAttr(wrap, wrap.List)
				propagateFlowChild(wrap, wrap.List)
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
		// A table without a header takes this path too: only
		// outputTableRows splits rows, keeps the rows a rowspan joins
		// together and sets the rest of the table again at the width of a
		// wider or narrower page.
		if tableVL, ok := cur.(*node.VList); ok && tableVL.Attributes != nil {
			buildHeadersFn, tok := tableVL.Attributes["_buildHeaders"]
			if !tok {
				o, _ := tableVL.Attributes["origin"].(string)
				tok = o == "table"
			}
			if tok {
				tableIncoming := fc.insertsOn(cur)
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
					phc := fc.holdsContent(cb) || topFloatH > 0
					if err := cb.flushInsertsIn(fc.cur); err != nil {
						return -1, nil, err
					}
					yLocal := fc.cur.top - topFloatH - flushedBodyH
					yLimitLocal := fc.cur.bottom()
					if err := cb.outputTableRows(tableVL, buildHeadersFn, &yLocal, &yLimitLocal, &phc, fc); err != nil {
						return -1, nil, err
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
						usedH := fc.cur.top - yLocal
						if usedH > 0 {
							k := node.NewKern()
							k.Kern = usedH
							spacer := node.Vpack(k)
							spacer.Attributes = node.H{"origin": "table continuation spacer"}
							cb.bufferBody(spacer, usedH)
						}
					}
					cur = next
					continue
				}
			}
		}

		incoming := fc.insertsOn(cur)

		// The overhang of a float that overflows the page ends at the page
		// break rather than taking a page of its own.
		if _, ok := floatOverhang(cur); ok {
			cb.endOverhangAt([]node.Node{cur}, contentArea-trialPageHeight(incoming, 0))
			h = vlistNodeHeight(cur)
		}

		// With -bag-text-box-trim-at-break, a block whose last line fits by
		// its text fits here, but is trimmed only once it stays: a
		// break-after: avoid chain can still move it on, to stand mid-page.
		fit := h
		if t := blockTrimAtBreak(cur); t > 0 && trialPageHeight(incoming, h-t) <= contentArea {
			fit = h - t
		}

		// Splittable block (<pre>, block container with bg/border) that's
		// taller than what fits even on an empty page: fragment it across
		// pages instead of letting the wrapped vlist run off the bottom.
		// Short splittable blocks fall through to the normal pageBuf path,
		// and so does a block with break-inside: avoid that fits on an
		// empty page: it moves on whole. A forced break inside it is taken
		// all the same.
		if vlS, ok := cur.(*node.VList); ok && vlS.Attributes != nil {
			if isSplittable, _ := vlS.Attributes["_splittable"].(bool); isSplittable {
				keepWhole := avoidBreakInside(vlS) && h <= contentArea
				if (trialPageHeight(incoming, fit) > contentArea && !keepWhole) || forcedInside(splitChildren(vlS), fc.forcedKeyword) {
					// Commit incoming inserts so outputBlockSplit's
					// availOnPage sees the correct float/footnote
					// reservations. Don't ship pageBuf here — the splitter
					// appends its first fragment after whatever's already
					// buffered (e.g. a heading just placed via the
					// avoidBreakAfter relaxation), and only breaks
					// between fragments.
					if len(incoming) > 0 {
						for _, ins := range incoming {
							cb.pageInserts[ins.Class] = append(cb.pageInserts[ins.Class], ins)
						}
						cb.pageInsertHeight[InsertFloatTop] = cb.totalFloatTopHeight(cb.pageInserts[InsertFloatTop])
						cb.pageInsertHeight[InsertFloatBottom] = cb.totalFloatBottomHeight(cb.pageInserts[InsertFloatBottom])
						cb.pageInsertHeight[InsertFootnote] = cb.totalFootnoteHeight(cb.pageInserts[InsertFootnote])
					}
					if err := cb.outputBlockSplit(vlS, fc); err != nil {
						return -1, nil, err
					}
					if brk := fc.breakAfter(cur); brk != "" && next != nil {
						if err := fc.breakTo(brk, next); err != nil {
							return -1, nil, err
						}
					}
					cur = next
					continue
				}
			}
		}

		// A float box reserves no height of its own, so it would always fit
		// where the block beside it does not, and a page break would leave
		// it alone at the bottom of the page. It stays with that block: the
		// page has to hold the float's painted extent and a foothold of the
		// block beside it, or both move to the next page.
		if fh, isFloat := floatBoxHeight(cur); isFloat && next != nil {
			if need := floatKeepWithNext(fh, siblingsFrom(next)); trialPageHeight(incoming, need) > contentArea && fc.holdsContent(cb) {
				if err := fc.moveOn(cb, cur); err != nil {
					return -1, nil, err
				}
				if idx, ok := restartIdx(cur); ok {
					return idx, collectReflowCarry(cur), nil
				}
			}
		}

		// A run of break-after: avoid blocks is weighed once, at its first
		// block: the chain and the foothold of the block after it go to the
		// next page together unless they fit here. The chain's later blocks
		// are not weighed again, or one that does not fit an empty page
		// would break after the head it has just been moved with.
		if chained[cur] {
			delete(chained, cur)
		} else if avoidBreakAfter(cur) && next != nil {
			need, rest := avoidChainHeight(cur)
			for _, n := range rest {
				chained[n] = true
			}
			if trialPageHeight(incoming, need) > contentArea && fc.holdsContent(cb) {
				if err := fc.moveOn(cb, cur); err != nil {
					return -1, nil, err
				}
				// The fresh page may use a different content width; if cur
				// is a whole item, re-break it (and everything after) there
				// instead of placing old-width lines.
				if idx, ok := restartIdx(cur); ok {
					return idx, collectReflowCarry(cur), nil
				}
			}
		}

		// The space a CSS height adds below a block's content fills the
		// page, and its rest goes on at the top of the next one, as the
		// content it stands for would.
		if k, ok := cssHeight(cur); ok {
			if room := contentArea - trialPageHeight(incoming, 0); k.Kern > room {
				if room > 0 {
					box := node.NewVList()
					box.List = cssHeightKern(room)
					box.Width = contentWidth
					box.Height = room
					cb.bufferBody(box, room)
					k.Kern -= room
				}
				if err := fc.breakTo("", cur); err != nil {
					return -1, nil, err
				}
				continue
			}
		}

		if trialPageHeight(incoming, fit) > contentArea && fc.holdsContent(cb) {
			if err := fc.moveOn(cb, cur); err != nil {
				return -1, nil, err
			}
			// Same width check as above: cur has not been buffered yet, so
			// a whole item can still be re-broken at the new width.
			if idx, ok := restartIdx(cur); ok {
				return idx, collectReflowCarry(cur), nil
			}
		}

		// A break above may have left cur, a margin, at the top of a region.
		if fc.truncated(cb, cur) {
			cur = next
			continue
		}
		// In an occupied one, the margin is kept: the top of the loop
		// collapses it with MarginBefore and weighs cur again.
		if fc.top == topKept && fc.regionEmpty(cb) {
			continue
		}

		// cur stays here: trim it if it fits only by its text.
		if t := blockTrimAtBreak(cur); t > 0 && trialPageHeight(incoming, h) > contentArea && trialPageHeight(incoming, h-t) <= contentArea {
			trimBlockAtBreak(cur)
			h = vlistNodeHeight(cur)
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

		cb.bufferBody(box, h)
		if _, isFloat := floatBoxHeight(cur); isFloat {
			floatRegion = fc.serial
		}

		if brk := fc.breakAfter(cur); brk != "" && next != nil {
			if err := fc.breakTo(brk, next); err != nil {
				return -1, nil, err
			}
		}

		cur = next
	}

	return -1, nil, nil
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
// surrounding paragraphs in the page buffer; the next region is taken between
// fragments to ship the partial page.
func (cb *CSSBuilder) outputBlockSplit(blockVL *node.VList, fc *flowCursor) error {
	children, _ := blockVL.Attributes["_splittableInner"].([]node.Node)
	hv, _ := blockVL.Attributes["_splittableHv"].(HTMLValues)
	innerWidth, _ := blockVL.Attributes["_splittableInnerWidth"].(bag.ScaledPoint)

	if len(children) == 0 {
		return nil
	}

	// Width-change reflow state. Leaf splittables (a paragraph or <pre>
	// whose children are line HLists) carry their source Text and its
	// formatting width; when a break between fragments switches to a page
	// with a different content width, the not-yet-placed lines are
	// re-broken at the new width via FormatParagraphTail. Box-container
	// splittables (a bordered card holding block children) are not stamped
	// and keep their built width.
	splitTe, _ := blockVL.Attributes["_splittableTe"].(*frontend.Text)
	teWidth, _ := blockVL.Attributes["_splittableTeWidth"].(bag.ScaledPoint)
	curContentWidth := fc.cur.width
	// A block container carries its source Text and offered width instead:
	// children a page break parts from the float they were set beside are
	// rebuilt from the items (see rebuildContainerRemainder).
	containerTe, _ := blockVL.Attributes["_splittableContainerTe"].(*frontend.Text)
	containerWd, _ := blockVL.Attributes["_splittableContainerWd"].(bag.ScaledPoint)
	// The band a float imposed on a leaf paragraph's first lines. The lines
	// are placed as built while they stay on the float's page; the ones
	// pushed to the next page are beside nothing and are re-broken at full
	// width, with the band replayed for the lines before them.
	bandIndent, hasBand := blockVL.Attributes[attrFloatBandIndent].(floatBandIndent)
	bandRows := 0
	var bandSettings frontend.TypesettingSettings
	if hasBand {
		bandRows = bandIndent.rows
		bandSettings = bandIndent.settings()
	}

	// history records one step per successful rebuild so a later rebuild
	// (e.g. alternating :left/:right widths) can reproduce every previous
	// break to locate the remaining text.
	var history []frontend.ParagraphTailStep
	// placedLines counts the line HLists buffered from the current children
	// set; it becomes the Lines value of the next history step.
	placedLines := 0

	countLines := func(items []node.Node) int {
		n := 0
		for _, c := range items {
			if _, ok := c.(*node.HList); ok {
				n++
			}
		}
		return n
	}

	// reflowRemainder re-breaks the not-yet-placed rest (children[i:]) at
	// the current page's content width. Returns the new children slice, or
	// nil when reflow is not possible (no source Text, degenerate paragraph,
	// reproduction failed) — the caller then keeps the old-width lines.
	reflowRemainder := func(i int, force bool) []node.Node {
		if splitTe == nil || (fc.cur.width == curContentWidth && !force) {
			return nil
		}
		newTeWidth := teWidth + (fc.cur.width - curContentWidth)
		if newTeWidth <= 0 {
			return nil
		}
		step := frontend.ParagraphTailStep{Width: teWidth, Lines: placedLines}
		if len(history) == 0 {
			// The first pass is the one the band narrowed; every later
			// pass is a tail set without it.
			step.Settings = bandSettings
		}
		steps := append(append([]frontend.ParagraphTailStep{}, history...), step)
		// Strip the htmlbag-private sentinels around the frontend call:
		// they would hit the strict unknown-setting default in Mknodes.
		pbi, hasPBI := splitTe.Settings[settingPageBreakInside]
		delete(splitTe.Settings, settingPageBreakInside)
		ch, hasCH := splitTe.Settings[settingCSSHeight]
		delete(splitTe.Settings, settingCSSHeight)
		bm, hasBM := splitTe.Settings[settingBookmark]
		delete(splitTe.Settings, settingBookmark)
		lt, hasLT := splitTe.Settings[settingLangTag]
		delete(splitTe.Settings, settingLangTag)
		restoreTrace := captureTraceSettings(splitTe)
		tailVL, err := cb.frontend.FormatParagraphTail(splitTe, steps, newTeWidth)
		restoreTrace()
		if err == nil && tailVL != nil {
			// The block's first line was placed already.
			trimLines(tailVL, textBoxTrim{end: cb.trims[splitTe].end})
		}
		if hasPBI {
			splitTe.Settings[settingPageBreakInside] = pbi
		}
		if hasCH {
			splitTe.Settings[settingCSSHeight] = ch
		}
		if hasBM {
			splitTe.Settings[settingBookmark] = bm
		}
		if hasLT {
			splitTe.Settings[settingLangTag] = lt
		}
		if err != nil || tailVL == nil {
			bag.Logger.Debug("width reflow of splittable block failed, keeping built width", "error", err)
			return nil
		}

		var newChildren []node.Node
		if i == 0 && placedLines == 0 {
			// Nothing placed yet: keep unconsumed leading kerns (padding-top).
			for _, c := range children {
				if k, ok := c.(*node.Kern); ok {
					newChildren = append(newChildren, k)
					continue
				}
				break
			}
		}
		lead := len(newChildren)
		for n := tailVL.List; n != nil; n = n.Next() {
			newChildren = append(newChildren, n)
		}
		// Keep unplaced trailing kerns (padding-bottom); the lineskip glues
		// belong to the old break and are replaced by the tail vlist's own.
		var trailing []node.Node
		for j := len(children) - 1; j >= i && j >= lead; j-- {
			k, ok := children[j].(*node.Kern)
			if !ok {
				break
			}
			trailing = append([]node.Node{k}, trailing...)
		}
		newChildren = append(newChildren, trailing...)
		for _, c := range newChildren {
			c.SetPrev(nil)
			c.SetNext(nil)
		}

		history = steps
		teWidth = newTeWidth
		innerWidth = newTeWidth
		curContentWidth = fc.cur.width
		placedLines = 0
		bandRows = 0
		return newChildren
	}

	// rebuildContainerRemainder builds the not-yet-placed children of a block
	// container afresh from their items. The rebuilt chain starts after the
	// float whose band narrowed them, so they come back at full width, as
	// they have to be on a page the float is not on. Returns nil when the
	// container carries no source or the rebuild fails; the caller then
	// keeps the built children.
	rebuildContainerRemainder := func(i int) []node.Node {
		if containerTe == nil {
			return nil
		}
		itemIdx := -1
		for _, c := range children[i:] {
			if vl, ok := c.(*node.VList); ok && vl.Attributes != nil {
				if idx, ok := vl.Attributes[containerItemIdxKey].(int); ok {
					itemIdx = idx
					break
				}
			}
		}
		if itemIdx < 0 {
			return nil
		}
		// Headings, anchors and inserts were recorded on the first build
		// and are not re-created by a rebuild (see reflowCarryKeys).
		carry := map[int]node.H{}
		for _, c := range children[i:] {
			vl, ok := c.(*node.VList)
			if !ok || vl.Attributes == nil {
				continue
			}
			idx, ok := vl.Attributes[containerItemIdxKey].(int)
			if !ok {
				continue
			}
			attrs := node.H{}
			for _, key := range reflowCarryKeys {
				if v, ok := vl.Attributes[key]; ok {
					attrs[key] = v
				}
			}
			if len(attrs) > 0 {
				carry[idx-itemIdx] = attrs
			}
		}
		// The container's own settings, less what describes the whole
		// box rather than its content: a declared height was already
		// spent on the first fragment.
		settings := make(frontend.TypesettingSettings, len(containerTe.Settings))
		for k, v := range containerTe.Settings {
			settings[k] = v
		}
		delete(settings, settingCSSHeight)
		delete(settings, settingBookmark)
		wrapper := &frontend.Text{Settings: settings, Items: containerTe.Items[itemIdx:]}
		if cb.clones[containerTe] {
			cb.clones[wrapper] = true
		}
		cb.reflowRebuild = true
		vl, err := cb.CreateVlist(wrapper, containerWd)
		cb.reflowRebuild = false
		if err != nil || vl == nil {
			bag.Logger.Debug("rebuild of split container failed, keeping built children", "error", err)
			return nil
		}
		var rebuilt []node.Node
		if snap, ok := vl.Attributes["_splittableInner"].([]node.Node); ok && len(snap) > 0 {
			rebuilt = snap
		} else {
			for n := vl.List; n != nil; n = n.Next() {
				rebuilt = append(rebuilt, n)
			}
		}
		// The collapsed margin before the first rebuilt child was placed
		// with the previous fragment.
		if len(rebuilt) > 0 {
			if k, ok := rebuilt[0].(*node.Kern); ok && k.Attributes != nil {
				if o, _ := k.Attributes["origin"].(string); o == "margin" {
					rebuilt = rebuilt[1:]
				}
			}
		}
		for _, c := range rebuilt {
			c.SetPrev(nil)
			c.SetNext(nil)
			vl, ok := c.(*node.VList)
			if !ok || vl.Attributes == nil {
				continue
			}
			if idx, ok := vl.Attributes[containerItemIdxKey].(int); ok {
				for k, v := range carry[idx] {
					vl.Attributes[k] = v
				}
			}
		}
		return rebuilt
	}

	// rebuildRemainder is called right after a page break between fragments,
	// with i the first child of the next fragment. It returns new children
	// when the rest has to be rebuilt: the page has another content width,
	// or the rest was set beside a float that stayed on the previous page.
	rebuildRemainder := func(i int) []node.Node {
		if containerTe != nil {
			for _, c := range children[i:] {
				if !isContentNode(c) {
					continue
				}
				if inFloatBand(c) {
					return rebuildContainerRemainder(i)
				}
				break
			}
			return nil
		}
		return reflowRemainder(i, bandRows > 0 && placedLines < bandRows)
	}

	// Detach so children can be re-linked into per-fragment vlists.
	for _, c := range children {
		c.SetPrev(nil)
		c.SetNext(nil)
	}

	availOnPage := func() bag.ScaledPoint {
		contentArea := fc.cur.height
		used := cb.pageBufHeight +
			cb.pageInsertHeight[InsertFloatTop] +
			cb.pageInsertHeight[InsertFloatBottom] +
			cb.pageInsertHeight[InsertFootnote]
		return contentArea - used
	}

	fl := fragLinesOf(blockVL)
	trim := cb.fragmentTrim(blockVL)
	i := 0
	isFirst := true
	for i < len(children) {
		avail := availOnPage()
		if trim.start && !isFirst {
			trimFragmentStart(children[i:])
		}

		// Try to fit all remaining children with bottom-fragment overhead.
		topOverhead := bag.ScaledPoint(0)
		if isFirst {
			topOverhead = hv.PaddingTop + hv.BorderTopWidth
		}
		bottomOverhead := hv.PaddingBottom + hv.BorderBottomWidth
		// Under clone, every fragment has both sides.
		var cutOverhead bag.ScaledPoint
		if top, bottom, ok := decorationClone(blockVL); ok {
			topOverhead, bottomOverhead, cutOverhead = top, bottom, bottom
		}
		remaining := childrenHeight(children[i:])

		if topOverhead+remaining+bottomOverhead <= avail && !forcedInside(children[i:], fc.forcedKeyword) {
			kind := fragBottom
			if isFirst {
				kind = fragOnly
			}
			wrapped, h := cb.buildFragment(blockVL, children[i:], kind, innerWidth)
			stampFragment(wrapped, blockVL)
			if isFirst {
				carryMarks(blockVL, wrapped)
			}
			cb.bufferBody(wrapped, h)
			return nil
		}

		// Doesn't all fit: collect a top/middle fragment that does fit.
		batchStart := i
		var batch []node.Node
		var overflow bool
		var inner *splitPlan
		var brk string
		batch, i, overflow, inner, brk = cb.fitChildren(children, i, avail-topOverhead-cutOverhead, fc.forcedKeyword, trim)
		// A paragraph keeps orphans and widows lines (CSS Fragmentation 3
		// §4.4); a container cuts between its blocks, or through one, as
		// long as something goes before the cut and the cut does not fall
		// between two blocks a break-after: avoid keeps together.
		short := countContent(batch) < fl.orphans && i < len(children) && !inner.heightCut()
		if splitTe == nil {
			if inner == nil && brk == "" && i < len(children) {
				if b, j := pullBackForAvoid(children, batch, i); countContent(b) > 0 || fc.holdsContent(cb) {
					batch, i = b, j
				}
			}
			short = countContent(batch) == 0 && inner == nil && i < len(children)
		}

		// The rest of a split block that reaches an occupied region weighs
		// it as a block that starts there does: it moves on once when its
		// first child does not fit or it would leave fewer than orphans.
		if !isFirst && fc.holdsContent(cb) && (short || overflow) {
			if err := fc.moveOn(cb, blockVL); err != nil {
				return err
			}
			i = batchStart
			if nc := rebuildRemainder(i); nc != nil {
				children = nc
			}
			continue
		}

		// Orphan protection: if the first fragment of the block would leave
		// fewer than `orphans` on the current page, or its first line does
		// not fit at all, break first so
		// the block restarts on a fresh page with full available space. Only
		// applies when there's something already on the page — on an empty
		// page even a single line has to land here.
		if isFirst && fc.holdsContent(cb) && (short || overflow) {
			if err := fc.moveOn(cb, blockVL); err != nil {
				return err
			}
			i = 0
			// The block restarts on the fresh page; if that page has a
			// different content width, or the block was set beside a
			// float that stays behind, re-break it there from the start.
			if nc := rebuildRemainder(0); nc != nil {
				children = nc
			}
			continue
		}

		if splitTe != nil && i < len(children) && !inner.heightCut() {
			var remainingLines int
			batch, i, remainingLines = pullBackForWidows(children, batch, i, fl)
			// Both cannot be kept, so there is no break inside the block
			// here: it moves on whole, as it would between blocks, unless
			// the page holds nothing else.
			if splitTe != nil && remainingLines < fl.widows && isFirst && fc.holdsContent(cb) {
				if err := fc.moveOn(cb, blockVL); err != nil {
					return err
				}
				i = 0
				if nc := rebuildRemainder(0); nc != nil {
					children = nc
				}
				continue
			}
		}
		if inner == nil {
			batch, i = keepFloatWithChild(children, batch, i)
		} else {
			// The cut runs through children[i]: its part before the cut
			// ends this fragment, its rest takes its place.
			head, rest := cb.cutBlock(inner)
			batch = append(batch, head)
			children = append([]node.Node(nil), children...)
			children[i] = rest
		}

		kind := fragTop
		if !isFirst {
			kind = fragMiddle
		}
		room := avail - topOverhead - cutOverhead
		// A batch that takes the last child is the block's last fragment and
		// closes it, even when its bottom padding and border overrun the
		// room (a float overhang ending at the page break fills it).
		if i == len(children) {
			kind = fragBottom
			if isFirst {
				kind = fragOnly
			}
			room = avail - topOverhead - bottomOverhead
		}
		// Under clone every fragment ends trimmed. With
		// -bag-text-box-trim-at-break only one before an unforced break is,
		// which can be the block's last when it fits only by its text.
		if trim.end && (trim.clone || brk == "") {
			if i < len(children) || !trim.clone && bottomOverhead == 0 && childrenHeight(batch) > room {
				trimFragmentEnd(batch, trim.clone)
			}
		}
		cb.endOverhangAt(batch, room)
		wrapped, h := cb.buildFragment(blockVL, batch, kind, innerWidth)
		stampFragment(wrapped, blockVL)
		if isFirst {
			carryMarks(blockVL, wrapped)
		}
		cb.bufferBody(wrapped, h)
		isFirst = false
		placedLines += countLines(batch)

		// More fragments to come: ship this page and start fresh.
		if i < len(children) {
			if err := fc.breakTo(brk, blockVL); err != nil {
				return err
			}
			// The fresh page may use a different content width (@page
			// :first vs. @page), or the rest was set beside a float that
			// stays on the page just shipped: rebuild it instead of
			// placing lines built for another situation.
			if nc := rebuildRemainder(i); nc != nil {
				children = nc
				i = 0
			}
			for i < len(children) && fc.truncated(cb, children[i]) {
				i++
			}
		}
	}
	return nil
}

// The kinds of fragment a split block is cut into (CSS Fragmentation 3,
// box-decoration-break: slice):
//   - fragTop keeps padding-top and border-top and drops the bottom side
//   - fragMiddle drops both
//   - fragBottom drops the top side and keeps padding-bottom and
//     border-bottom
//   - fragOnly is the whole block, both sides kept
const (
	fragTop = iota
	fragMiddle
	fragBottom
	fragOnly
)

// childrenHeight is the vertical extent of items stacked.
func childrenHeight(items []node.Node) bag.ScaledPoint {
	var s bag.ScaledPoint
	for _, n := range items {
		s += vlistNodeHeight(n)
	}
	return s
}

// countContent counts the content children of items. A splittable block has
// two shapes: line-level children (a <pre> is HList lines interleaved with
// Glue) and block-level children (a bordered card is VList paragraphs/divs
// interleaved with margin Kerns). Both an HList and a VList count as one
// unit of content here; only the Glue/Kern fillers between them are skipped.
// A float box is neither: it paints beside the content.
func countContent(items []node.Node) int {
	n := 0
	for _, c := range items {
		if isContentNode(c) {
			n++
		}
	}
	return n
}

// fitChildren collects the children from i on that fit in room into a batch
// and returns it with the index of the first child left out. When that child
// splits so that its first part fits in the room left, inner plans the cut
// through it, and the batch ends before it. Otherwise the first child goes
// in even when it does not fit, which only an empty page may take; overflow
// reports that. A forced break between two blocks, or inside a block, ends
// the batch there; brk is its keyword, as forced reports it.
func (cb *CSSBuilder) fitChildren(children []node.Node, i int, room bag.ScaledPoint, forced func(any) string, trim breakTrim) (batch []node.Node, next int, overflow bool, inner *splitPlan, brk string) {
	if trim.end && trim.clone {
		defer func() {
			if inner == nil && brk == "" && next < len(children) {
				batch, next = pullBackGrownLine(children, batch, next, room)
			}
		}()
	}
	var batchH bag.ScaledPoint
	var last node.Node
	for ; i < len(children); i++ {
		if last != nil && isContentNode(children[i]) {
			if k := cmp.Or(forcedAfter(last, forced), forcedBefore(children[i], forced)); k != "" {
				return batch, i, false, nil, k
			}
		}
		ch := vlistNodeHeight(children[i])
		// A float box has no height of its own; what has to fit is its
		// painted extent together with the child beside it, or the float is
		// parted from that child by the page break.
		if fh, isFloat := floatBoxHeight(children[i]); isFloat {
			ch = floatKeepWithNext(fh, children[i+1:])
		}
		// The overhang of a float that overflows the page ends at the page
		// break; the caller shortens it once the batch is placed.
		if _, ok := floatOverhang(children[i]); ok && batchH+ch > room {
			ch = max(room-batchH, 0)
		}
		if batchH+ch > room {
			// A line whose end is trimmed at the break fits by its text, and
			// the fragment ends after it.
			if t := lineTrimEnd(children[i]); trim.end && t > 0 && batchH+ch-t <= room {
				return append(batch, children[i]), i + 1, false, nil, ""
			}
			if p := cb.planSplit(children[i], room-batchH, forced); p != nil {
				return batch, i, false, p, p.brk
			}
			if len(batch) > 0 {
				break
			}
			overflow = true
		} else if vl, ok := children[i].(*node.VList); ok && forcedInside(splitChildren(vl), forced) {
			if p := cb.planSplit(vl, room-batchH, forced); p != nil && p.brk != "" {
				return batch, i, false, p, p.brk
			}
		}
		batch = append(batch, children[i])
		if _, ok := floatOverhang(children[i]); ok {
			batchH += ch
		} else {
			batchH += vlistNodeHeight(children[i])
		}
		if isContentNode(children[i]) {
			last = children[i]
		}
	}
	if len(batch) == 0 {
		// One child is taller than a full empty page. Place it anyway —
		// truncation is unavoidable. Advance so the loop terminates.
		batch = append(batch, children[i])
		i++
	}
	return batch, i, overflow, nil, ""
}

// splitChildren is the children a split of vl cuts between, nil when vl
// does not split.
func splitChildren(vl *node.VList) []node.Node {
	if vl.Attributes == nil {
		return nil
	}
	if spl, _ := vl.Attributes["_splittable"].(bool); !spl {
		return nil
	}
	children, _ := vl.Attributes["_splittableInner"].([]node.Node)
	return children
}

// forcedBefore is the forced break-before keyword of n, as forced reports it,
// or of its first block, which passes it on to n (CSS Fragmentation 3 §3.1),
// or "".
func forcedBefore(n node.Node, forced func(any) string) string {
	for depth := 0; depth < 32 && n != nil; depth++ {
		vl, ok := n.(*node.VList)
		if !ok || vl.Attributes == nil {
			return ""
		}
		if k := forced(vl.Attributes["pageBreakBefore"]); k != "" {
			return k
		}
		n = nil
		for _, c := range splitChildren(vl) {
			if isContentNode(c) {
				n = c
				break
			}
		}
	}
	return ""
}

// forcedAfter is the forced break-after keyword of n or of its last block, as
// forcedBefore is for break-before.
func forcedAfter(n node.Node, forced func(any) string) string {
	for depth := 0; depth < 32 && n != nil; depth++ {
		vl, ok := n.(*node.VList)
		if !ok || vl.Attributes == nil {
			return ""
		}
		if k := forced(vl.Attributes["pageBreakAfter"]); k != "" {
			return k
		}
		n = nil
		children := splitChildren(vl)
		for j := len(children) - 1; j >= 0; j-- {
			if isContentNode(children[j]) {
				n = children[j]
				break
			}
		}
	}
	return ""
}

// forcedInside reports whether there is a forced break between two of the
// blocks in items or inside one of them, at any depth.
func forcedInside(items []node.Node, forced func(any) string) bool {
	var last node.Node
	for _, c := range items {
		if !isContentNode(c) {
			continue
		}
		if last != nil && cmp.Or(forcedAfter(last, forced), forcedBefore(c, forced)) != "" {
			return true
		}
		if vl, ok := c.(*node.VList); ok && forcedInside(splitChildren(vl), forced) {
			return true
		}
		last = c
	}
	return false
}

// pullBackForAvoid moves the cut before children[i] up while it falls between
// two blocks that a break-after: avoid on the first or a break-before: avoid
// on the second keeps together (CSS Fragmentation 3 §3.3). The batch it
// returns may hold no block; the caller decides what that means.
func pullBackForAvoid(children, batch []node.Node, i int) ([]node.Node, int) {
	for i < len(children) && len(batch) > 0 {
		var after node.Node
		for _, c := range children[i:] {
			if isContentNode(c) {
				after = c
				break
			}
		}
		j := len(batch) - 1
		for j >= 0 && !isContentNode(batch[j]) {
			j--
		}
		if j < 0 || !avoidBreakAfter(batch[j]) && !avoidBreakBefore(after) {
			break
		}
		i -= len(batch) - j
		batch = batch[:j]
	}
	return batch, i
}

// avoidBreakBefore reports whether n has break-before: avoid.
func avoidBreakBefore(n node.Node) bool {
	vl, ok := n.(*node.VList)
	return ok && vl.Attributes != nil && vl.Attributes["pageBreakBefore"] == "avoid"
}

// splitPlan is a cut through a splittable block that a block being split
// cannot place whole: the first n of its children go into the fragment before
// the cut, and with inner set, the cut runs through children[n], which inner
// cuts in turn. It is worked out on heights alone and only built by cutBlock
// once the cut is certain, as building links the children into the
// fragments.
type splitPlan struct {
	vl       *node.VList
	children []node.Node
	n        int
	inner    *splitPlan
	// brk is the keyword of the forced break the cut is made at, "" when
	// the room runs out there.
	brk string
	// room is the height the part before the cut is placed in.
	room bag.ScaledPoint
	// height is set when the cut falls in the space a CSS height adds
	// below a block's content (cssHeight), which is cut at room.
	height *node.Kern
}

// heightCut reports whether p cuts the space a CSS height adds below the
// content. No line or block follows the cut, so orphans and widows, which
// keep lines together, do not apply.
func (p *splitPlan) heightCut() bool {
	return p != nil && p.height != nil
}

// attrSplitRest marks the rest of a block cut by cutBlock: its first
// fragment has no top padding or border.
const attrSplitRest = "_splitRest"

// planSplit plans a cut through n, a child of a block being split, so that
// its part before the cut fits in room. It returns nil when n does not split
// or no part of it fits: not its first line or block, fewer lines than
// orphans, a rest of fewer lines than widows, or blocks that a break-after:
// avoid keeps with the rest.
func (cb *CSSBuilder) planSplit(n node.Node, room bag.ScaledPoint, forced func(any) string) *splitPlan {
	if k, ok := cssHeight(n); ok {
		if room <= 0 || room >= k.Kern {
			return nil
		}
		return &splitPlan{height: k, room: room}
	}
	vl, ok := n.(*node.VList)
	if !ok || vl.Attributes == nil {
		return nil
	}
	if spl, _ := vl.Attributes["_splittable"].(bool); !spl {
		return nil
	}
	// A float is placed whole; one taller than the page overflows it.
	if _, isFloat := floatBoxHeight(vl); isFloat {
		return nil
	}
	if pbi, _ := vl.Attributes["pageBreakInside"].(string); pbi == "avoid" {
		return nil
	}
	children, _ := vl.Attributes["_splittableInner"].([]node.Node)
	if len(children) == 0 {
		return nil
	}
	if top, bottom, clone := decorationClone(vl); clone {
		room -= top + bottom
	} else if rest, _ := vl.Attributes[attrSplitRest].(bool); !rest {
		hv, _ := vl.Attributes["_splittableHv"].(HTMLValues)
		room -= hv.PaddingTop + hv.BorderTopWidth
	}
	batch, next, overflow, inner, brk := cb.fitChildren(children, 0, room, forced, cb.fragmentTrim(vl))
	if overflow || next >= len(children) {
		return nil
	}
	if brk != "" || inner.heightCut() {
		// A forced break is taken whatever orphans, widows or avoid say.
	} else if _, leaf := vl.Attributes["_splittableTe"]; leaf {
		fl := fragLinesOf(vl)
		if countContent(batch) < fl.orphans {
			return nil
		}
		var rest int
		batch, next, rest = pullBackForWidows(children, batch, next, fl)
		if rest < fl.widows {
			return nil
		}
		batch, next = keepFloatWithChild(children, batch, next)
	} else if inner == nil {
		batch, next = pullBackForAvoid(children, batch, next)
		batch, next = keepFloatWithChild(children, batch, next)
	}
	if countContent(batch) == 0 && inner == nil {
		return nil
	}
	return &splitPlan{vl: vl, children: children, n: next, inner: inner, brk: brk, room: room}
}

// carryMarks puts the heading and the anchors of the split block vl on frag,
// its first fragment, where flushInsertsIn finds them: the page reference
// is the page the block starts on.
func carryMarks(vl, frag *node.VList) {
	for _, k := range []string{"_heading_idx", "_anchor_idx", "_anchor_indices"} {
		if v, ok := vl.Attributes[k]; ok {
			frag.SetAttribute(k, v)
		}
	}
}

// cutBlock builds the cut p plans: the fragment before it, which carries the
// block's heading and anchors, and the rest of the block, which splits
// again.
func (cb *CSSBuilder) cutBlock(p *splitPlan) (node.Node, node.Node) {
	if p.heightCut() {
		return cssHeightKern(p.room), cssHeightKern(p.height.Kern - p.room)
	}
	vl := p.vl
	headItems := append([]node.Node(nil), p.children[:p.n]...)
	restItems := append([]node.Node(nil), p.children[p.n:]...)
	if p.inner != nil {
		h, r := cb.cutBlock(p.inner)
		headItems = append(headItems, h)
		restItems[0] = r
	}
	// The children are still linked as the block was built.
	for _, c := range headItems {
		c.SetPrev(nil)
		c.SetNext(nil)
	}
	for _, c := range restItems {
		c.SetPrev(nil)
		c.SetNext(nil)
	}
	innerWidth, _ := vl.Attributes["_splittableInnerWidth"].(bag.ScaledPoint)
	kind := fragTop
	if r, _ := vl.Attributes[attrSplitRest].(bool); r {
		kind = fragMiddle
	}
	if trim := cb.fragmentTrim(vl); trim != (breakTrim{}) {
		if trim.end && (trim.clone || p.brk == "") {
			trimFragmentEnd(headItems, trim.clone)
		}
		if trim.start {
			trimFragmentStart(restItems)
		}
	}
	// The head is placed now; fitChildren may have weighed an overhang in it
	// as the room left.
	cb.endOverhangAt(headItems, p.room)
	head, _ := cb.buildFragment(vl, headItems, kind, innerWidth)
	rest, _ := cb.buildFragment(vl, restItems, fragBottom, innerWidth)
	carryMarks(vl, head)
	for _, k := range []string{"_splittable", "_splittableHv", "_splittableInnerWidth", "_splittableTe", attrFragLines, "pageBreakAfter", attrDecorationClone} {
		if v, ok := vl.Attributes[k]; ok {
			rest.SetAttribute(k, v)
		}
	}
	rest.SetAttribute("_splittableInner", restItems)
	rest.SetAttribute(attrSplitRest, true)
	stampFragment(head, vl)
	stampFragment(rest, vl)
	return head, rest
}

// pullBackForWidows is widow protection: the rest, children from i on, must
// carry at least `widows` content children; otherwise children are pulled
// back from the end of batch until it does, while leaving at least `orphans`
// in batch (don't trade a widow for an orphan). A pulled-back VList counts
// like an HList, as in countContent: only counting HLists never advances
// remainingLines for a box container (a <ul> whose children are <li>
// VLists), so the loop would drain the batch down to the orphan minimum and
// leave the page half empty. It returns the batch, the index of the rest and
// the content children the rest carries.
func pullBackForWidows(children, batch []node.Node, i int, fl fragLines) ([]node.Node, int, int) {
	remainingLines := countContent(children[i:])
	for remainingLines < fl.widows && countContent(batch) > fl.orphans {
		last := batch[len(batch)-1]
		batch = batch[:len(batch)-1]
		i--
		if isContentNode(last) {
			remainingLines++
		}
	}
	return batch, i, remainingLines
}

// keepFloatWithChild keeps a float at the end of batch with the child beside
// it, children[i], which the widow pullback may just have moved on. The
// float goes with that child; when it is all the batch holds, the child
// comes along instead, whether the page has room for it or not.
func keepFloatWithChild(children, batch []node.Node, i int) ([]node.Node, int) {
	for i < len(children) && len(batch) > 0 {
		last := batch[len(batch)-1]
		if _, isFloat := floatBoxHeight(last); !isFloat {
			break
		}
		if countContent(batch) == 0 {
			for i < len(children) {
				c := children[i]
				batch = append(batch, c)
				i++
				if isContentNode(c) {
					break
				}
			}
			break
		}
		batch = batch[:len(batch)-1]
		i--
	}
	return batch, i
}

// splitColor is the text color of the paragraph a split leaf block was built
// from, or nil. The node builder emits a single color instruction inside the
// first line and the reset inside the last one; a page's content stream
// starts with default black, so a fragment on a later page would silently
// lose the color. Black is skipped, it equals the content-stream default.
// Box-container splittables carry no source Text; their children hold
// self-contained color instructions already.
func (cb *CSSBuilder) splitColor(blockVL *node.VList) *color.Color {
	splitTe, _ := blockVL.Attributes["_splittableTe"].(*frontend.Text)
	if splitTe == nil {
		return nil
	}
	var c *color.Color
	switch t := splitTe.Settings[frontend.SettingColor].(type) {
	case string:
		c = cb.frontend.GetColor(t)
	case *color.Color:
		c = t
	}
	if c != nil {
		if black := cb.frontend.GetColor("black"); black != nil &&
			c.PDFStringNonStroking() == black.PDFStringNonStroking() {
			return nil
		}
	}
	return c
}

// colorStartNode and colorResetNode build zero-height StartStop nodes whose
// shipout callbacks mirror the instructions the node builder emits for
// SettingColor.
func colorStartNode(c *color.Color) node.Node {
	s := node.NewStartStop()
	s.Position = node.PDFOutputPage
	s.ShipoutCallback = func(n node.Node) string {
		return c.PDFStringNonStroking() + " "
	}
	return s
}

func colorResetNode() node.Node {
	s := node.NewStartStop()
	s.Position = node.PDFOutputPage
	s.ShipoutCallback = func(n node.Node) string {
		return "0 0 0 RG 0 0 0 rg "
	}
	return s
}

// buildFragment builds a fragment of kind from items, children of the
// splittable block blockVL, innerWidth wide. A block with a border or a
// background is wrapped with HTMLBorder, its paddings and borders dropped on
// the cut sides; a bare block (vlistbuilder.go marks it with
// hv == HTMLValues{}) is not wrapped. A fragment after the first re-emits the
// color of a split paragraph, and a fragment the paragraph continues after
// resets it, as the page ends mid-paragraph and floats and footnotes paint
// after the body in the same stream.
func (cb *CSSBuilder) buildFragment(blockVL *node.VList, items []node.Node, kind int, innerWidth bag.ScaledPoint) (*node.VList, bag.ScaledPoint) {
	hv, _ := blockVL.Attributes["_splittableHv"].(HTMLValues)
	if c := cb.splitColor(blockVL); c != nil {
		if kind == fragMiddle || kind == fragBottom {
			items = append([]node.Node{colorStartNode(c)}, items...)
		}
		if kind == fragTop || kind == fragMiddle {
			items = append(items, colorResetNode())
		}
	}
	_, _, clone := decorationClone(blockVL)
	if pad, _ := blockVL.Attributes[attrDecorationClone].(clonePadding); clone {
		items = padFragment(items, pad)
	}
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
	// Carry the PDF/UA StructureElement linkage onto the first fragment.
	// tagVList stamps it on blockVL before splitting; without this copy the
	// wrapped fragment would have no /K MCR entry and the StructElem would
	// be structurally empty. Only the first fragment (Only / Top) gets the
	// tag — later fragments would create duplicate MCID references.
	tag, hasTag := blockVL.Attributes["tag"]
	hasTag = hasTag && (kind == fragOnly || kind == fragTop)
	if hasTag {
		innerVL.SetAttribute("tag", tag)
	}
	// The block's horizontal shift (margin-left plus the parent's
	// padding-left, stamped by the box branch of buildVlistInternal) sits on
	// the original VList; every fragment must inherit it or an indented
	// block (e.g. a blockquote) snaps to the left edge.
	if !hv.hasBorder() && hv.BackgroundColor == nil {
		innerVL.ShiftX = blockVL.ShiftX
		return innerVL, vlistNodeHeight(innerVL)
	}
	fragHv := hv
	if kind != fragTop && kind != fragOnly && !clone {
		fragHv.PaddingTop = 0
		fragHv.BorderTopWidth = 0
	}
	if kind != fragBottom && kind != fragOnly && !clone {
		fragHv.PaddingBottom = 0
		fragHv.BorderBottomWidth = 0
	}
	wrapped := cb.HTMLBorder(innerVL, fragHv)
	// Same rationale for the bordered path: HTMLBorder produced a fresh
	// outer VList, so the tag would otherwise be dropped.
	if hasTag {
		wrapped.SetAttribute("tag", tag)
	}
	wrapped.ShiftX = blockVL.ShiftX
	return wrapped, vlistNodeHeight(wrapped)
}

// outputTableRows unpacks a table VList into individual rows and places them
// on pages, repeating header rows after each page break.
//
// A nil buildHeadersFn is a table without header rows. Its <tfoot> rows, if
// any, are then placed once at the end like the other rows, as they are for
// such tables without a row that breaks inside.
func (cb *CSSBuilder) outputTableRows(tableVL *node.VList, buildHeadersFn any, y *bag.ScaledPoint, yLimit *bag.ScaledPoint, pageHasContent *bool, fc *flowCursor) error {
	noHeaders := buildHeadersFn == nil
	buildHeaders := func() ([]*node.HList, error) { return nil, nil }
	headerCount := 0
	if !noHeaders {
		buildHeaders = buildHeadersFn.(func() ([]*node.HList, error))
		headerCount = tableVL.Attributes["_headerCount"].(int)
	}
	tableWidth := tableVL.Width
	// curContentWidth tracks the content width the table rows were built
	// for. A page break onto a page with a different width triggers a
	// rebuild of the table (see below).
	curContentWidth := fc.cur.width

	// Footer support: tables with <tfoot> repeat the footer at the
	// bottom of every page they span (HTML semantics, CSS Tables 3 §11.1).
	var footerCount int
	var footerHeight bag.ScaledPoint
	var buildFooters func() ([]*node.HList, error)
	if !noHeaders {
		if fc, ok := tableVL.Attributes["_footerCount"].(int); ok {
			footerCount = fc
		}
		if fh, ok := tableVL.Attributes["_footerHeight"].(bag.ScaledPoint); ok {
			footerHeight = fh
		}
		if bf, ok := tableVL.Attributes["_buildFooters"].(func() ([]*node.HList, error)); ok {
			buildFooters = bf
		}
	}

	// Collect all row nodes from the table VList. The trailing
	// footerCount rows are pulled out of the normal stream and placed
	// explicitly at the end of each page.
	var rows []node.Node
	for n := tableVL.List; n != nil; n = n.Next() {
		rows = append(rows, n)
	}
	dataEnd := len(rows) - footerCount
	// A row is placed in a box of the table's width, shifted by the
	// table's margin-left. It is taken here, as a table rebuilt at another
	// width below has none.
	shiftX := tableVL.ShiftX
	shifted := func(box *node.VList) *node.VList {
		box.ShiftX = shiftX
		return box
	}

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
			propagateFlowChild(tableVL, box)
			fc.cur.output(*y, shifted(box), h)
			*y -= h
		}
		*pageHasContent = true
		return nil
	}

	// breakTo goes on in the next region. An occupied one takes the table
	// only when need, the repeated headers and the next rows, fits there;
	// otherwise the table moves on from it once, as a block does.
	breakTo := func(need bag.ScaledPoint) error {
		for moved := false; ; moved = true {
			if err := fc.breakTo("", tableVL); err != nil {
				return err
			}
			*y = fc.cur.top
			*yLimit = fc.cur.bottom()
			*pageHasContent = false
			if moved || !fc.cur.occupied || *y-need >= *yLimit+footerHeight {
				return nil
			}
		}
	}

	// carry is set when rows[i] is the rest of a row split at the end of the
	// last page, so it starts a new page.
	carry := false
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
		pageContent := fc.cur.height
		effectiveLimit := *yLimit + footerHeight
		// fitH is what must still fit on this page for row i to be placed
		// here. For the very first row of a table with headers that is the
		// whole header block plus the first data row: headers that fit while
		// the first data row does not would sit orphaned at the bottom of
		// the page, with the data starting on the next page under a repeated
		// header. The empty-page guard mirrors avoidForcesBreak — when even
		// a fresh page cannot hold the group, breaking cannot help.
		//
		// Rows a rowspan joins (_keepWithNext) go to a page together, so
		// the break is only taken before the first of them. A group taller
		// than a page then starts at the top of one and breaks like other
		// rows.
		fitH := h
		if i >= headerCount && !carry && (i == headerCount || !keepsWithNext(rows[i-1])) {
			fitH = keepGroupHeight(rows[:dataEnd], i)
		}
		if i == 0 && headerCount > 0 && dataEnd > headerCount {
			headersH := bag.ScaledPoint(0)
			for j := 0; j < headerCount; j++ {
				headersH += vlistNodeHeight(rows[j])
			}
			groupH := headersH + keepGroupHeight(rows[:dataEnd], headerCount)
			// A first data row that may break inside only needs a part of
			// it to follow the headers here.
			if sp := rowSplitterOf(rows[headerCount]); sp != nil && *y-headersH > effectiveLimit {
				if _, _, ok := sp(*y - headersH - effectiveLimit); ok {
					groupH = headersH
				}
			}
			if groupH+footerHeight <= pageContent {
				fitH = groupH
			}
		}
		// A row that may break inside is split where the page ends, if a
		// line of it fits there; the rest goes on after the next page's
		// header rows. The limit is read when the split is made: after a
		// break it is the foot of the region the rest goes on in.
		var rest *node.HList
		splitHere := func() {
			limit := *yLimit + footerHeight
			if i < headerCount || *y-h >= limit {
				return
			}
			if sp := rowSplitterOf(row); sp != nil {
				if first, more, ok := sp(*y - limit); ok {
					row, rest, h = first, more, vlistNodeHeight(first)
				}
			}
		}
		if !carry {
			splitHere()
		}
		avoidForcesBreak := avoidBreakInside(row) && *y-h < effectiveLimit && !*pageHasContent && h+footerHeight <= pageContent
		if carry || rest == nil && ((*y-fitH < effectiveLimit && *pageHasContent) || avoidForcesBreak) {
			// Place footer at the bottom of the current page before
			// breaking so it appears on every spanned page. At i == 0 no
			// row of this table is on the page yet, unless it is carried
			// from a split, so there is nothing for a footer to close off.
			if i > 0 || carry {
				if err := placeFooters(); err != nil {
					return err
				}
			}
			need := fitH
			if carry {
				need = h
			}
			if i >= headerCount {
				for j := 0; j < headerCount; j++ {
					need += vlistNodeHeight(rows[j])
				}
			}
			if err := breakTo(need); err != nil {
				return err
			}

			// The fresh page has a different content width (@page :first
			// vs. @page): rebuild the table from its source Text at the new
			// width, so the remaining rows and the repeated headers span
			// the new measure. Rows are matched by position — buildTable is
			// deterministic and the row count is width-independent. The
			// already-placed rows keep their old width; when the rebuild is
			// not possible the old rows are sliced as before.
			if fc.cur.width != curContentWidth {
				if te, ok := tableVL.Attributes["_tableTe"].(*frontend.Text); ok {
					if teWd, ok := tableVL.Attributes["_tableTeWidth"].(bag.ScaledPoint); ok {
						newWd := teWd + (fc.cur.width - curContentWidth)
						if newWd > 0 {
							cb.reflowRebuild = true
							newVL, err := cb.buildTable(te, newWd)
							cb.reflowRebuild = false
							if err == nil && newVL != nil {
								var newRows []node.Node
								for n := newVL.List; n != nil; n = n.Next() {
									newRows = append(newRows, n)
								}
								if len(newRows) == len(rows) {
									// The rest of a split row cannot be matched
									// in the rebuilt table, so it keeps the
									// old width.
									if carry {
										newRows[i] = row
									}
									rows = newRows
									// The rebuilt table is still the same flow child.
									propagateFlowChild(tableVL, newVL)
									tableVL = newVL
									tableWidth = newVL.Width
									if bh, ok := newVL.Attributes["_buildHeaders"].(func() ([]*node.HList, error)); ok {
										buildHeaders = bh
									}
									if fh, ok := newVL.Attributes["_footerHeight"].(bag.ScaledPoint); ok && !noHeaders {
										footerHeight = fh
									}
									if bf, ok := newVL.Attributes["_buildFooters"].(func() ([]*node.HList, error)); ok && !noHeaders {
										buildFooters = bf
									}
									row = rows[i]
									h = vlistNodeHeight(row)
								} else {
									bag.Logger.Debug("width reflow of split table failed: row count mismatch", "old", len(rows), "new", len(newRows))
								}
							} else if err != nil {
								bag.Logger.Debug("width reflow of split table failed", "error", err)
							}
						}
					}
				}
				curContentWidth = fc.cur.width
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
					propagateFlowChild(tableVL, box)
					fc.cur.output(*y, shifted(box), hdrH)
					*y -= hdrH
				}
				*pageHasContent = true
			}
			carry = false
			splitHere()
		}

		// Detach row from linked list and place it.
		row.SetPrev(nil)
		row.SetNext(nil)

		box := node.NewVList()
		box.List = row
		box.Width = tableWidth
		box.Height = h

		propagateFlowChild(tableVL, box)
		fc.cur.output(*y, shifted(box), h)
		for _, idx := range anchorIndicesOn(row) {
			if idx >= 0 && idx < len(cb.Anchors) {
				cb.Anchors[idx].Page = fc.cur.pageNum
			}
		}
		*y -= h
		*pageHasContent = true
		if rest != nil {
			rows[i] = rest
			carry = true
			i--
		}
	}

	// Footer on the last page.
	return placeFooters()
}

func keepsWithNext(n node.Node) bool {
	if hl, ok := n.(*node.HList); ok {
		keep, _ := hl.Attributes["_keepWithNext"].(bool)
		return keep
	}
	return false
}

// keepGroupHeight is the height of rows[i] and the rows it keeps with.
func keepGroupHeight(rows []node.Node, i int) bag.ScaledPoint {
	h := vlistNodeHeight(rows[i])
	for j := i; j+1 < len(rows) && keepsWithNext(rows[j]); j++ {
		h += vlistNodeHeight(rows[j+1])
	}
	return h
}

// rowSplitterOf returns the splitter bag gives a table row that may break
// inside, or nil.
func rowSplitterOf(n node.Node) frontend.RowSplitter {
	if hl, ok := n.(*node.HList); ok {
		sp, _ := hl.Attributes["_split"].(frontend.RowSplitter)
		return sp
	}
	return nil
}

// avoidBreakAfter checks if a node has the page-break-after: avoid attribute.
func avoidBreakAfter(n node.Node) bool {
	vl, ok := n.(*node.VList)
	if !ok {
		return false
	}
	if vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakAfter"]; ok {
			return v == "avoid"
		}
	}
	// CSS Fragmentation 3 §3.3: a break-after on the last in-flow child
	// also applies between the parent and the parent's next sibling. The
	// paginator only inspects top-level nodes, so a heading wrapped in a
	// <div>/<section> would otherwise never be seen. Walk down the chain
	// of last content children; the depth guard keeps a pathological tree
	// from recursing without bound.
	for depth := 0; depth < 32; depth++ {
		vl = lastContentChild(vl)
		if vl == nil {
			return false
		}
		if vl.Attributes != nil {
			if v, ok := vl.Attributes["pageBreakAfter"]; ok {
				return v == "avoid"
			}
		}
	}
	return false
}

// avoidChainHeight returns the height that has to fit for the run of
// break-after: avoid blocks starting at head to stay where it is, and the
// run's blocks after head. The height is the run with the margins inside it,
// and the foothold of the block after it: its first lines when it can split,
// all of it otherwise.
func avoidChainHeight(head node.Node) (bag.ScaledPoint, []node.Node) {
	need := vlistNodeHeight(head)
	var rest []node.Node
	for n := head.Next(); n != nil; n = n.Next() {
		if fh, isFloat := floatBoxHeight(n); isFloat {
			return need + floatKeepWithNext(fh, siblingsFrom(n.Next())), rest
		}
		if !isContentNode(n) {
			need += vlistNodeHeight(n)
			continue
		}
		if !avoidBreakAfter(n) {
			if reduced, ok := splittablePeekHeight(n); ok {
				return need + reduced, rest
			}
			return need + vlistNodeHeight(n), rest
		}
		need += vlistNodeHeight(n)
		rest = append(rest, n)
	}
	return need, rest
}

// lastContentChild returns the last VList child of vl, skipping trailing
// kerns, glue and rules (margins, padding fillers, background rules). It
// returns nil when the list has no VList child, which ends the walk in
// avoidBreakAfter.
func lastContentChild(vl *node.VList) *node.VList {
	var last *node.VList
	for n := vl.List; n != nil; n = n.Next() {
		if child, ok := n.(*node.VList); ok {
			last = child
		}
	}
	return last
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
	fl := fragLinesOf(vl)
	orphans := fl.orphans
	// A paragraph that cannot leave `orphans` lines here and `widows` on
	// the next page is not split but moved on whole (see outputBlockSplit),
	// so it offers no foothold.
	if _, leaf := vl.Attributes["_splittableTe"].(*frontend.Text); leaf {
		lines := 0
		for _, c := range children {
			if isContentNode(c) {
				lines++
			}
		}
		if lines < fl.orphans+fl.widows {
			return 0, false
		}
	}
	peek := hv.PaddingTop + hv.BorderTopWidth
	if top, bottom, clone := decorationClone(vl); clone {
		peek = top + bottom
	}
	if _, leaf := vl.Attributes["_splittableTe"].(*frontend.Text); leaf {
		// Reserve room for `orphans` lines, not just the first one:
		// outputBlockSplit refuses to start a paragraph that would leave
		// fewer than that on the current page and moves it on whole.
		// Promising the caller a one-line foothold would therefore orphan
		// the very heading this relaxation exists to keep in place. Leading
		// padding kerns are counted towards the height but are not content.
		seen := 0
		for _, c := range children {
			peek += vlistNodeHeight(c)
			if isContentNode(c) {
				seen++
				if seen >= orphans {
					return peek, true
				}
			}
		}
		return 0, false
	}
	// A container splits after its first block, or through it: its
	// foothold is the foothold of that block.
	for _, c := range children {
		if !isContentNode(c) {
			peek += vlistNodeHeight(c)
			continue
		}
		if p, ok := splittablePeekHeight(c); ok {
			return peek + p, true
		}
		return peek + vlistNodeHeight(c), true
	}
	return 0, false
}

// floatKeepWithNext is the height a page has to have free to place a float
// box of painted extent fh together with the block after it. The two overlap:
// the block starts level with the float, so the taller of the two decides.
// Margin kerns between them belong to the block. A splittable block needs
// only its foothold, the lines the splitter's orphan rule keeps on the page
// (see splittablePeekHeight).
func floatKeepWithNext(fh bag.ScaledPoint, rest []node.Node) bag.ScaledPoint {
	var beside bag.ScaledPoint
	for _, n := range rest {
		if !isContentNode(n) {
			beside += vlistNodeHeight(n)
			continue
		}
		if reduced, ok := splittablePeekHeight(n); ok {
			beside += reduced
		} else {
			beside += vlistNodeHeight(n)
		}
		break
	}
	if fh > beside {
		return fh
	}
	return beside
}

// siblingsFrom collects a chain into a slice, up to and including the first
// content node: what floatKeepWithNext needs to see of it.
func siblingsFrom(n node.Node) []node.Node {
	var out []node.Node
	for ; n != nil; n = n.Next() {
		out = append(out, n)
		if isContentNode(n) {
			break
		}
	}
	return out
}

// isContentNode reports whether a sibling is a block or a line, as opposed to
// the kerns and glues between them and a float box, which paints beside the
// content rather than being any of it.
func isContentNode(n node.Node) bool {
	switch n.(type) {
	case *node.HList, *node.VList:
		_, isFloat := floatBoxHeight(n)
		return !isFloat
	}
	return false
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

// inlineNodeElement is the name of InlineNode's element, an inline one.
const inlineNodeElement = "bag-inline-node"

// InlineNode returns an element that stands for n in a document given to
// ParseHTMLFromNode. Where the element is in a paragraph, n goes into the
// line, as an inline image does, so a node.StartStop there is shipped out
// with its line and on its page. The element has no content, and styles do
// not apply to it. n is copied each time the paragraph is formatted, which
// keeps a StartStop's ShipoutCallback.
func (cb *CSSBuilder) InlineNode(n node.Node) *html.Node {
	elt := &html.Node{Type: html.ElementNode, Data: inlineNodeElement}
	if cb.inlineNodes == nil {
		cb.inlineNodes = map[*html.Node]node.Node{}
	}
	cb.inlineNodes[elt] = n
	return elt
}

// inlineNode is the node InlineNode made elt stand for.
func (cb *CSSBuilder) inlineNode(elt *html.Node) (node.Node, bool) {
	if cb == nil || elt == nil {
		return nil, false
	}
	n, ok := cb.inlineNodes[elt]
	return n, ok
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
	// The same goes for @-bag-color: the names must be defined before the
	// color properties of this chunk are resolved.
	if err := AddColorsFromCSS(cb.css, cb.frontend); err != nil {
		return nil, err
	}

	var te *frontend.Text
	if te, err = HTMLNodeToText(cb, n, cb.stylesStack, cb.frontend, cb.anchorPages); err != nil {
		return nil, err
	}

	return te, nil
}

// RegisterLineModel makes name a value of -bag-leading-model that sets a
// paragraph's lines by the model f returns, which htmlbag passes to bag as
// frontend.SettingLineModel. Names are case-insensitive. half, trailing, a
// CSS-wide keyword and the empty name are rejected; registering a name again
// replaces its function.
func (cb *CSSBuilder) RegisterLineModel(name string, f LineModelFunc) error {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case name == "":
		return fmt.Errorf("line model: empty name")
	case builtinLeadingModel(name):
		return fmt.Errorf("line model %q: the name is reserved", name)
	case f == nil:
		return fmt.Errorf("line model %q: nil function", name)
	}
	if cb.lineModels == nil {
		cb.lineModels = map[string]LineModelFunc{}
	}
	cb.lineModels[name] = f
	return nil
}

// RegisterBreaker makes name a value of -bag-line-breaker that chooses a
// paragraph's breaks by the breaker f returns, which htmlbag passes to bag as
// frontend.SettingBreaker. Names are case-insensitive. auto, a CSS-wide keyword
// and the empty name are rejected; registering a name again replaces its
// function.
func (cb *CSSBuilder) RegisterBreaker(name string, f BreakerFunc) error {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case name == "":
		return fmt.Errorf("breaker: empty name")
	case builtinBreaker(name):
		return fmt.Errorf("breaker %q: the name is reserved", name)
	case f == nil:
		return fmt.Errorf("breaker %q: nil function", name)
	}
	if cb.breakers == nil {
		cb.breakers = map[string]BreakerFunc{}
	}
	cb.breakers[name] = f
	return nil
}

// AddCSS reads the CSS instructions in css.
func (cb *CSSBuilder) AddCSS(css string) error {
	curwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cb.css.PushDir(curwd)
	defer cb.css.PopDir()
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

func hasContents(areaAttributes StyleMap, contentTokens []ContentToken) bool {
	if len(contentTokens) > 0 {
		return true
	}
	content := areaAttributes.Get("content")
	return content != "none" && content != "normal"
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
	bag.Logger.Debug("Read file", "filename", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return err
	}
	cb.css.PushDir(abs)
	defer cb.css.PopDir()
	return cb.css.AddCSSText(string(data))
}

// shiftChildren moves the shift (margin-left) of a box that is being
// unwrapped onto its children, as it is dropped with the box; each placed
// node is wrapped in a fresh box, where a child's ShiftX takes effect.
func shiftChildren(vl *node.VList) {
	if vl.ShiftX == 0 {
		return
	}
	for n := vl.List; n != nil; n = n.Next() {
		switch c := n.(type) {
		case *node.VList:
			c.ShiftX += vl.ShiftX
		case *node.HList:
			c.ShiftX += vl.ShiftX
		}
	}
}
