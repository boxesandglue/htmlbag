package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// relativeFirstLine renders html and returns its first line.
func relativeFirstLine(t *testing.T, html string) *node.HList {
	t.Helper()
	return relativeFirstLineCSS(t, "", html)
}

func relativeFirstLineCSS(t *testing.T, css, html string) *node.HList {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if css != "" {
		if err = cb.AddCSS(css); err != nil {
			t.Fatal(err)
		}
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("200pt"))
	if err != nil {
		t.Fatal(err)
	}
	var line *node.HList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil && line == nil; e = e.Next() {
			switch v := e.(type) {
			case *node.VList:
				walk(v.List)
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin == "line" {
					line = v
				} else {
					walk(v.List)
				}
			}
		}
	}
	walk(vl.List)
	if line == nil {
		t.Fatal("no line")
	}
	return line
}

// glyphOffsets returns the YOffset of each glyph of the first line, by
// codepoint.
func glyphOffsets(t *testing.T, html string) (map[rune]bag.ScaledPoint, *node.HList) {
	t.Helper()
	return glyphOffsetsCSS(t, "", html)
}

func glyphOffsetsCSS(t *testing.T, css, html string) (map[rune]bag.ScaledPoint, *node.HList) {
	t.Helper()
	line := relativeFirstLineCSS(t, css, html)
	offsets := map[rune]bag.ScaledPoint{}
	for n := line.List; n != nil; n = n.Next() {
		if g, ok := n.(*node.Glyph); ok {
			for _, r := range g.Components {
				offsets[r] = g.YOffset
			}
		}
	}
	return offsets, line
}

func TestARelativeInlineMovesByTopOrBottom(t *testing.T) {
	for _, tc := range []struct {
		style string
		want  bag.ScaledPoint
	}{
		{"position: relative; top: -3pt", bag.MustSP("3pt")},
		{"position: relative; top: 2pt", bag.MustSP("-2pt")},
		{"position: relative; bottom: 2pt", bag.MustSP("2pt")},
		{"position: relative; top: 1pt; bottom: 5pt", bag.MustSP("-1pt")}, // top wins
		{"position: relative; top: auto; bottom: 1pt", bag.MustSP("1pt")},
		{"position: relative; font-size: 5pt; top: -1em", bag.MustSP("5pt")}, // its own size
		{"position: relative; top: -50%", 0},                                 // no containing block height
		{"position: relative; left: 4pt", 0},
		{"position: static; top: -3pt", 0},
		{"top: -3pt", 0},
	} {
		offsets, _ := glyphOffsets(t, `<p>a <span style="`+tc.style+`">x</span> b</p>`)
		if got := offsets['x']; got != tc.want {
			t.Errorf("%s: x is moved by %s, want %s", tc.style, got, tc.want)
		}
		if offsets['a'] != 0 || offsets['b'] != 0 {
			t.Errorf("%s: the neighbours moved (%s, %s)", tc.style, offsets['a'], offsets['b'])
		}
	}
}

func TestNestedRelativeInlinesAddUp(t *testing.T) {
	offsets, _ := glyphOffsets(t, `<p>a <span style="position: relative; top: -2pt">x<span style="position: relative; bottom: 1pt">y</span><span style="vertical-align: 3pt">z</span></span></p>`)
	for r, want := range map[rune]bag.ScaledPoint{'x': bag.MustSP("2pt"), 'y': bag.MustSP("3pt"), 'z': bag.MustSP("5pt")} {
		if offsets[r] != want {
			t.Errorf("%c is moved by %s, want %s", r, offsets[r], want)
		}
	}
}

// Generated content belongs to the element's box and moves with it.
func TestARelativeInlineMovesItsGeneratedContent(t *testing.T) {
	offsets, _ := glyphOffsetsCSS(t, `.q::before { content: "Q" } .q::after { content: "W" }`,
		`<p>a <span class="q" style="position: relative; top: -2pt">x</span></p>`)
	for _, r := range "QxW" {
		if offsets[r] != bag.MustSP("2pt") {
			t.Errorf("%c is moved by %s, want 2pt", r, offsets[r])
		}
	}
}

// The offset is applied after the lines are set, so the line box is the
// one the text would have without it.
func TestARelativeInlineLeavesTheLineAlone(t *testing.T) {
	_, plain := glyphOffsets(t, `<p>a <span>x</span> b</p>`)
	for _, style := range []string{"top: -8pt", "bottom: -8pt"} {
		_, moved := glyphOffsets(t, `<p>a <span style="position: relative; `+style+`">x</span> b</p>`)
		if moved.Height != plain.Height || moved.Depth != plain.Depth || moved.Width != plain.Width {
			t.Errorf("%s: line is %s + %s, %s wide; without the offset %s + %s, %s wide", style, moved.Height, moved.Depth, moved.Width, plain.Height, plain.Depth, plain.Width)
		}
	}
}

// Under a registered model a relative inline inside a vertical-align span
// moves by both, and only the vertical-align part reaches the model.
func TestARelativeInlineInsideAVerticalAlignUnderARegisteredModel(t *testing.T) {
	var seen bag.ScaledPoint
	register := func(cb *CSSBuilder) {
		_ = cb.RegisterLineModel("fixed", func(LineModelStyles) node.LineModel {
			return fixedModel{&seen}
		})
	}
	line := lineModelLines(t, `p { -bag-leading-model: fixed }`,
		`<p>a <span style="vertical-align: 3pt">b<span style="position: relative; top: -2pt">c</span></span></p>`, register)[0]
	var c *node.Glyph
	for n := line.List; n != nil; n = n.Next() {
		if g, ok := n.(*node.Glyph); ok && g.Components == "c" {
			c = g
		}
	}
	if c == nil {
		t.Fatal("no glyph c")
	}
	if want := bag.MustSP("5pt"); c.YOffset != want {
		t.Errorf("c is moved by %s, want %s", c.YOffset, want)
	}
	if want := bag.MustSP("3pt"); c.LineShift != want {
		t.Errorf("c has line shift %s, want %s", c.LineShift, want)
	}
	if want := bag.MustSP("3pt"); seen != want {
		t.Errorf("the model sees a line shift of %s, want %s", seen, want)
	}
}
