package htmlbag

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
)

// Line grid trace (CSSBuilder.TraceLineGrid, -bag-trace: linegrid): red lines
// on the baselines a page would have if all its lines were set at one
// leading, numbered from 1 at the top of the page area. The grid belongs to
// the page and not to the regions that fill it, so a number means the same
// height in every column and on every page, and text off its line shows
// where a flow has left the grid.
//
// A caller's regions have no page: FlowText puts the grid in the box of each
// region, as far as the box goes.
//
// The grid is measured on a paragraph set like the element that defines it,
// so every leading model gives the baseline it really has. An @page rule may
// set the distance with -bag-line-grid.

// lineGrid is the distance of the lines and the first baseline's distance
// from the top of the area they are drawn in. src is the Text that gives the
// font of the first baseline.
type lineGrid struct {
	cb          *CSSBuilder
	src         *frontend.Text
	step, first bag.ScaledPoint
}

const (
	// lineGridOverhang is how far a line reaches past an area on both sides,
	// so that it shows next to justified text. The numbers stand behind it
	// at a distance of lineGridNumberGap.
	lineGridOverhang  = 3 * bag.Factor
	lineGridNumberGap = 2 * bag.Factor
	// lineGridNumberSize is the font size of the numbers.
	lineGridNumberSize = 5 * bag.Factor
)

var traceLineGridColor = color.Color{Space: color.ColorRGB, R: 0.85, G: 0.2, B: 0.2}

// lineGridFor returns the grid of the flow of te, nil when there is none to
// draw. The element with -bag-trace: linegrid defines it. The global switch
// and the mark on the body take the running text, as every element has a
// line-height and the body's need not be the one of the text.
func (cb *CSSBuilder) lineGridFor(te *frontend.Text) *lineGrid {
	if markedForLineGrid(te) == nil && !cb.TraceLineGrid {
		return nil
	}
	src := lineGridSource(te)
	if src == nil {
		return nil
	}
	return cb.lineGridAt(src, paragraphLeading(src.Settings))
}

// markedForLineGrid returns the first Text with -bag-trace: linegrid.
func markedForLineGrid(te *frontend.Text) *frontend.Text {
	return findText(te, func(t *frontend.Text) bool {
		marked, _ := t.Settings[settingTraceLineGrid].(bool)
		return marked
	})
}

// lineGridSource returns the Text whose line-height is the grid of te: the
// one with the mark, or the running text, nil when there is no text.
func lineGridSource(te *frontend.Text) *frontend.Text {
	src, body := markedForLineGrid(te), findBody(te)
	if src == nil || src == body {
		return runningText(body)
	}
	return src
}

// lineGridAt returns the grid of step, with the first baseline a line of src
// has at that leading.
func (cb *CSSBuilder) lineGridAt(src *frontend.Text, step bag.ScaledPoint) *lineGrid {
	if step <= 0 {
		return nil
	}
	first, err := cb.firstBaseline(src, step)
	if err != nil {
		bag.Logger.Warn("The line grid trace cannot measure the first baseline, leaving it out", "error", err)
		return nil
	}
	return &lineGrid{cb: cb, src: src, step: step, first: first}
}

// runningText returns the first Text set at the leading that carries the
// most characters of te's tree, nil when it holds no text.
func runningText(te *frontend.Text) *frontend.Text {
	chars := map[bag.ScaledPoint]int{}
	first := map[bag.ScaledPoint]*frontend.Text{}
	var best bag.ScaledPoint
	var walk func(t *frontend.Text)
	walk = func(t *frontend.Text) {
		for _, itm := range t.Items {
			switch v := itm.(type) {
			case *frontend.Text:
				walk(v)
			case string:
				leading := paragraphLeading(t.Settings)
				if first[leading] == nil {
					first[leading] = t
				}
				chars[leading] += utf8.RuneCountInString(v)
				if chars[leading] > chars[best] {
					best = leading
				}
			}
		}
	}
	walk(te)
	return first[best]
}

// findText returns the first Text of te's tree that match accepts, parents
// first.
func findText(te *frontend.Text, match func(*frontend.Text) bool) *frontend.Text {
	if te == nil {
		return nil
	}
	if match(te) {
		return te
	}
	for _, itm := range te.Items {
		if t, ok := itm.(*frontend.Text); ok {
			if found := findText(t, match); found != nil {
				return found
			}
		}
	}
	return nil
}

// firstBaseline sets one line with the font of src at the given leading and
// returns the height above its baseline.
func (cb *CSSBuilder) firstBaseline(src *frontend.Text, leading bag.ScaledPoint) (bag.ScaledPoint, error) {
	probe := frontend.NewText()
	for _, key := range []frontend.SettingType{
		frontend.SettingFontFamily, frontend.SettingFontFamilyStack, frontend.SettingFontWeight,
		frontend.SettingStyle, frontend.SettingSize, frontend.SettingHalfLeading,
		frontend.SettingLineModel,
	} {
		if v, ok := src.Settings[key]; ok {
			probe.Settings[key] = v
		}
	}
	probe.Settings[frontend.SettingLeading] = leading
	probe.Items = append(probe.Items, "x")
	vl, _, err := cb.frontend.FormatParagraph(probe, bag.MustSP("100pt"))
	if err != nil {
		return 0, err
	}
	for n := node.Node(vl.List); n != nil; n = n.Next() {
		if hl, ok := n.(*node.HList); ok {
			return hl.Height, nil
		}
	}
	return 0, nil
}

// baselines returns the baselines of an area of the given height, each as its
// distance from the top.
func (g lineGrid) baselines(height bag.ScaledPoint) []bag.ScaledPoint {
	var lines []bag.ScaledPoint
	for y := g.first; y <= height; y += g.step {
		lines = append(lines, y)
	}
	return lines
}

// overlay returns the box that paints the grid of an area, to be placed at
// the area's top left corner. It takes up no room, wherever it stands.
func (g lineGrid) overlay(width, height bag.ScaledPoint) *node.VList {
	lines := g.baselines(height)
	vl := node.NewVList()
	vl.Attributes = node.H{"origin": "linegrid trace", "artifact": document.ArtifactLayout}
	tail := node.Node(g.rule(width, lines))
	vl.List = tail
	var cursor bag.ScaledPoint
	for i, y := range lines {
		number, err := g.number(i+1, width+lineGridOverhang+lineGridNumberGap)
		if err != nil {
			bag.Logger.Warn("The line grid trace cannot set its numbers, leaving them out", "error", err)
			break
		}
		k := node.NewKern()
		k.Kern = y - number.Height - cursor
		vl.List = node.InsertAfter(vl.List, tail, k)
		vl.List = node.InsertAfter(vl.List, k, number)
		tail = number
		cursor = y + number.Depth
	}
	return vl
}

// rule returns the hidden rule that paints the lines.
func (g lineGrid) rule(width bag.ScaledPoint, lines []bag.ScaledPoint) *node.Rule {
	draw := pdfdraw.NewStandalone().LineWidth(bag.ScaledPointFromFloat(0.25)).ColorStroking(traceLineGridColor)
	for _, y := range lines {
		draw.Moveto(-lineGridOverhang, -y).Lineto(width+lineGridOverhang, -y).Stroke()
	}
	r := node.NewRule()
	r.Hide = true
	r.Attributes = node.H{"origin": "linegrid trace"}
	r.Pre = "q " + draw.String() + " Q"
	return r
}

// number sets n at x.
func (g lineGrid) number(n int, x bag.ScaledPoint) (*node.HList, error) {
	return g.label(strconv.Itoa(n), x)
}

// label sets text at x, in the monospace font every builder has.
func (g lineGrid) label(text string, x bag.ScaledPoint) (*node.HList, error) {
	fe := g.cb.frontend
	te := frontend.NewText()
	te.Settings[frontend.SettingFontFamily] = fe.FindFontFamily("monospace")
	te.Settings[frontend.SettingSize] = lineGridNumberSize
	te.Settings[frontend.SettingLeading] = lineGridNumberSize
	te.Settings[frontend.SettingColor] = &traceLineGridColor
	te.Items = append(te.Items, text)
	vl, _, err := fe.FormatParagraph(te, 10*lineGridNumberSize)
	if err != nil {
		return nil, err
	}
	k := node.NewKern()
	k.Kern = x
	k.SetNext(vl.List)
	return node.Hpack(k), nil
}

// traceLineGridOnPage paints the grid of the current page, whatever regions
// it was filled through.
func (cb *CSSBuilder) traceLineGridOnPage() error {
	g := cb.pageLineGrid
	if g == nil {
		return nil
	}
	if cb.pageLineStep > 0 && cb.pageLineStep != g.step {
		if g = cb.lineGridAt(g.src, cb.pageLineStep); g == nil {
			return nil
		}
	}
	reg, err := cb.pageRegion()
	if err != nil {
		return err
	}
	reg.page.OutputAt(reg.left, reg.top, g.overlay(reg.width, reg.height))
	cb.traceSnapMarks(g, reg.page)
	return nil
}

// lineStep reads -bag-line-grid of the @page rule p, 0 without a valid one.
func (cb *CSSBuilder) lineStep(p *Page) bag.ScaledPoint {
	v, ok := resolveDeclarations(p.Attributes)["-bag-line-grid"]
	if !ok {
		return 0
	}
	value := strings.TrimSpace(v.String())
	step, err := bag.SP(value)
	if err != nil || step <= 0 {
		if cb.css.warnOnce("@page -bag-line-grid " + value) {
			bag.Logger.Warn("@page -bag-line-grid: not a length above zero, ignoring it", "value", value)
		}
		return 0
	}
	return step
}

// traceLineGrid hands the grid to the caller's region being handed back,
// which paints it in its box as far as the box goes: what the flow leaves of
// a region may go to another.
func (fc *flowCursor) traceLineGrid() {
	if fc.lineGrid == nil || fc.cur.trial || fc.cur.sink == nil {
		return
	}
	// The flow starts below the margin still open above the region.
	g := *fc.lineGrid
	g.first += max(fc.cur.marginBefore, 0)
	fc.cur.sink.lineGrid = &g
}
