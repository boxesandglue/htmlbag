package htmlbag

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/PuerkitoBio/goquery"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

var onecm = bag.MustSP("1cm")

// HeadingEntry records a heading found during VList construction.
// The Page field is filled later during OutputPages when the heading
// is placed on a page.
type HeadingEntry struct {
	Level string // "h1", "h2", etc.
	Text  string
	Page  int // 1-based page number, 0 until assigned
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
}

// New creates an instance of the CSSBuilder.
func New(fd *frontend.Document, c *csshtml.CSS) (*CSSBuilder, error) {
	cb := CSSBuilder{
		css:         c,
		frontend:    fd,
		stylesStack: make(StylesStack, 0),
		pagebox:     []node.Node{},
		Counters:    map[string]int{},
	}
	if err := LoadIncludedFonts(fd); err != nil {
		return nil, err
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
	if err := cb.BeforeShipout(); err != nil {
		return err
	}
	cb.frontend.Doc.CurrentPage.Shipout()
	cb.frontend.Doc.NewPage()
	// Store page dimensions on the new page for callback access.
	if pd, err := cb.PageSize(); err == nil {
		storePageDimensions(cb, pd)
	}
	cb.firePageInit()
	return nil
}

func (cb *CSSBuilder) firePageInit() {
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

	// Unwrap nested single-child VLists (html > body > content).
	contentList := vl.List
	contentWidth := vl.Width
	for {
		inner, ok := contentList.(*node.VList)
		if !ok || inner.Next() != nil {
			break
		}
		contentList = inner.List
		if inner.Width > 0 {
			contentWidth = inner.Width
		}
	}

	// Store page dimensions as Userdata on the current page so callbacks
	// can access margins.
	storePageDimensions(cb, pd)

	yStart := pd.Height - pd.MarginTop
	yLimit := pd.MarginBottom
	y := yStart
	pageHasContent := false // true once a VList/HList has been placed on the page
	cur := contentList

	for cur != nil {
		next := cur.Next()
		h := vlistNodeHeight(cur)

		// page-break-before: always — force a new page before this node
		// (but not if the page has no real content yet).
		if forceBreakBefore(cur) && pageHasContent {
			if err := cb.NewPage(); err != nil {
				return err
			}
			y = yStart
			pageHasContent = false
		}

		// page-break-after: avoid — if this node has the attribute, look
		// ahead at the next nodes (typically kern + following block) and
		// break before this node if all of them wouldn't fit.
		if avoidBreakAfter(cur) && next != nil {
			peekH := h
			// Add the next node (usually a margin kern)
			peekH += vlistNodeHeight(next)
			// Add the node after that (the actual following content)
			if nn := next.Next(); nn != nil {
				peekH += vlistNodeHeight(nn)
			}
			if y-peekH < yLimit && pageHasContent {
				if err := cb.NewPage(); err != nil {
					return err
				}
				y = yStart
				pageHasContent = false
			}
		}

		// Start a new page if this node would overflow (but not on an empty page).
		if y-h < yLimit && pageHasContent {
			if err := cb.NewPage(); err != nil {
				return err
			}
			y = yStart
			pageHasContent = false
		}

		// Detach the node from the original list.
		cur.SetPrev(nil)
		cur.SetNext(nil)

		// Wrap in a VList for OutputAt.
		box := node.NewVList()
		box.List = cur
		box.Width = contentWidth
		box.Height = h

		cb.frontend.Doc.CurrentPage.OutputAt(pd.MarginLeft, y, box)
		y -= h

		// Assign page number to heading if this node carries a heading index.
		if vl, ok := cur.(*node.VList); ok && vl.Attributes != nil {
			if idx, ok := vl.Attributes["_heading_idx"].(int); ok && idx < len(cb.Headings) {
				cb.Headings[idx].Page = len(cb.frontend.Doc.Pages)
			}
		}

		// Track whether real content (not just spacing) has been placed.
		switch cur.(type) {
		case *node.VList, *node.HList:
			pageHasContent = true
		}

		// break-after: always — force a new page after this node
		// (but only if there is more content to come).
		if forceBreakAfter(cur) && next != nil {
			if err := cb.NewPage(); err != nil {
				return err
			}
			y = yStart
			pageHasContent = false
		}

		cur = next
	}

	if err := cb.BeforeShipout(); err != nil {
		return err
	}
	cb.frontend.Doc.CurrentPage.Shipout()
	return nil
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

// forceBreakAfter checks if a node has the break-after: always attribute.
func forceBreakAfter(n node.Node) bool {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakAfter"]; ok {
			return v == "always"
		}
	}
	return false
}

// forceBreakBefore checks if a node has the break-before: always attribute.
func forceBreakBefore(n node.Node) bool {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		if v, ok := vl.Attributes["pageBreakBefore"]; ok {
			return v == "always"
		}
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

// ParseHTMLFromNode interprets the HTML structure and applies all previously read CSS data.
func (cb *CSSBuilder) ParseHTMLFromNode(input *html.Node) (*frontend.Text, error) {
	doc := goquery.NewDocumentFromNode(input)
	gq, err := cb.css.ApplyCSS(doc)
	if err != nil {
		return nil, err
	}
	var te *frontend.Text
	n := gq.Nodes[0]
	if te, err = HTMLNodeToText(n, cb.stylesStack, cb.frontend); err != nil {
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

	var te *frontend.Text
	if te, err = HTMLNodeToText(n, cb.stylesStack, cb.frontend); err != nil {
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
