package htmlbag

import (
	"unicode/utf8"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
)

// Line grid trace (CSSBuilder.TraceLineGrid, -bag-trace: linegrid): red lines
// on the baselines a region would have if all its lines were set at one
// leading, numbered from the top of the region. The same number means the
// same height across the columns of a row, and text off its line shows where
// a flow has left the grid.
//
// The grid is measured on a paragraph set like the element that defines it,
// so every leading model gives the baseline it really has. The numbers are strokes and not text:
// they need no font and stay out of the text of the document.

// lineGrid is the distance of the lines and the first baseline's distance
// from the top of a region.
type lineGrid struct {
	step, first bag.ScaledPoint
}

const (
	// lineGridOverhang is how far a line reaches past a region on both
	// sides, so that it shows next to justified text.
	lineGridOverhang = 3 * bag.Factor
	// lineGridDigitWidth and lineGridDigitHeight are the size of a digit,
	// lineGridDigitGap the distance of two.
	lineGridDigitWidth  = 18 * bag.Factor / 10
	lineGridDigitHeight = 36 * bag.Factor / 10
	lineGridDigitGap    = 9 * bag.Factor / 10
)

var traceLineGridColor = color.Color{Space: color.ColorRGB, R: 0.85, G: 0.2, B: 0.2}

// lineGridDigits holds the segments of each digit: a top, b top right,
// c bottom right, d bottom, e bottom left, f top left, g middle.
var lineGridDigits = [10]string{"abcdef", "bc", "abged", "abgcd", "fgbc", "afgcd", "afgedc", "abc", "abcdefg", "abcdfg"}

// lineGridFor returns the grid of the flow of te, nil when there is none to
// draw. The element with -bag-trace: linegrid defines it. The global switch
// and the mark on the body take the running text, as every element has a
// line-height and the body's need not be the one of the text.
func (cb *CSSBuilder) lineGridFor(te *frontend.Text) *lineGrid {
	src := findText(te, func(t *frontend.Text) bool {
		marked, _ := t.Settings[settingTraceLineGrid].(bool)
		return marked
	})
	if src == nil && !cb.TraceLineGrid {
		return nil
	}
	if body := findBody(te); src == nil || src == body {
		if src = runningText(body); src == nil {
			return nil
		}
	}
	step := paragraphLeading(src.Settings)
	if step <= 0 {
		return nil
	}
	first, err := cb.firstBaseline(src)
	if err != nil {
		bag.Logger.Warn("The line grid trace cannot measure the first baseline, leaving it out", "error", err)
		return nil
	}
	return &lineGrid{step: step, first: first}
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

// firstBaseline sets one line with the font and leading of src and returns
// the height above its baseline.
func (cb *CSSBuilder) firstBaseline(src *frontend.Text) (bag.ScaledPoint, error) {
	probe := frontend.NewText()
	for _, key := range []frontend.SettingType{
		frontend.SettingFontFamily, frontend.SettingFontFamilyStack, frontend.SettingFontWeight,
		frontend.SettingStyle, frontend.SettingSize, frontend.SettingLeading,
		frontend.SettingHalfLeading, frontend.SettingLineModel,
	} {
		if v, ok := src.Settings[key]; ok {
			probe.Settings[key] = v
		}
	}
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

// baselines returns the baselines of a region of the given height, each as
// its distance from the top.
func (g lineGrid) baselines(height bag.ScaledPoint) []bag.ScaledPoint {
	var lines []bag.ScaledPoint
	for y := g.first; y <= height; y += g.step {
		lines = append(lines, y)
	}
	return lines
}

// rule returns the hidden rule that paints the grid of a region, to be
// placed at the region's top left corner.
func (g lineGrid) rule(width, height bag.ScaledPoint) *node.Rule {
	draw := pdfdraw.NewStandalone().LineWidth(bag.ScaledPointFromFloat(0.25)).ColorStroking(traceLineGridColor)
	for i, y := range g.baselines(height) {
		draw.Moveto(-lineGridOverhang, -y).Lineto(width+lineGridOverhang, -y).Stroke()
		lineGridNumber(draw, width+lineGridOverhang+lineGridDigitGap, -y, i+1)
	}
	r := node.NewRule()
	r.Hide = true
	r.Attributes = node.H{"origin": "linegrid trace"}
	r.Pre = "q " + draw.String() + " Q"
	return r
}

// lineGridNumber draws n with its first digit at x, standing on y.
func lineGridNumber(draw *pdfdraw.Object, x, y bag.ScaledPoint, n int) {
	digits := []int{n % 10}
	for n /= 10; n > 0; n /= 10 {
		digits = append([]int{n % 10}, digits...)
	}
	for _, d := range digits {
		lineGridDigit(draw, x, y, d)
		x += lineGridDigitWidth + lineGridDigitGap
	}
}

func lineGridDigit(draw *pdfdraw.Object, x, y bag.ScaledPoint, digit int) {
	right, mid, top := x+lineGridDigitWidth, y+lineGridDigitHeight/2, y+lineGridDigitHeight
	for _, segment := range lineGridDigits[digit] {
		switch segment {
		case 'a':
			draw.Moveto(x, top).Lineto(right, top)
		case 'b':
			draw.Moveto(right, top).Lineto(right, mid)
		case 'c':
			draw.Moveto(right, mid).Lineto(right, y)
		case 'd':
			draw.Moveto(x, y).Lineto(right, y)
		case 'e':
			draw.Moveto(x, mid).Lineto(x, y)
		case 'f':
			draw.Moveto(x, top).Lineto(x, mid)
		case 'g':
			draw.Moveto(x, mid).Lineto(right, mid)
		}
		draw.Stroke()
	}
}

// traceLineGrid paints the grid of the region being handed back, whose
// height is final by now. A caller's region gets it with its box and only as
// far as the box goes: what the flow leaves of a region may go to another.
func (fc *flowCursor) traceLineGrid() {
	if fc.lineGrid == nil || fc.cur.trial {
		return
	}
	// The flow starts below the margin still open above the region.
	g := *fc.lineGrid
	g.first += max(fc.cur.marginBefore, 0)
	switch {
	case fc.cur.sink != nil:
		fc.cur.sink.lineGrid = &g
	case fc.cur.page != nil:
		vl := node.NewVList()
		vl.List = g.rule(fc.cur.width, fc.cur.height)
		fc.cur.page.OutputAt(fc.cur.left, fc.cur.top, vl)
	}
}
