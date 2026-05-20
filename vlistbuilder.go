package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"golang.org/x/net/html"
)

// CreateVlist builds a vlist (a vertical list) from the Text object.
func (cb *CSSBuilder) CreateVlist(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	vl, err := cb.buildVlistInternal(te, wd)
	if err != nil {
		return nil, err
	}
	return vl, nil
}

// isWhitespaceOnly returns true if the Text element contains only whitespace strings.
func isWhitespaceOnly(te *frontend.Text) bool {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return false
			}
		case *frontend.Text:
			if !isWhitespaceOnly(t) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (cb *CSSBuilder) buildVlistInternal(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	// If a CSS width is specified, use it instead of the inherited width.
	if sWd, ok := settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok {
			wd = ParseRelativeSize(wdStr, wd, wd)
		}
	}

	// SHALLOW extract: only direct insertMarkers in te.Items (no recursion
	// into nested *frontend.Text). Catches block-level markers that sit
	// as siblings of paragraph subtrees in a body container; inline
	// markers (footnote inside a span inside a <p>) stay nested and are
	// caught by the paragraph-branch's deep extractFootnotes below.
	inserts, err := cb.extractFootnotesShallow(te, wd)
	if err != nil {
		return nil, err
	}
	topFloats, err := cb.extractFloatsShallow(te, wd, InsertFloatTop)
	if err != nil {
		return nil, err
	}
	bottomFloats, err := cb.extractFloatsShallow(te, wd, InsertFloatBottom)
	if err != nil {
		return nil, err
	}
	inserts = append(inserts, topFloats...)
	inserts = append(inserts, bottomFloats...)

	// attachInserts is called by either branch on the resulting top-level
	// VList just before returning, so the page builder sees the inserts
	// attribute regardless of whether te was a block container or a
	// single paragraph.
	attachInserts := func(vl *node.VList) {
		if len(inserts) == 0 {
			return
		}
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["inserts"] = inserts
	}

	// Get padding-left from this element. It gets stamped onto the
	// container VList as PadLeft so the backend renderer shifts every
	// child (HList line or nested VList) to the right by that amount —
	// CSS-conformant block-content indentation.
	var paddingLeft bag.ScaledPoint
	if pl, ok := settings[frontend.SettingPaddingLeft]; ok {
		paddingLeft = pl.(bag.ScaledPoint)
	}
	// padding-right narrows the content area on the right (no shift, no
	// stamp). Without border/background the visual padding is invisible
	// but the line-break width must still respect it — otherwise a
	// `<ul>` with padding-inline-start resolved to padding-right (under
	// `direction: rtl`) renders text up to the page margin and the
	// outside marker drifts past the gutter.
	var paddingRight bag.ScaledPoint
	if pr, ok := settings[frontend.SettingPaddingRight]; ok {
		paddingRight = pr.(bag.ScaledPoint)
	}

	if isBox, ok := settings[frontend.SettingBox]; ok && isBox.(bool) {
		// PDF/UA: push a container structure element for this block
		var containerSE *document.StructureElement
		var savedStructureCurrent *document.StructureElement
		if cb.enableTagging {
			if tag, ok := settings[frontend.SettingDebug].(string); ok {
				if canonical := canonicalRoleForTag(tag); canonical != "" {
					containerSE = newSE(canonical, cb.frontend.Doc.Format)
					cb.structureCurrent.AddChild(containerSE)
					savedStructureCurrent = cb.structureCurrent
					cb.structureCurrent = containerSE
					// LI must contain LBody (PDF/UA 7.2)
					if canonical == "LI" {
						lbody := newSE("LBody", cb.frontend.Doc.Format)
						containerSE.AddChild(lbody)
						cb.structureCurrent = lbody
					}
				}
			}
		}
		// If this box container has a prepend (e.g., list bullet), pass it
		// to the first child Text element so FormatParagraph can render it.
		if prep, ok := settings[frontend.SettingPrepend]; ok {
			for _, itm := range te.Items {
				if t, ok := itm.(*frontend.Text); ok {
					t.Settings[frontend.SettingPrepend] = prep
					break
				}
			}
		}

		// Extract border/padding values for this container
		hv := settingsToHTMLValues(settings)
		hasBorderOrBg := hv.hasBorder() || hv.BackgroundColor != nil

		// Calculate effective width for children
		childBaseWidth := wd
		if hasBorderOrBg {
			// HTMLBorder will handle all padding and borders visually
			childBaseWidth = wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight
		}

		vls := node.NewVList()
		vls.Attributes = node.H{"origin": "buildVListInternal"}

		// Track previous element's margin-bottom for margin collapsing
		var prevMarginBottom bag.ScaledPoint

		for i, itm := range te.Items {
			switch t := itm.(type) {
			case *frontend.Text:
				// Skip whitespace-only text elements (e.g. whitespace
				// between </ul> and </li> in the HTML tree).
				if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
					continue
				}

				// Get margin-top of current element
				var curMarginTop bag.ScaledPoint
				if mt, ok := t.Settings[frontend.SettingMarginTop]; ok {
					curMarginTop = mt.(bag.ScaledPoint)
				}

				// Calculate collapsed margin (CSS margin collapsing)
				var marginGlue bag.ScaledPoint
				if i == 0 {
					// First element: use margin-top only
					marginGlue = curMarginTop
				} else {
					// Collapsed margin: max of previous bottom and current top
					marginGlue = bag.Max(prevMarginBottom, curMarginTop)
				}

				// Insert margin kern if needed
				if marginGlue > 0 {
					k := node.NewKern()
					k.Kern = marginGlue
					k.Attributes = node.H{"origin": "margin"}
					vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
					vls.Height += marginGlue
				}

				var vl *node.VList
				if dbg, ok := t.Settings[frontend.SettingDebug].(string); ok && dbg == "table" {
					// CSS border/padding/background declared on the <table>
					// element itself are not handled inside buildTable
					// (which only paints cell borders). Wrap the resulting
					// VList with HTMLBorder so they render around the whole
					// table (CSS 2.1 §17.6, separated borders model).
					//
					// Skip the wrap when the table uses <thead>/<tfoot>:
					// those tables go through the splittable multi-page row
					// path in the backend (driven by _headerCount), and
					// wrapping would convert the table into an opaque VList
					// that the page builder cannot split — causing tail
					// rows to be silently dropped.
					tableHv := settingsToHTMLValues(t.Settings)
					hasTableBorderOrBg := tableHv.hasBorder() || tableHv.BackgroundColor != nil
					hasTheadOrTfoot := false
					if hasTableBorderOrBg {
						for _, itm := range t.Items {
							tt, ok := itm.(*frontend.Text)
							if !ok {
								continue
							}
							if elt, _ := tt.Settings[frontend.SettingDebug].(string); elt == "thead" || elt == "tfoot" {
								hasTheadOrTfoot = true
								break
							}
						}
					}
					wrapTable := hasTableBorderOrBg && !hasTheadOrTfoot
					tableWidth := wd
					if wrapTable {
						tableWidth = wd - tableHv.BorderLeftWidth - tableHv.BorderRightWidth - tableHv.PaddingLeft - tableHv.PaddingRight
					}
					var err error
					vl, err = cb.buildTable(t, tableWidth)
					if err != nil {
						return nil, err
					}
					if wrapTable {
						vl = cb.HTMLBorder(vl, tableHv)
					}
				} else {
					// Two CSS shifts apply to every child of a block
					// container: the parent's padding-left (an offset
					// every child inherits) and the child's own
					// margin-left (a per-child offset). Both stack onto
					// the rendered VList's ShiftX. margin-right needs
					// only a width adjustment.
					var childMarginLeft, childMarginRight bag.ScaledPoint
					if ml, ok := t.Settings[frontend.SettingMarginLeft]; ok {
						childMarginLeft = ml.(bag.ScaledPoint)
					}
					if mr, ok := t.Settings[frontend.SettingMarginRight]; ok {
						childMarginRight = mr.(bag.ScaledPoint)
					}

					// Reduce the formatting width by padding-left so the
					// linebreaker builds lines that fit inside the
					// post-shift content area. HTMLBorder handles the
					// case with border/background.
					childWidth := childBaseWidth
					if !hasBorderOrBg {
						childWidth = childBaseWidth - paddingLeft - paddingRight
					}
					childWidth -= childMarginLeft + childMarginRight
					var err error
					vl, err = cb.buildVlistInternal(t, childWidth)
					if err != nil {
						return nil, err
					}
					// CSS padding-left on the container shifts every
					// child to the right. margin-left on the child
					// itself stacks on top of that.
					shift := childMarginLeft
					if !hasBorderOrBg {
						shift += paddingLeft
					}
					if shift > 0 {
						vl.ShiftX += shift
					}
				}
				// Propagate page-break-after to node attributes
				if pba, ok := t.Settings[frontend.SettingPageBreakAfter]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakAfter"] = pba
				}
				if pbb, ok := t.Settings[frontend.SettingPageBreakBefore]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakBefore"] = pbb
				}
				// page-break-inside / break-inside rides on an htmlbag-
				// private SettingType sentinel; move it to the VList's
				// Attributes and delete it from Settings so the sentinel
				// cannot leak into frontend.FormatParagraph.
				if pbi, ok := t.Settings[settingPageBreakInside]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakInside"] = pbi
					delete(t.Settings, settingPageBreakInside)
				}

				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), vl)
				if vl.Width > vls.Width {
					vls.Width = vl.Width
				}
				vls.Height += vl.Height
				vls.Depth = vl.Depth

				if cb.ElementCallback != nil {
					if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
						cb.ElementCallback(ElementEvent{
							TagName:     tag,
							TextContent: extractTextContent(t),
							VList:       vl,
						})
					}
				}

				// Annotate heading VLists so OutputPages can assign page numbers.
				if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
					switch tag {
					case "h1", "h2", "h3", "h4", "h5", "h6":
						if vl.Attributes == nil {
							vl.Attributes = node.H{}
						}
						vl.Attributes["_heading_idx"] = cb.headingCount
						entry := HeadingEntry{Level: tag, Text: extractTextContent(t)}
						if se, ok := vl.Attributes["_heading_se"].(*document.StructureElement); ok {
							entry.SE = se
						}
						cb.Headings = append(cb.Headings, entry)
						cb.headingCount++
					}
				}

				// Block-level id="..." → record as an anchor target for
				// CSS target-counter() and target-text(). A heading with
				// an id ends up in both Headings and Anchors (same page
				// assignment): intentional, the CSS reference uses the
				// id while the outline uses the heading text.
				if dest, ok := t.Settings[frontend.SettingDest].(string); ok && dest != "" {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["_anchor_idx"] = cb.anchorCount
					cb.Anchors = append(cb.Anchors, AnchorEntry{
						ID:   dest,
						Text: truncateAnchorText(extractTextContent(t)),
					})
					cb.anchorCount++
				}

				// Store margin-bottom for next iteration
				if mb, ok := t.Settings[frontend.SettingMarginBottom]; ok {
					prevMarginBottom = mb.(bag.ScaledPoint)
				} else {
					prevMarginBottom = 0
				}
			}
		}

		// Handle final margin-bottom after last element.
		if prevMarginBottom > 0 {
			if hasBorderOrBg {
				// Border/padding blocks margin collapsing: add kern.
				k := node.NewKern()
				k.Kern = prevMarginBottom
				k.Attributes = node.H{"origin": "margin-bottom"}
				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
				vls.Height += prevMarginBottom
			} else {
				// No border/padding: the last child's margin-bottom
				// collapses through the parent boundary (CSS margin
				// collapsing). Propagate the maximum to the parent.
				if mb, ok := te.Settings[frontend.SettingMarginBottom]; ok {
					parentMB := mb.(bag.ScaledPoint)
					if prevMarginBottom > parentMB {
						te.Settings[frontend.SettingMarginBottom] = prevMarginBottom
					}
				} else {
					te.Settings[frontend.SettingMarginBottom] = prevMarginBottom
				}
			}
		}

		// Apply borders/background to this block container
		if hasBorderOrBg {
			vls.Width = childBaseWidth
			// Snapshot the inner children before HTMLBorder mutates the
			// list (it prepends a bg-rule and re-wraps in hpack/vpack).
			// outputBlockSplit reuses this snapshot to fragment the block
			// across pages when it's taller than the content area.
			var splittableInner []node.Node
			for n := vls.List; n != nil; n = n.Next() {
				splittableInner = append(splittableInner, n)
			}
			splittableHv := hv
			splittableInnerWidth := childBaseWidth

			vls = cb.HTMLBorder(vls, hv)

			if len(splittableInner) > 0 {
				if vls.Attributes == nil {
					vls.Attributes = node.H{}
				}
				vls.Attributes["_splittable"] = true
				vls.Attributes["_splittableInner"] = splittableInner
				vls.Attributes["_splittableHv"] = splittableHv
				vls.Attributes["_splittableInnerWidth"] = splittableInnerWidth
			}
		}

		// PDF/UA: pop structure element back to parent
		if containerSE != nil {
			cb.structureCurrent = savedStructureCurrent
		}

		attachInserts(vls)
		return vls, nil
	}

	// Extract border/padding values first to calculate content width
	hv := settingsToHTMLValues(settings)

	// Reduce width by border and padding (CSS box-sizing: border-box behavior)
	contentWidth := wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight

	// DEEP extract: nested insertMarkers (e.g. footnote inside a span
	// inside this <p>). Top-of-function shallow only caught direct
	// te.Items, so inline markers still need a recursive pass here.
	deepFootnotes, err := cb.extractFootnotes(te, contentWidth)
	if err != nil {
		return nil, err
	}
	deepTopFloats, err := cb.extractFloats(te, contentWidth, InsertFloatTop)
	if err != nil {
		return nil, err
	}
	deepBottomFloats, err := cb.extractFloats(te, contentWidth, InsertFloatBottom)
	if err != nil {
		return nil, err
	}
	inserts = append(inserts, deepFootnotes...)
	inserts = append(inserts, deepTopFloats...)
	inserts = append(inserts, deepBottomFloats...)

	// Pull inline-anchor markers out of the Text tree before the
	// paragraph builds so they don't confuse Mknodes. The indices
	// land on the resulting VList so flushInserts can stamp the page.
	inlineAnchorIndices := extractAnchorMarkers(te)

	// Resolve any DeferredSizer-marked replaced content against the
	// contentWidth that actually reaches this leaf. Sizers were attached
	// upstream (collectHorizontalNodes / similar) when the real container
	// width was not yet known; this is the canonical materialization
	// point for block flow. The cell path materializes separately at its
	// own known cell width (Phase 2+). Sizers are idempotent, so a later
	// pass at a different width re-renders correctly.
	resolveDeferredSizing(te.Items, contentWidth)

	// If HTMLBorder will wrap this leaf (background or border set), the
	// padding-left / padding-right are applied by HTMLBorder as glue
	// around the inner vl. FormatParagraph also reads SettingPaddingLeft
	// from te.Settings and adds it as paragraph IndentLeft for every
	// line — which would double-apply the padding (once as IndentLeft
	// inside the inner vl, once as paddingLeftGlue around it). Strip
	// the padding settings here so FormatParagraph does not consume
	// them; HTMLBorder still sees them via the hv struct captured above.
	hasBorderOrBg := hv.hasBorder() || hv.BackgroundColor != nil
	if hasBorderOrBg {
		delete(te.Settings, frontend.SettingPaddingLeft)
		delete(te.Settings, frontend.SettingPaddingRight)
	}

	// Capture-and-strip settingPageBreakInside before FormatParagraph.
	// Block-level Text that only contains inline children reaches the leaf
	// branch (HTMLNodeToText leaves SettingBox off because cur flips to
	// ModeHorizontal after inline content), so the box branch above never
	// sees the sentinel for those blocks. The negative sentinel would
	// otherwise hit the strict "unknown setting" default inside
	// FormatParagraph → Mknodes → BuildNodelistFromString.
	pbi, hasPBI := te.Settings[settingPageBreakInside]
	if hasPBI {
		delete(te.Settings, settingPageBreakInside)
	}

	// FormatParagraph -> Mknodes handles SettingPrepend (e.g., bullet points).
	vl, _, err := cb.frontend.FormatParagraph(te, contentWidth)
	if err != nil {
		return nil, err
	}
	// Attach the captured page-break-inside value onto the returned VList.
	// Done after FormatParagraph (and before HTMLBorder below) so the
	// attribute sits on the outermost wrapper the paginator will see —
	// matching the shape produced by the box branch.
	if hasPBI {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["pageBreakInside"] = pbi
	}
	// Propagate page-break-before / page-break-after on leaf blocks
	// (e.g. <h1 style="break-before: page">). The box branch above does
	// this for container blocks; without the mirror here the paginator
	// never sees the forced-break attribute on simple block leaves.
	if pbb, ok := te.Settings[frontend.SettingPageBreakBefore]; ok {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["pageBreakBefore"] = pbb
	}
	if pba, ok := te.Settings[frontend.SettingPageBreakAfter]; ok {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["pageBreakAfter"] = pba
	}

	if len(inlineAnchorIndices) > 0 {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		existing, _ := vl.Attributes["_anchor_indices"].([]int)
		vl.Attributes["_anchor_indices"] = append(existing, inlineAnchorIndices...)
	}

	attachInserts(vl)

	// Apply borders if any are defined
	if hv.hasBorder() || hv.BackgroundColor != nil {
		// Snapshot the inner children (typically HList lines for <pre>)
		// before HTMLBorder mutates the list. outputBlockSplit reuses this
		// snapshot to fragment the block across pages when its wrapped
		// height exceeds the content area.
		var splittableInner []node.Node
		for n := vl.List; n != nil; n = n.Next() {
			splittableInner = append(splittableInner, n)
		}
		splittableHv := hv
		splittableInnerWidth := contentWidth

		vl = cb.HTMLBorder(vl, hv)

		if len(splittableInner) > 0 {
			if vl.Attributes == nil {
				vl.Attributes = node.H{}
			}
			vl.Attributes["_splittable"] = true
			vl.Attributes["_splittableInner"] = splittableInner
			vl.Attributes["_splittableHv"] = splittableHv
			vl.Attributes["_splittableInnerWidth"] = splittableInnerWidth
		}
	} else {
		// No border/background: still expose the line list so the paginator
		// can fragment overlong paragraphs across pages. Zero-value hv acts
		// as a sentinel for outputBlockSplit to skip HTMLBorder wrapping.
		var splittableInner []node.Node
		for n := vl.List; n != nil; n = n.Next() {
			splittableInner = append(splittableInner, n)
		}
		if len(splittableInner) > 1 {
			if vl.Attributes == nil {
				vl.Attributes = node.H{}
			}
			vl.Attributes["_splittable"] = true
			vl.Attributes["_splittableInner"] = splittableInner
			vl.Attributes["_splittableHv"] = HTMLValues{}
			vl.Attributes["_splittableInnerWidth"] = contentWidth
		}
	}

	// PDF/UA: tag leaf block elements (p, h1-h6, pre, code)
	if cb.enableTagging {
		if tag, ok := settings[frontend.SettingDebug].(string); ok {
			canonical := canonicalRoleForTag(tag)

			// If this paragraph contains an image, use Figure role with alt text
			if canonical == "P" {
				if alt := findImageAlt(te); alt != "" {
					canonical = "Figure"
				}
			}

			if canonical != "" {
				format := cb.frontend.Doc.Format
				se := newSE(canonical, format)
				if canonical == "Figure" {
					se.Alt = findImageAlt(te)
				} else {
					se.ActualText = extractTextContent(te)
				}
				// Stash heading SE on the VList so the outer caller can
				// link it to its HeadingEntry (UA-2 §8.8: outline
				// destinations must be structure destinations).
				switch canonical {
				case "H1", "H2", "H3", "H4", "H5", "H6":
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["_heading_se"] = se
				}
				// LI must contain exactly one LBody (PDF/UA 7.2)
				switch {
				case canonical == "LI":
					cb.structureCurrent.AddChild(se)
					lbody := newSE("LBody", format)
					lbody.ActualText = se.ActualText
					se.ActualText = ""
					se.AddChild(lbody)
					tagVList(vl, lbody)
				case tag == "pre":
					// PDF/UA-1 (ISO 14289-1, based on PDF 1.7 §14.8) treats
					// Code as an inline structure element — it must live
					// inside a block container, never at the block level
					// itself. Markdown fenced code blocks render as
					// <pre><code>…</code></pre>; the natural structure tree
					// is therefore P > Code > glyphs. We mirror the LI/LBody
					// pattern: the outer VList stays untouched (visual
					// layout unchanged), the StructElem hierarchy gains a
					// Code child, and the glyph-level marked content
					// attaches under Code via the inner tag.
					cb.structureCurrent.AddChild(se)
					code := newSE("Code", format)
					code.ActualText = se.ActualText
					se.ActualText = ""
					se.AddChild(code)
					tagVList(vl, code)
				case canonical == "Figure" && hasFormXObjectImage(te):
					// PDF/UA-1 §7.1 Note 1: a Figure whose entire body is a
					// Form XObject (imported PDF) attaches via /StructParent
					// on the XObject and an OBJR entry in se.K — no marked
					// content sequence on the page. This stops Acrobat's
					// tag inspector from expanding XObject path operators.
					cb.structureCurrent.AddChild(se)
					tagVListAsXObjectFigure(vl, se)
				default:
					cb.structureCurrent.AddChild(se)
					tagVList(vl, se)
				}
			}
		}
	}

	return vl, nil
}

// extractTextContent recursively collects string content from a Text tree.
func extractTextContent(te *frontend.Text) string {
	var b strings.Builder
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			b.WriteString(t)
		case *frontend.Text:
			b.WriteString(extractTextContent(t))
		}
	}
	return b.String()
}

// extractTextFromHTMLItem collects text content from an HTMLItem tree.
// Used to capture an anchor's textual representation at collection
// time, before the inline subtree has been rendered into frontend.Text.
func extractTextFromHTMLItem(item *HTMLItem) string {
	if item == nil {
		return ""
	}
	var b strings.Builder
	var walk func(it *HTMLItem)
	walk = func(it *HTMLItem) {
		if it.Typ == html.TextNode {
			b.WriteString(it.Data)
			return
		}
		for _, c := range it.Children {
			walk(c)
		}
	}
	walk(item)
	return b.String()
}

// hasFormXObjectImage reports whether the (recursively inspected) Text tree
// contains an *node.Image whose underlying Imagefile is a PDF import (Format
// == "pdf"). Only PDF imports are written as Form XObjects whose internal
// path operators trip Acrobat's tag inspector; raster images (PNG/JPEG) go
// through a /Subtype /Image XObject which Acrobat treats as atomic, so they
// can keep using the conventional MCID-on-page tagging path.
func hasFormXObjectImage(te *frontend.Text) bool {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *node.Image:
			if t.ImageFile != nil && t.ImageFile.Format == "pdf" {
				return true
			}
		case *frontend.Text:
			if hasFormXObjectImage(t) {
				return true
			}
		}
	}
	return false
}

// findImageNodeForXObjectFigure walks a VList looking for the *node.Image
// whose underlying Imagefile is a PDF import. The backend uses this at
// shipout time to attach the /StructParent index to the right Imagefile.
func findImageNodeForXObjectFigure(head node.Node) *node.Image {
	for n := head; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.Image:
			if t.ImageFile != nil && t.ImageFile.Format == "pdf" {
				return t
			}
		case *node.HList:
			if img := findImageNodeForXObjectFigure(t.List); img != nil {
				return img
			}
		case *node.VList:
			if img := findImageNodeForXObjectFigure(t.List); img != nil {
				return img
			}
		}
	}
	return nil
}

// findImageAlt checks if a Text element contains an image and returns its
// alt text. Two image carriers are recognised:
//
//   - *node.Image — produced by the raster/PDF path in inheritablestyles.go
//     (the alt attribute is stamped onto imgNode.Attributes["alt"]).
//   - *node.VList — produced by the SVG path, which wraps the SVG render
//     in a Vpack and stamps alt onto its Attributes.
//
// Returns the empty string when neither is present in the (recursively
// inspected) Text tree.
func findImageAlt(te *frontend.Text) string {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *node.Image:
			if t.Attributes != nil {
				if alt, ok := t.Attributes["alt"].(string); ok {
					return alt
				}
			}
		case *node.VList:
			if t.Attributes != nil {
				if alt, ok := t.Attributes["alt"].(string); ok {
					return alt
				}
			}
		case *frontend.Text:
			if alt := findImageAlt(t); alt != "" {
				return alt
			}
		}
	}
	return ""
}

// settingsToHTMLValues extracts border/padding/background settings into HTMLValues.
func settingsToHTMLValues(settings frontend.TypesettingSettings) HTMLValues {
	hv := HTMLValues{}

	if v, ok := settings[frontend.SettingBackgroundColor]; ok && v != nil {
		hv.BackgroundColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderTopWidth]; ok && v != nil {
		hv.BorderTopWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderRightWidth]; ok && v != nil {
		hv.BorderRightWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomWidth]; ok && v != nil {
		hv.BorderBottomWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderLeftWidth]; ok && v != nil {
		hv.BorderLeftWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderTopColor]; ok && v != nil {
		hv.BorderTopColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderRightColor]; ok && v != nil {
		hv.BorderRightColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderBottomColor]; ok && v != nil {
		hv.BorderBottomColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderLeftColor]; ok && v != nil {
		hv.BorderLeftColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderTopStyle]; ok && v != nil {
		hv.BorderTopStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderRightStyle]; ok && v != nil {
		hv.BorderRightStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderBottomStyle]; ok && v != nil {
		hv.BorderBottomStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderLeftStyle]; ok && v != nil {
		hv.BorderLeftStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderTopLeftRadius]; ok && v != nil {
		hv.BorderTopLeftRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderTopRightRadius]; ok && v != nil {
		hv.BorderTopRightRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomLeftRadius]; ok && v != nil {
		hv.BorderBottomLeftRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomRightRadius]; ok && v != nil {
		hv.BorderBottomRightRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingTop]; ok && v != nil {
		hv.PaddingTop = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingRight]; ok && v != nil {
		hv.PaddingRight = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingBottom]; ok && v != nil {
		hv.PaddingBottom = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingLeft]; ok && v != nil {
		hv.PaddingLeft = v.(bag.ScaledPoint)
	}

	return hv
}
