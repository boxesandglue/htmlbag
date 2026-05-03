package htmlbag

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
)

// InsertClass distinguishes the layout role of an Insert: footnotes stack at
// the bottom of the page, top-floats at the top of the content area, etc.
type InsertClass int

const (
	// InsertFootnote: bottom-of-page stack, separator rule above, running
	// number rendered as a superscript call in body text.
	InsertFootnote InsertClass = iota
	// InsertFloatTop: top-of-page stack, no separator, no in-text marker.
	// With Phase 3 (two-pass page assembly), top-floats and their source
	// paragraphs share a page when they fit; otherwise the page-builder
	// ships and starts fresh.
	InsertFloatTop
	// InsertFloatBottom: bottom-of-page float stack, painted between the
	// body content and the footnote stack. No separator, no in-text
	// marker. Same fit-or-ship semantics as InsertFloatTop.
	InsertFloatBottom
)

// Detection inputs for footnote inline elements.
//
// An inline element is treated as a footnote if any of the following holds:
//   - tag name equals footnoteTagName ("fn")
//   - one of its class tokens equals footnoteClassName ("footnote")
//
// CSS GCPM `float: footnote` is a possible third trigger; deferred until the
// CSS pipeline supports the value.
const (
	footnoteTagName   = "fn"
	footnoteClassName = "footnote"
)

// Float elements are detected via the CSS `float` property on any HTML
// element (typically `<span>` for inline-content floats and `<div>` for
// block-content floats). Recognised values:
//
//   - "top" / "before"   → InsertFloatTop
//   - "bottom" / "after" → InsertFloatBottom
//
// Other values (e.g. "left"/"right" — standard CSS side-floats with text
// wrap) are not handled and pass through to normal block/inline rendering.

// Default values for footnote layout. Used to seed CSSBuilder fields in New();
// callers may override the corresponding cb.Footnote* fields after construction.
var (
	defaultFootnoteSeparatorHeight = bag.MustSP("0.4pt")
	defaultFootnoteSeparatorSkip   = bag.MustSP("6pt") // skip between content area bottom and rule
	defaultFootnoteInterSkip       = bag.MustSP("2pt") // skip between consecutive footnotes
	defaultFloatTopInterSkip       = bag.MustSP("6pt") // skip between consecutive top-floats and below the stack
	defaultFloatBottomInterSkip    = bag.MustSP("6pt") // skip between consecutive bottom-floats and above the stack
)

// Marker call (in-text superscript) sizing relative to the surrounding font
// size. PDF rise (Ts operator) is positive = upward.
const (
	defaultFootnoteCallSizeRatio = 0.7
	defaultFootnoteCallRiseRatio = 0.4
)

// Insert is one extracted, fully formatted page-layer item ready for
// placement (footnote, float, ...). The Class field selects the placement
// strategy at flush time.
//
// For InsertFootnote: Body width equals the footnote-area width (currently
// the paragraph content width); Number is assigned at extraction time as a
// running document counter (cb.Counters["footnote"]).
type Insert struct {
	Class  InsertClass
	Number int
	Body   *node.VList
}

// insertMarker is a sentinel value placed inside frontend.Text.Items at
// inline-collection time. It is invisible to FormatParagraph: per-class
// extractors must replace every insertMarker with a class-appropriate in-text
// placeholder (or nothing) and lift the body out before FormatParagraph runs.
//
// Body is the raw, unformatted Text subtree of the insert content. Class
// selects which extractor handles it.
type insertMarker struct {
	Class InsertClass
	Body  *frontend.Text
}

// isFootnoteElement reports whether an HTMLItem should be treated as a
// footnote by extraction. Called from collectHorizontalNodes.
func isFootnoteElement(item *HTMLItem) bool {
	if item == nil {
		return false
	}
	if item.Data == footnoteTagName {
		return true
	}
	if cls, ok := item.Attributes["class"]; ok {
		if slices.Contains(strings.Fields(cls), footnoteClassName) {
			return true
		}
	}
	return false
}

// isFloatElement reports whether an HTMLItem should be treated as a float
// (any position) based on its computed CSS `float` property.
func isFloatElement(item *HTMLItem) bool {
	if item == nil {
		return false
	}
	switch item.Styles["float"] {
	case "top", "before", "bottom", "after":
		return true
	}
	return false
}

// floatClassFor maps the CSS `float` property of an element to an
// InsertClass. Default is top for any non-bottom value, matching the
// XSL-FO `float="before"` default. Caller is expected to gate on
// isFloatElement first (this function returns InsertFloatTop for any
// item whose float style isn't a recognised value).
func floatClassFor(item *HTMLItem) InsertClass {
	if item == nil {
		return InsertFloatTop
	}
	switch item.Styles["float"] {
	case "bottom", "after":
		return InsertFloatBottom
	default:
		return InsertFloatTop
	}
}

// extractFootnotesShallow scans only the direct te.Items for footnote-class
// insertMarkers (no recursion into nested *frontend.Text), replaces each
// with the appropriate superscript call, and returns the formatted Inserts.
//
// Used by buildVlistInternal at the top to catch *block-level* markers
// that sit as direct siblings of paragraph Texts in a body container.
// The paragraph-branch's deep extractFootnotes catches markers nested
// inside paragraph subtrees (e.g. footnote inside a span inside a <p>).
func (cb *CSSBuilder) extractFootnotesShallow(te *frontend.Text, footnoteWidth bag.ScaledPoint) ([]*Insert, error) {
	if te == nil {
		return nil, nil
	}
	var ins []*Insert
	for i, itm := range te.Items {
		t, ok := itm.(insertMarker)
		if !ok || t.Class != InsertFootnote {
			continue
		}
		cb.Counters["footnote"]++
		number := cb.Counters["footnote"]
		body, err := cb.formatFootnoteBody(t.Body, number, footnoteWidth)
		if err != nil {
			return nil, err
		}
		if cb.enableTagging && cb.structureCurrent != nil {
			noteSE := &document.StructureElement{
				Role:       "Note",
				ActualText: extractTextContent(t.Body),
			}
			cb.structureCurrent.AddChild(noteSE)
			tagVList(body, noteSE)
		}
		te.Items[i] = cb.makeFootnoteCall(te.Settings, number)
		ins = append(ins, &Insert{Class: InsertFootnote, Number: number, Body: body})
	}
	return ins, nil
}

// extractFloatsShallow is the float-class counterpart of
// extractFootnotesShallow: it finds direct float-class insertMarkers in
// te.Items (no recursion), replaces them with empty placeholders, and
// returns the formatted Inserts.
func (cb *CSSBuilder) extractFloatsShallow(te *frontend.Text, floatWidth bag.ScaledPoint, class InsertClass) ([]*Insert, error) {
	if te == nil {
		return nil, nil
	}
	var ins []*Insert
	for i, itm := range te.Items {
		t, ok := itm.(insertMarker)
		if !ok || t.Class != class {
			continue
		}
		body, err := cb.CreateVlist(t.Body, floatWidth)
		if err != nil {
			return nil, err
		}
		te.Items[i] = frontend.NewText()
		ins = append(ins, &Insert{Class: class, Body: body})
	}
	return ins, nil
}

// extractFootnotes walks te recursively (depth-first over Items, descending
// into nested *frontend.Text), finds every insertMarker of class
// InsertFootnote, and:
//
//   - replaces the marker with a superscript-style call carrying the running
//     number (cb.Counters["footnote"])
//   - formats the marker's Body into a node.VList of width footnoteWidth,
//     prefixed with "<n>. "
//   - returns the resulting []*Insert in document order
//
// Returned inserts all have Class == InsertFootnote.
func (cb *CSSBuilder) extractFootnotes(te *frontend.Text, footnoteWidth bag.ScaledPoint) ([]*Insert, error) {
	if te == nil {
		return nil, nil
	}
	var ins []*Insert
	if err := cb.extractFootnotesInto(te, footnoteWidth, &ins); err != nil {
		return nil, err
	}
	return ins, nil
}

// extractFootnotesInto is the recursive worker for extractFootnotes. It
// mutates te.Items in place: every footnote-class insertMarker is replaced
// with the corresponding call element. Nested *frontend.Text items are
// descended into. Markers of other classes are left untouched here (their
// own extractors will handle them).
func (cb *CSSBuilder) extractFootnotesInto(te *frontend.Text, footnoteWidth bag.ScaledPoint, out *[]*Insert) error {
	for i, itm := range te.Items {
		switch t := itm.(type) {
		case insertMarker:
			if t.Class != InsertFootnote {
				continue
			}
			cb.Counters["footnote"]++
			number := cb.Counters["footnote"]

			body, err := cb.formatFootnoteBody(t.Body, number, footnoteWidth)
			if err != nil {
				return err
			}

			// PDF/UA: tag the body VList as a Note structure element so
			// assistive technology recognizes it as a footnote rather
			// than free-floating text. The Note attaches to the current
			// structure context (typically the enclosing Document or
			// block) — a closer-fitting parent (the actual paragraph SE)
			// would require coordination with vlistbuilder, deferred.
			if cb.enableTagging && cb.structureCurrent != nil {
				noteSE := &document.StructureElement{
					Role:       "Note",
					ActualText: extractTextContent(t.Body),
				}
				cb.structureCurrent.AddChild(noteSE)
				tagVList(body, noteSE)
			}

			te.Items[i] = cb.makeFootnoteCall(te.Settings, number)
			*out = append(*out, &Insert{Class: InsertFootnote, Number: number, Body: body})

		case *frontend.Text:
			if err := cb.extractFootnotesInto(t, footnoteWidth, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// makeFootnoteCall builds the in-text superscripted reference number.
//
// The call inherits the surrounding paragraph's settings (font family, color,
// etc.) and applies a smaller size + positive Y-offset (PDF rise) so the
// number sits above the baseline. Ratios come from cb.FootnoteCallSizeRatio
// and cb.FootnoteCallRiseRatio (configurable per CSSBuilder instance).
func (cb *CSSBuilder) makeFootnoteCall(parentSettings frontend.TypesettingSettings, number int) *frontend.Text {
	call := frontend.NewText()
	maps.Copy(call.Settings, parentSettings)

	baseSize := footnoteBaseSize(parentSettings)
	// Stay in scaled-point integer arithmetic to avoid float→sp double-conversion.
	call.Settings[frontend.SettingSize] = bag.ScaledPoint(float64(baseSize) * cb.FootnoteCallSizeRatio)
	call.Settings[frontend.SettingYOffset] = bag.ScaledPoint(float64(baseSize) * cb.FootnoteCallRiseRatio)

	call.Items = append(call.Items, strconv.Itoa(number))
	return call
}

// formatFootnoteBody turns the raw inline content of a <fn> element into a
// formatted block-level VList of the given width, with a "<n>. " prefix.
//
// The body's font settings (size, family, color) are taken from rawBody as
// they were resolved at HTML/CSS time — i.e. the CSS author controls footnote
// appearance via rules on .footnote / fn directly. No magic resizing.
func (cb *CSSBuilder) formatFootnoteBody(rawBody *frontend.Text, number int, width bag.ScaledPoint) (*node.VList, error) {
	rawBody.Items = append([]any{strconv.Itoa(number) + ". "}, rawBody.Items...)
	vl, _, err := cb.frontend.FormatParagraph(rawBody, width)
	if err != nil {
		return nil, err
	}
	return vl, nil
}

// footnoteBaseSize returns the font size to scale the call from. Falls back
// to 10pt if no size is set in the parent's settings.
func footnoteBaseSize(s frontend.TypesettingSettings) bag.ScaledPoint {
	if v, ok := s[frontend.SettingSize]; ok {
		if sz, ok := v.(bag.ScaledPoint); ok && sz > 0 {
			return sz
		}
	}
	return bag.MustSP("10pt")
}

// pageBufEntry is one body box awaiting placement at shipout time.
// headingIdx is -1 if the box doesn't carry a heading anchor.
type pageBufEntry struct {
	box        *node.VList
	height     bag.ScaledPoint
	headingIdx int
}

// bufferBody appends a body box to the page buffer, updating the running
// height. Called by the page builder in place of a direct OutputAt; the
// buffered entries are painted at flushInserts time once the page's float
// reservation is final.
func (cb *CSSBuilder) bufferBody(box *node.VList, height bag.ScaledPoint, headingIdx int) {
	cb.pageBuf = append(cb.pageBuf, pageBufEntry{box: box, height: height, headingIdx: headingIdx})
	cb.pageBufHeight += height
}

// filterInserts returns the subset of ins whose Class equals class. Returns
// nil for an empty result so callers can use len() == 0 as the absence
// check without an explicit nil branch.
func filterInserts(ins []*Insert, class InsertClass) []*Insert {
	var out []*Insert
	for _, i := range ins {
		if i.Class == class {
			out = append(out, i)
		}
	}
	return out
}

// insertsOnNode returns the []*Insert stored in n.Attributes["inserts"], or
// nil if absent or of unexpected type. Used by the page builder.
//
// Recognises both *node.VList and *node.HList because the unwrap step in
// outputGroupNodes / OutputPages strips outer VLists and propagates the
// attribute onto the first remaining node — which is typically the HList
// of the paragraph's first line.
func insertsOnNode(n node.Node) []*Insert {
	var attrs node.H
	switch t := n.(type) {
	case *node.VList:
		attrs = t.Attributes
	case *node.HList:
		attrs = t.Attributes
	}
	if attrs == nil {
		return nil
	}
	v, ok := attrs["inserts"]
	if !ok {
		return nil
	}
	ins, _ := v.([]*Insert)
	return ins
}

// propagateInsertsAttr moves an []*Insert from a VList that is being
// unwrapped onto the next VList/HList carrier in the linked list starting
// at `to`. Walks past non-carrier nodes (Kerns from CSS margins, etc.)
// because they don't survive the page-builder's per-node attribute lookup.
// Used in the unwrap loops of OutputPages and outputGroupNodes.
func propagateInsertsAttr(from *node.VList, to node.Node) {
	if from == nil || from.Attributes == nil || to == nil {
		return
	}
	ins, ok := from.Attributes["inserts"].([]*Insert)
	if !ok || len(ins) == 0 {
		return
	}
	// Walk forward until we find a carrier (VList or HList). Margin
	// kerns and similar transparent nodes are skipped.
	for cur := to; cur != nil; cur = cur.Next() {
		switch t := cur.(type) {
		case *node.VList:
			if t.Attributes == nil {
				t.Attributes = node.H{}
			}
			if existing, ok := t.Attributes["inserts"].([]*Insert); ok {
				t.Attributes["inserts"] = append(append([]*Insert{}, ins...), existing...)
			} else {
				t.Attributes["inserts"] = ins
			}
			return
		case *node.HList:
			if t.Attributes == nil {
				t.Attributes = node.H{}
			}
			if existing, ok := t.Attributes["inserts"].([]*Insert); ok {
				t.Attributes["inserts"] = append(append([]*Insert{}, ins...), existing...)
			} else {
				t.Attributes["inserts"] = ins
			}
			return
		}
	}
}

// makeFootnoteSeparator builds the horizontal rule that visually separates
// footnotes from the main content area. Drawn black across the full content
// width.
func (cb *CSSBuilder) makeFootnoteSeparator(width bag.ScaledPoint) *node.VList {
	black := cb.frontend.GetColor("black")
	rule := node.NewRule()
	rule.Width = width
	rule.Height = cb.FootnoteSeparatorHeight
	rule.Pre = pdfdraw.NewStandalone().
		ColorNonstroking(*black).
		Rect(0, 0, width, -cb.FootnoteSeparatorHeight).
		Fill().
		String()
	rule.Attributes = node.H{"origin": "footnote separator"}
	vl := node.Vpack(rule)
	vl.Attributes = node.H{"origin": "footnote separator vlist"}
	return vl
}

// flushInserts paints the current page in four layers — top-floats,
// buffered body, bottom-floats, footnotes — and clears the per-page
// state. Called by cb.NewPage() before shipout, and once at the end of
// the final page in OutputPages / OutputPagesFromText.
//
// Painting order:
//  1. Top-floats at yStart, going down (placeFloatTopInserts).
//  2. Buffered body entries (cb.pageBuf), starting just below the top
//     float stack. Heading-index tracking happens here so the recorded
//     page number reflects the *painted* page.
//  3. Bottom-floats just above the footnote zone (placeFloatBottomInserts).
//  4. Footnotes at yLimit, going up (placeFootnoteInserts).
//
// All accumulators (pageInserts of every class, pageBuf) are cleared by
// their respective placers — the next NewPage starts with a fresh page
// state.
func (cb *CSSBuilder) flushInserts() error {
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}

	// Snapshot the top-float reservation height *before* placeFloatTopInserts
	// clears it, so we know where the body cursor starts.
	topFloatHeight := cb.pageInsertHeight[InsertFloatTop]

	if err := cb.placeFloatTopInserts(); err != nil {
		return err
	}

	// Paint the buffered body just below the top-float zone.
	yCursor := pd.Height - pd.MarginTop - topFloatHeight
	pageNum := len(cb.frontend.Doc.Pages)
	for _, entry := range cb.pageBuf {
		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, yCursor, entry.box)
		if entry.headingIdx >= 0 && entry.headingIdx < len(cb.Headings) {
			cb.Headings[entry.headingIdx].Page = pageNum
		}
		yCursor -= entry.height
	}
	cb.pageBuf = nil
	cb.pageBufHeight = 0

	// Bottom-floats first (they need pageInsertHeight[InsertFootnote] to
	// know their floor), then footnotes.
	if err := cb.placeFloatBottomInserts(); err != nil {
		return err
	}
	return cb.placeFootnoteInserts()
}

// placeFloatTopInserts paints the top-float inserts at the top of the
// current page's content area and clears that class's accumulators. The body
// cursor was already started below the reserved zone (see drainDeferredFloats),
// so this method only renders.
//
// No-op if nothing accumulated. Stack order is document order: first marker
// → top of stack.
func (cb *CSSBuilder) placeFloatTopInserts() error {
	fls := cb.pageInserts[InsertFloatTop]
	if len(fls) == 0 {
		return nil
	}
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}
	yTop := pd.Height - pd.MarginTop
	for i, fl := range fls {
		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, yTop, fl.Body)
		yTop -= fl.Body.Height + fl.Body.Depth
		if i < len(fls)-1 {
			yTop -= cb.FloatTopInterSkip
		}
	}
	delete(cb.pageInserts, InsertFloatTop)
	delete(cb.pageInsertHeight, InsertFloatTop)
	return nil
}

// placeFloatBottomInserts paints the bottom-float stack just above the
// footnote zone. The floor is yLimit + footnoteHeight; the stack grows
// upward from there to yLimit + footnoteHeight + bottomFloatHeight.
// First insert in document order ends up at the top of the stack.
//
// Reads cb.pageInsertHeight[InsertFootnote] (must still be valid — call
// before placeFootnoteInserts clears it).
//
// No-op if nothing accumulated.
func (cb *CSSBuilder) placeFloatBottomInserts() error {
	fls := cb.pageInserts[InsertFloatBottom]
	if len(fls) == 0 {
		return nil
	}
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}
	// Top of the bottom-float zone = yLimit + footnoteHeight + stackHeight.
	yTop := pd.MarginBottom + cb.pageInsertHeight[InsertFootnote] + cb.pageInsertHeight[InsertFloatBottom]
	for i, fl := range fls {
		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, yTop, fl.Body)
		yTop -= fl.Body.Height + fl.Body.Depth
		if i < len(fls)-1 {
			yTop -= cb.FloatBottomInterSkip
		}
	}
	delete(cb.pageInserts, InsertFloatBottom)
	delete(cb.pageInsertHeight, InsertFloatBottom)
	return nil
}

// totalFloatBottomHeight measures the vertical space the given
// bottom-float inserts occupy above the footnote zone, including
// inter-float skip and a leading skip above the stack (separating it
// from body content). Returns 0 for an empty slice.
func (cb *CSSBuilder) totalFloatBottomHeight(fls []*Insert) bag.ScaledPoint {
	if len(fls) == 0 {
		return 0
	}
	total := bag.ScaledPoint(0)
	for i, fl := range fls {
		total += fl.Body.Height + fl.Body.Depth
		if i < len(fls)-1 {
			total += cb.FloatBottomInterSkip
		}
	}
	// Leading skip between body content and the float stack.
	total += cb.FloatBottomInterSkip
	return total
}

// totalFloatTopHeight measures the vertical space the given top-float
// inserts occupy at the top of a page, including inter-float skip and a
// trailing skip below the stack (so body content doesn't butt against the
// last float). Returns 0 for an empty slice.
func (cb *CSSBuilder) totalFloatTopHeight(fls []*Insert) bag.ScaledPoint {
	if len(fls) == 0 {
		return 0
	}
	total := bag.ScaledPoint(0)
	for i, fl := range fls {
		total += fl.Body.Height + fl.Body.Depth
		if i < len(fls)-1 {
			total += cb.FloatTopInterSkip
		}
	}
	// Trailing skip between the float stack and body content.
	total += cb.FloatTopInterSkip
	return total
}

// extractFloats walks te recursively, finds every insertMarker of the
// given float class (InsertFloatTop or InsertFloatBottom), and:
//
//   - replaces the marker with an empty placeholder (no in-text glyph,
//     unlike footnotes which become a superscript call)
//   - formats the marker's Body into a node.VList of width floatWidth
//   - returns the resulting []*Insert in document order
//
// Returned inserts all have Class == class.
func (cb *CSSBuilder) extractFloats(te *frontend.Text, floatWidth bag.ScaledPoint, class InsertClass) ([]*Insert, error) {
	if te == nil {
		return nil, nil
	}
	var ins []*Insert
	if err := cb.extractFloatsInto(te, floatWidth, class, &ins); err != nil {
		return nil, err
	}
	return ins, nil
}

// extractFloatsInto recursively replaces matching-class float
// insertMarkers in te.Items with empty placeholders and appends the
// formatted Insert bodies to *out. Markers of other classes are left
// untouched.
//
// Body formatting goes through CreateVlist (not FormatParagraph) so the
// float can contain block-level content — multiple paragraphs, tables,
// lists, etc. — when the source was a `<div style="float: ...">` rather
// than an inline `<span style="float: ...">`. CreateVlist handles both
// shapes correctly.
func (cb *CSSBuilder) extractFloatsInto(te *frontend.Text, floatWidth bag.ScaledPoint, class InsertClass, out *[]*Insert) error {
	for i, itm := range te.Items {
		switch t := itm.(type) {
		case insertMarker:
			if t.Class != class {
				continue
			}
			body, err := cb.CreateVlist(t.Body, floatWidth)
			if err != nil {
				return err
			}
			te.Items[i] = frontend.NewText()
			*out = append(*out, &Insert{Class: class, Body: body})

		case *frontend.Text:
			if err := cb.extractFloatsInto(t, floatWidth, class, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// placeFootnoteInserts writes the footnote-class inserts at the bottom of
// the current page, above pd.MarginBottom, and clears that class's
// accumulators. No-op if nothing accumulated.
func (cb *CSSBuilder) placeFootnoteInserts() error {
	fns := cb.pageInserts[InsertFootnote]
	if len(fns) == 0 {
		return nil
	}
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}
	contentWidth := pd.Width - pd.MarginLeft - pd.MarginRight

	// Top of footnote area = MarginBottom + total height. The skip above
	// the rule is the first thing to subtract.
	yTop := pd.MarginBottom + cb.pageInsertHeight[InsertFootnote] - cb.FootnoteSeparatorSkip
	sep := cb.makeFootnoteSeparator(contentWidth)
	cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, yTop, sep)
	yTop -= cb.FootnoteSeparatorHeight
	for i, fn := range fns {
		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, yTop, fn.Body)
		yTop -= fn.Body.Height + fn.Body.Depth
		if i < len(fns)-1 {
			yTop -= cb.FootnoteInterSkip
		}
	}
	delete(cb.pageInserts, InsertFootnote)
	delete(cb.pageInsertHeight, InsertFootnote)
	return nil
}

// totalFootnoteHeight computes the vertical space the given footnote-class
// inserts will occupy at the bottom of a page, including separator rule, the
// skip above the rule, and inter-footnote skips. Reads cb.FootnoteSeparator*
// and cb.FootnoteInterSkip for layout.
//
// Returns 0 for an empty slice (no separator either).
func (cb *CSSBuilder) totalFootnoteHeight(fns []*Insert) bag.ScaledPoint {
	if len(fns) == 0 {
		return 0
	}
	total := cb.FootnoteSeparatorSkip + cb.FootnoteSeparatorHeight
	for i, fn := range fns {
		total += fn.Body.Height + fn.Body.Depth
		if i < len(fns)-1 {
			total += cb.FootnoteInterSkip
		}
	}
	return total
}
