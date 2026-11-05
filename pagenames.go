package htmlbag

import (
	"cmp"
	"maps"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// The page property sets a block on pages of a name, which the @page rule of
// that name applies to (CSS Paged Media 3 §6). The value is not inherited, but
// a block with page: auto is set on the pages of its parent. Where the name
// changes between two neighboring blocks, the page breaks. An automatic break
// keeps the name of the page before it.
//
// A change of name often comes with another page width, and what is built
// already can only partly be set again at a new width. So resolvePageNames
// works on the Text tree before anything is built: it cuts the blocks around a
// change in two, which leaves every change between two children of the body
// with a forced break before the second one. Each part of the flow is built
// for its page from then on.

// startSettings describe the start of a block, endSettings its end. The parts
// of a cut block keep them on one side.
var (
	startSettings = []frontend.SettingType{
		frontend.SettingMarginTop, frontend.SettingPageBreakBefore, frontend.SettingDest, frontend.SettingElementID,
		settingBookmark, settingCSSHeight,
	}
	endSettings = []frontend.SettingType{frontend.SettingMarginBottom, frontend.SettingPageBreakAfter}
	// topEdge and bottomEdge are the sides a cut takes off, as a block that
	// splits with box-decoration-break: slice loses them.
	topEdge = []frontend.SettingType{
		frontend.SettingPaddingTop, frontend.SettingBorderTopWidth,
		frontend.SettingBorderTopLeftRadius, frontend.SettingBorderTopRightRadius,
	}
	bottomEdge = []frontend.SettingType{
		frontend.SettingPaddingBottom, frontend.SettingBorderBottomWidth,
		frontend.SettingBorderBottomLeftRadius, frontend.SettingBorderBottomRightRadius,
	}
)

// pagePart is a block, or a part of a cut one, with its start and end page
// value: the names of the pages its first and its last block are set on.
type pagePart struct {
	t          *frontend.Text
	start, end string
}

// notePageName records the page property of the block te is built from.
func (cb *CSSBuilder) notePageName(te *frontend.Text, item *HTMLItem) {
	v, ok := item.Styles["page"]
	if !ok {
		return
	}
	name := strings.TrimSpace(v.String())
	if name == "" || name == "auto" {
		return
	}
	if cb.pageNames == nil {
		cb.pageNames = map[*frontend.Text]string{}
	}
	cb.pageNames[te] = name
}

// resolvePageNames puts the blocks of te with a page property in place for the
// flow and names the first page. A document without the property is left alone.
func (cb *CSSBuilder) resolvePageNames(te *frontend.Text) error {
	if len(cb.pageNames) == 0 {
		return nil
	}
	body := findBody(te)
	name := ""
	for t := te; ; t = t.Items[0].(*frontend.Text) {
		if n, ok := cb.pageNames[t]; ok {
			name = n
		}
		if t == body {
			break
		}
	}
	// The body stays in one piece: its parts hand their children back to it,
	// each one that changes the name with a break before it.
	settings := maps.Clone(body.Settings)
	parts := cb.pageParts(body, name)
	body.Settings = settings
	cb.pageChanges = map[*frontend.Text]string{}
	items := parts[0].t.Items
	for _, p := range parts[1:] {
		first := p.t.Items[0].(*frontend.Text)
		cb.pageChanges[first] = p.start
		first.Settings[frontend.SettingPageBreakBefore] = cmp.Or(textBreakBefore(first, breakKeyword), "page")
		items = append(items, p.t.Items...)
	}
	body.Items = items
	cb.pageNames = nil
	return cb.nameFirstPage(parts[0].start)
}

// splitAtPageNames cuts t where the page name changes inside it. inherited is
// the name of the pages its parent is set on. A table and the columns of a
// block stay whole, whatever the page property of their parts says.
func (cb *CSSBuilder) splitAtPageNames(t *frontend.Text, inherited string) []pagePart {
	if name, ok := cb.pageNames[t]; ok {
		inherited = name
	}
	tag, _ := t.Settings[frontend.SettingDebug].(string)
	if _, columns := cb.multicols[t]; columns || tag == "table" {
		return []pagePart{{t, inherited, inherited}}
	}
	return cb.pageParts(t, inherited)
}

// pageParts cuts the children of t, set on pages named used unless they say
// otherwise, into parts of one page name each. Loose text has no block a break
// could go on: it stays on the page of the block before it.
func (cb *CSSBuilder) pageParts(t *frontend.Text, used string) []pagePart {
	var parts []pagePart
	cur := pagePart{t, used, used}
	var items []any
	placed := false
	for _, itm := range t.Items {
		child, ok := itm.(*frontend.Text)
		if ok {
			_, isFloat := child.Settings[settingFloat]
			ok = !isFloat && !isWhitespaceOnly(child)
		}
		if !ok {
			items = append(items, itm)
			if s, isText := itm.(string); isText && strings.TrimSpace(s) != "" {
				placed = true
			}
			continue
		}
		for _, p := range cb.splitAtPageNames(child, used) {
			switch {
			case !placed:
				cur.start = p.start
			case p.start != cur.end:
				rest := cb.cutForPageName(cur.t)
				cur.t.Items = items
				parts = append(parts, cur)
				cur, items = pagePart{t: rest, start: p.start}, nil
			}
			items = append(items, p.t)
			cur.end, placed = p.end, true
		}
	}
	cur.t.Items = items
	return append(parts, cur)
}

// cutForPageName ends the block t where it is cut and returns the block that
// goes on after the cut, which holds none of t's items yet.
func (cb *CSSBuilder) cutForPageName(t *frontend.Text) *frontend.Text {
	rest := &frontend.Text{Settings: maps.Clone(t.Settings)}
	dropSettings(rest.Settings, startSettings)
	dropSettings(t.Settings, endSettings)
	if cb.clones[t] {
		cb.clones[rest] = true
	} else {
		dropSettings(rest.Settings, topEdge)
		dropSettings(t.Settings, bottomEdge)
	}
	return rest
}

func dropSettings(s frontend.TypesettingSettings, keys []frontend.SettingType) {
	for _, k := range keys {
		delete(s, k)
	}
}

// nameFirstPage gives the first page of the flow the name of the first block.
// HTMLToText has started that page if the document has a positioned element,
// and as nothing is on it yet, it starts over. A page with content stays as it
// is, and the name holds from the next page on.
func (cb *CSSBuilder) nameFirstPage(name string) error {
	cb.pageName = name
	doc := cb.frontend.Doc
	if name == "" || doc.CurrentPage == nil {
		return nil
	}
	if len(doc.Pages) != 1 || len(doc.CurrentPage.Objects) != cb.pageStartObjects {
		bag.Logger.Warn("page: the current page has content, the name applies from the next page on", "page", name)
		return nil
	}
	doc.Pages, doc.CurrentPage = nil, nil
	return cb.startPage()
}

// followPageChange takes the name of the page that t starts, if t changes it.
func (cb *CSSBuilder) followPageChange(t *frontend.Text) {
	if name, ok := cb.pageChanges[t]; ok {
		cb.pageName = name
	}
}

// clearPageNames ends the page names of a flow: pages after it are unnamed.
func (cb *CSSBuilder) clearPageNames() {
	cb.pageName, cb.pageNames, cb.pageChanges = "", nil, nil
}
