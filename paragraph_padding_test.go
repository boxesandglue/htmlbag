package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// paddingCSS sets justified lines, so that every line but the last spans the
// measure it was given.
const paddingCSS = "p { margin: 0; font-size: 10pt; line-height: 12pt; text-align: justify } * { -bag-float-gutter: 0 }"

// The padding of a paragraph without a border or background is the room
// between its edges and its lines, on both sides, as in a browser: the lines
// of a 200pt paragraph with 20pt padding-left run from 20pt to 200pt.
func TestAParagraphsPaddingIsTakenOffOnce(t *testing.T) {
	for _, c := range []struct {
		name, style string
		from, to    string
	}{
		{"padding-left", "padding-left:20pt", "20pt", "200pt"},
		{"padding-right", "padding-right:20pt", "0pt", "180pt"},
		{"both", "padding:0 10pt 0 20pt", "20pt", "190pt"},
		{"padding and a margin", "padding-left:20pt;margin-left:10pt", "30pt", "200pt"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb := floatBuilder(t)
			if err := cb.AddCSS(paddingCSS); err != nil {
				t.Fatal(err)
			}
			lines := linesAcross(buildHTML(t, cb, `<div><p style="`+c.style+`">`+floatProse+`</p></div>`))
			if len(lines) < 2 {
				t.Fatal("too few lines")
			}
			for i, l := range lines[:len(lines)-1] {
				if l.x0 != bag.MustSP(c.from) || l.x1 != bag.MustSP(c.to) {
					t.Errorf("line %d runs from %s to %s, want %s to %s", i, l.x0, l.x1, c.from, c.to)
				}
			}
		})
	}
}

// text-indent is in from the start of the content box, so it adds to the
// padding-left rather than taking its place.
func TestTextIndentAddsToAParagraphsPadding(t *testing.T) {
	cb := floatBuilder(t)
	if err := cb.AddCSS(paddingCSS); err != nil {
		t.Fatal(err)
	}
	lines := linesAcross(buildHTML(t, cb, `<div><p style="padding-left:20pt;text-indent:10pt">`+floatProse+`</p></div>`))
	if len(lines) < 3 {
		t.Fatal("too few lines")
	}
	if want := bag.MustSP("30pt"); lines[0].x0 != want {
		t.Errorf("the first line starts at %s, want %s", lines[0].x0, want)
	}
	for i, l := range lines[1 : len(lines)-1] {
		if want := bag.MustSP("20pt"); l.x0 != want || l.x1 != bag.MustSP("200pt") {
			t.Errorf("line %d runs from %s to %s, want 20pt to 200pt", i+1, l.x0, l.x1)
		}
	}
}

// A float without a width shrinks to its content plus its padding, and its
// text starts inside the padding: the padding is taken into its width once.
func TestAFloatsPaddingIsPartOfItsWidth(t *testing.T) {
	floatWidth := func(style string) (bag.ScaledPoint, bag.ScaledPoint) {
		cb := floatBuilder(t)
		if err := cb.AddCSS(paddingCSS); err != nil {
			t.Fatal(err)
		}
		vl := buildHTML(t, cb, `<div><div style="float:left;`+style+`">Short</div><p>`+floatProse+`</p></div>`)
		box := floatBox(vl)
		if box == nil {
			t.Fatal("no float")
		}
		// linesAcross passes over float boxes; this one is all it is given.
		content := *box
		content.Attributes = nil
		lines := linesAcross(&content)
		if len(lines) != 1 {
			t.Fatalf("%d lines in the float, want 1", len(lines))
		}
		return box.Width, lines[0].x0
	}
	bare, _ := floatWidth("")
	padded, x0 := floatWidth("padding-left:10pt;padding-right:5pt")
	if want := bare + bag.MustSP("15pt"); padded != want {
		t.Errorf("the float with padding is %s wide, want %s (%s without)", padded, want, bare)
	}
	if want := bag.MustSP("10pt"); x0 != want {
		t.Errorf("the float's text starts at %s, want %s", x0, want)
	}
}

// pageLines returns the lines on a page, x measured from the left edge of the
// page's content area, left in from the page's edge.
func pageLines(pg *document.Page, left bag.ScaledPoint) []lineAcross {
	var out []lineAcross
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		for _, l := range linesAcross(obj.Vlist) {
			shift := obj.X + obj.Vlist.ShiftX - left
			out = append(out, lineAcross{l.x0 + shift, l.x1 + shift, l.y})
		}
	}
	return out
}

// A paragraph that breaks across pages keeps its padding on every page, also
// where its rest is set again at the width of a wider page.
func TestAParagraphKeepsItsPaddingAcrossPages(t *testing.T) {
	html := `<html><body><p style="padding-left:20pt;text-align:justify">` + strings.Repeat("Wort ", 4000) + `</p></body></html>`
	for _, c := range []struct {
		name, css string
		widths    []string // content width of the first and of the later pages
	}{
		{"same width", "@page { size: a4; margin: 20mm } body { margin: 0 }", []string{"170mm", "170mm"}},
		{"wider pages after the first", reflowNarrowFirstCSS, []string{"130mm", "170mm"}},
		{"narrower pages after the first", reflowWideFirstCSS, []string{"170mm", "130mm"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages, _ := renderHTMLPagesCB(t, c.css, html)
			if len(pages) < 3 {
				t.Fatalf("got %d pages, want at least 3", len(pages))
			}
			// Every page but the last is full of justified lines.
			for i, pg := range pages[:len(pages)-1] {
				width := c.widths[min(i, 1)]
				lines := pageLines(pg, bag.MustSP("20mm"))
				if len(lines) == 0 {
					t.Fatalf("page %d: no lines", i+1)
				}
				for j, l := range lines {
					if l.x0 != bag.MustSP("20pt") || absSP(l.x1-bag.MustSP(width)) > bag.MustSP("0.01pt") {
						t.Errorf("page %d, line %d runs from %s to %s, want 20pt to %s", i+1, j, l.x0, l.x1, width)
						break
					}
				}
				// The box the lines are in is the paragraph's, padding and all.
				for _, wd := range lineHolderWidths(pg) {
					if absSP(wd-bag.MustSP(width)) > bag.MustSP("0.01pt") {
						t.Errorf("page %d: the paragraph's box is %s wide, want %s", i+1, wd, width)
					}
				}
			}
		})
	}
}

// lineHolderWidths returns the width of every box on a page that holds lines
// directly.
func lineHolderWidths(pg *document.Page) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(v *node.VList)
	walk = func(v *node.VList) {
		holds := false
		for n := v.List; n != nil; n = n.Next() {
			switch c := n.(type) {
			case *node.VList:
				walk(c)
			case *node.HList:
				if origin, _ := c.Attributes["origin"].(string); origin == "line" {
					holds = true
				}
			}
		}
		if holds {
			out = append(out, v.Width)
		}
	}
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			walk(obj.Vlist)
		}
	}
	return out
}

func absSP(v bag.ScaledPoint) bag.ScaledPoint {
	if v < 0 {
		return -v
	}
	return v
}

// The rest of a paragraph with a border that is set again at the width of
// another page keeps its padding once, around the lines as before the break.
func TestABorderedParagraphsRestKeepsItsPaddingOnce(t *testing.T) {
	html := `<html><body><p style="border:1pt solid black;padding-left:20pt;text-align:justify">` + strings.Repeat("Wort ", 4000) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, reflowNarrowFirstCSS, html)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want at least 3", len(pages))
	}
	for i, pg := range pages[:len(pages)-1] {
		width := "170mm"
		if i == 0 {
			width = "130mm"
		}
		for j, l := range pageLines(pg, bag.MustSP("20mm")) {
			if l.x0 != bag.MustSP("21pt") || absSP(l.x1-bag.MustSP(width)+bag.MustSP("1pt")) > bag.MustSP("0.01pt") {
				t.Errorf("page %d, line %d runs from %s to %s, want 21pt to %s less 1pt", i+1, j, l.x0, l.x1, width)
				break
			}
		}
	}
}
