package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// lineAcross is where a line's text sits: from x0 to x1 across, at y down from
// the top of the tree.
type lineAcross struct{ x0, x1, y bag.ScaledPoint }

// linesAcross returns every line in the tree with its text's position, the
// shifts of the boxes around it and the HTMLBorder glue beside them included.
// Line-end glue counts as room, so the lines have to be justified for x1 to
// be the end of the measure they were given.
func linesAcross(v *node.VList) []lineAcross {
	var out []lineAcross
	var walk func(n node.Node, x, y bag.ScaledPoint)
	walk = func(n node.Node, x, y bag.ScaledPoint) {
		switch c := n.(type) {
		case *node.VList:
			if origin, _ := c.Attributes["origin"].(string); origin == "float" {
				return
			}
			cy := y
			for e := c.List; e != nil; e = e.Next() {
				var sx, h, d bag.ScaledPoint
				switch k := e.(type) {
				case *node.VList:
					sx, h, d = k.ShiftX, k.Height, k.Depth
				case *node.HList:
					h, d = k.Height, k.Depth
				case *node.Kern:
					h = k.Kern
				case *node.Glue:
					h = k.Width
				case *node.Rule:
					h, d = k.Height, k.Depth
				}
				walk(e, x+sx, cy)
				cy += h + d
			}
		case *node.HList:
			if origin, _ := c.Attributes["origin"].(string); origin == "line" {
				var left, right bag.ScaledPoint
				for m := c.List; m != nil; m = m.Next() {
					if g, ok := m.(*node.Glue); ok {
						switch g.Attributes["origin"] {
						case "leftskip":
							left += g.Width
						case "lineend":
							right += g.Width
						}
					}
				}
				out = append(out, lineAcross{x + left, x + c.Width - right, y})
				return
			}
			cx := x
			for e := c.List; e != nil; e = e.Next() {
				w, _, _ := node.Dimensions(e, e, node.Horizontal)
				if vl, ok := e.(*node.VList); ok {
					walk(e, cx+vl.ShiftX, y+c.Height-vl.Height)
				} else {
					walk(e, cx, y)
				}
				cx += w
			}
		}
	}
	walk(v, 0, 0)
	return out
}

// A float shortens line boxes, not the blocks they are in (CSS 2.1 §9.5): a
// line keeps clear of the float's edge however far in its box starts, a box
// with a border or background runs on under the float, and a box that starts
// past the float is not narrowed at all. The space at the top of a box, its
// border and padding, is beside the float already, so the band covers fewer
// lines. The values are Chrome's for the same documents: a 60pt by 40pt float
// in a 200pt measure, lines of 12pt, no gutter. narrowed counts the lines that
// do not span what the last full line spans.
func TestABandIsMeasuredFromTheFloatsEdge(t *testing.T) {
	for _, c := range []struct {
		name, side, body string
		first            [2]string // the first line's text, from and to
		narrowed         int
		full             [2]string // a line below the float
	}{
		{"block with a border", "left", `<div style="border:1pt solid black;padding:5pt"><p>P</p></div>`, [2]string{"60pt", "194pt"}, 3, [2]string{"6pt", "194pt"}},
		{"block with a background", "left", `<div style="background-color:#eee"><p>P</p></div>`, [2]string{"60pt", "200pt"}, 4, [2]string{"0pt", "200pt"}},
		{"block with a margin and a border", "left", `<div style="margin-left:10pt;border:1pt solid black;padding:5pt"><p>P</p></div>`, [2]string{"60pt", "194pt"}, 3, [2]string{"16pt", "194pt"}},
		{"bare block with padding", "left", `<div style="padding-left:20pt"><p>P</p></div>`, [2]string{"60pt", "200pt"}, 4, [2]string{"20pt", "200pt"}},
		{"nested bare blocks with padding", "left", `<div style="padding-left:10pt"><div style="padding-left:10pt"><p>P</p></div></div>`, [2]string{"60pt", "200pt"}, 4, [2]string{"20pt", "200pt"}},
		{"paragraph with a margin", "left", `<p style="margin-left:20pt">P</p>`, [2]string{"60pt", "200pt"}, 4, [2]string{"20pt", "200pt"}},
		{"paragraph past the float", "left", `<p style="margin-left:80pt">P</p>`, [2]string{"80pt", "200pt"}, 0, [2]string{"80pt", "200pt"}},
		{"paragraph with a border", "left", `<p style="border:1pt solid black;padding:5pt">P</p>`, [2]string{"60pt", "194pt"}, 3, [2]string{"6pt", "194pt"}},
		{"paragraph with a background", "left", `<p style="background-color:#eee">P</p>`, [2]string{"60pt", "200pt"}, 4, [2]string{"0pt", "200pt"}},
		{"right float, block with a border", "right", `<div style="border:1pt solid black;padding:5pt"><p>P</p></div>`, [2]string{"6pt", "140pt"}, 3, [2]string{"6pt", "194pt"}},
		{"right float, bare block with padding", "right", `<div style="padding-right:20pt"><p>P</p></div>`, [2]string{"0pt", "140pt"}, 4, [2]string{"0pt", "180pt"}},
		{"padding on top of a bordered block", "left", `<div style="border:1pt solid black;padding:20pt 5pt 5pt 5pt"><p>P</p></div>`, [2]string{"60pt", "194pt"}, 2, [2]string{"6pt", "194pt"}},
		{"padding on top of a bare block", "left", `<div style="padding-top:20pt"><p>P</p></div>`, [2]string{"60pt", "200pt"}, 2, [2]string{"0pt", "200pt"}},
		{"float in a block with padding", "left", `<div style="padding-left:20pt">FLOAT<p>P</p></div>`, [2]string{"80pt", "200pt"}, 4, [2]string{"20pt", "200pt"}},
		{"right float, paragraph with padding", "right", `<p style="padding-right:20pt">P</p>`, [2]string{"0pt", "140pt"}, 4, [2]string{"0pt", "180pt"}},
		{"flow-root", "left", `<div style="display:flow-root;background-color:#eee"><p>P</p></div>`, [2]string{"60pt", "200pt"}, 0, [2]string{"60pt", "200pt"}},
		{"flow-root paragraph", "left", `<p style="display:flow-root;background-color:#eee">P</p>`, [2]string{"60pt", "200pt"}, 0, [2]string{"60pt", "200pt"}},
		{"flow-root past the float", "left", `<div style="display:flow-root;margin-left:80pt"><p>P</p></div>`, [2]string{"80pt", "200pt"}, 0, [2]string{"80pt", "200pt"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb := floatBuilder(t)
			if err := cb.AddCSS("p { margin: 0; font-size: 10pt; line-height: 12pt; text-align: justify } * { -bag-float-gutter: 0 }"); err != nil {
				t.Fatal(err)
			}
			float := `<div style="float:` + c.side + `;width:60pt;height:40pt"></div>`
			body := strings.Replace(c.body, ">P<", ">"+floatProse+"<", 1)
			if strings.Contains(body, "FLOAT") {
				body = strings.Replace(body, "FLOAT", float, 1)
			} else {
				body = float + body
			}
			lines := linesAcross(buildHTML(t, cb, `<div>`+body+`</div>`))
			if len(lines) < 6 {
				t.Fatalf("%d lines, too few to pass the float", len(lines))
			}
			// The last line is ragged; the one before it spans the measure.
			full := lines[len(lines)-2]
			narrowed := 0
			for _, l := range lines[:len(lines)-1] {
				if l.x0 != full.x0 || l.x1 != full.x1 {
					narrowed++
				}
			}
			if got, want := [2]bag.ScaledPoint{lines[0].x0, lines[0].x1}, [2]bag.ScaledPoint{bag.MustSP(c.first[0]), bag.MustSP(c.first[1])}; got != want {
				t.Errorf("the first line runs from %s to %s, want %s to %s", got[0], got[1], want[0], want[1])
			}
			if got, want := [2]bag.ScaledPoint{full.x0, full.x1}, [2]bag.ScaledPoint{bag.MustSP(c.full[0]), bag.MustSP(c.full[1])}; got != want {
				t.Errorf("a line below the float runs from %s to %s, want %s to %s", got[0], got[1], want[0], want[1])
			}
			if narrowed != c.narrowed {
				t.Errorf("%d lines are narrowed, want %d", narrowed, c.narrowed)
			}
		})
	}
}

// A float in a block with padding sits at the padding edge, where the
// block's content starts, on either side.
func TestAFloatSitsInsideItsBlocksPadding(t *testing.T) {
	for _, c := range []struct{ side, padding, want string }{
		{"left", "padding-left:20pt", "20pt"},
		{"right", "padding-right:20pt", "120pt"},
	} {
		vl := buildHTML(t, floatBuilder(t), `<div><div style="`+c.padding+`"><div style="float:`+c.side+
			`;width:60pt;height:40pt"></div><p>`+floatProse+`</p></div></div>`)
		var x bag.ScaledPoint
		found := false
		var walk func(n node.Node, x0 bag.ScaledPoint)
		walk = func(n node.Node, x0 bag.ScaledPoint) {
			for e := n; e != nil && !found; e = e.Next() {
				if v, ok := e.(*node.VList); ok {
					if origin, _ := v.Attributes["origin"].(string); origin == "float" {
						x, found = x0+v.ShiftX, true
						return
					}
					walk(v.List, x0+v.ShiftX)
				}
			}
		}
		walk(vl.List, 0)
		if !found {
			t.Fatalf("%s: no float", c.side)
		}
		if want := bag.MustSP(c.want); x != want {
			t.Errorf("a %s float in a block with %s sits at %s, want %s", c.side, c.padding, x, want)
		}
	}
}

// A float before the first child of a bare block stands level with that
// child, below its top margin: in a browser the margin collapses through the
// block and takes the float down with it. Set above the margin, a short float
// would end before the first line and narrow nothing.
func TestALeadingFloatStandsLevelWithTheFirstChild(t *testing.T) {
	cb := floatBuilder(t)
	if err := cb.AddCSS("p { font-size: 10pt; line-height: 12pt; text-align: justify } * { -bag-float-gutter: 0 }"); err != nil {
		t.Fatal(err)
	}
	lines := linesAcross(buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:12pt"></div><p style="margin-top:10pt">`+floatProse+`</p></div>`))
	if len(lines) < 2 {
		t.Fatal("too few lines")
	}
	if want := bag.MustSP("60pt"); lines[0].x0 != want {
		t.Errorf("the first line starts at %s, want %s beside the float", lines[0].x0, want)
	}
	if lines[1].x0 != 0 {
		t.Errorf("the second line starts at %s, want 0 below a float one line high", lines[1].x0)
	}
}

// A paragraph's padding-left without a border is the indent of its lines. A
// float narrower than that leaves them where they are: the float's edge is
// short of the content edge, as in a browser.
func TestAFloatInsideAParagraphsPaddingLeavesItsLines(t *testing.T) {
	cb := floatBuilder(t)
	if err := cb.AddCSS("p { margin: 0; font-size: 10pt; line-height: 12pt; text-align: justify } * { -bag-float-gutter: 0 }"); err != nil {
		t.Fatal(err)
	}
	lines := linesAcross(buildHTML(t, cb, `<div><div style="float:left;width:10pt;height:40pt"></div><p style="padding-left:20pt">`+floatProse+`</p></div>`))
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	if want := bag.MustSP("20pt"); lines[0].x0 != want {
		t.Errorf("the first line starts at %s, want %s at the padding", lines[0].x0, want)
	}
}

// A percentage width on a float refers to the content box of its block, not
// to the box with its padding.
func TestAFloatsPercentWidthIsOfTheContentBox(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div><div style="padding-left:40pt"><div style="float:left;width:50%;height:20pt"></div><p>`+floatProse+`</p></div></div>`)
	box := floatBox(vl)
	if box == nil {
		t.Fatal("no float")
	}
	if want := bag.MustSP("80pt"); box.Width != want {
		t.Errorf("the float is %s wide, want %s, half of the 160pt content box", box.Width, want)
	}
}

// An outside float in a block with padding keeps to the content box when it
// follows the page it is painted on.
func TestALogicalFloatInABlockWithPaddingFollowsThePage(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div><div style="padding-left:20pt;padding-right:10pt"><div style="float:outside;width:40pt;height:20pt;margin-left:10pt;margin-right:-50pt"></div><p>`+floatProse+`</p></div></div>`)
	boxes := logicalFloatBoxes(&document.Page{Objects: []document.Object{{Vlist: vl}}})
	if len(boxes) != 1 {
		t.Fatalf("got %d logical float boxes, want 1", len(boxes))
	}
	box := boxes[0]
	// The content box starts at 20pt and is 170pt wide.
	right, left := bag.MustSP("20pt")+bag.MustSP("170pt")-bag.MustSP("40pt")+bag.MustSP("50pt"), bag.MustSP("20pt")-bag.MustSP("50pt")
	if box.ShiftX != right {
		t.Fatalf("built for a right page: shift %s, want %s", box.ShiftX, right)
	}
	fixLogicalFloats(vl, false)
	if box.ShiftX != left {
		t.Errorf("painted on a left page: shift %s, want %s", box.ShiftX, left)
	}
}
