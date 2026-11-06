package htmlbag

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

func TestTraceValues(t *testing.T) {
	for _, tc := range []struct {
		value              string
		boxModel, lineGrid bool
	}{
		{"none", false, false},
		{"boxmodel", true, false},
		{"linegrid", false, true},
		{"LineGrid", false, true},
		{"boxmodel linegrid", true, true},
		{"grid", false, false},
	} {
		ih := resolveOnto(t, baseStyles(), resolveCSSText(`-bag-trace: `+tc.value))
		if ih.traceBoxModel != tc.boxModel || ih.traceLineGrid != tc.lineGrid {
			t.Errorf("-bag-trace: %s gives boxmodel %t, linegrid %t, want %t, %t", tc.value, ih.traceBoxModel, ih.traceLineGrid, tc.boxModel, tc.lineGrid)
		}
	}
}

func TestLineGridBaselines(t *testing.T) {
	g := lineGrid{step: bag.MustSP("12pt"), first: bag.MustSP("9pt")}
	want := []bag.ScaledPoint{bag.MustSP("9pt"), bag.MustSP("21pt"), bag.MustSP("33pt")}
	if got := g.baselines(bag.MustSP("40pt")); !slices.Equal(got, want) {
		t.Errorf("baselines = %v, want %v", got, want)
	}
	if got := g.baselines(bag.MustSP("8pt")); got != nil {
		t.Errorf("a region above the first baseline has the baselines %v", got)
	}
}

// The grid has to be the one the text really has, in every leading model.
func TestLineGridMatchesTheLines(t *testing.T) {
	for _, tc := range []struct{ name, css, html string }{
		{"the body", `body{line-height:15pt;font-size:9pt}`, `<p>x</p>`},
		{"a font size", `p{line-height:24pt;font-size:20pt}`, `<p>x</p>`},
		{"a factor", `p{line-height:1.5}`, `<p>x</p>`},
		{"trailing leading", `p{line-height:14pt;-bag-leading-model:trailing}`, `<p>x</p>`},
		{"an element that asks for it", `h1{line-height:30pt}p{line-height:14pt;-bag-trace:linegrid}`, `<h1>x</h1><p>x</p>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, tc.css)
			cb.TraceLineGrid = true
			if err := cb.InitPage(); err != nil {
				t.Fatal(err)
			}
			te, err := cb.HTMLToText(`<html><body>` + tc.html + `</body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			g := cb.lineGridFor(te)
			if g == nil {
				t.Fatal("no line grid")
			}
			vl, err := cb.CreateVlist(te, bag.MustSP("100pt"))
			if err != nil {
				t.Fatal(err)
			}
			lines := placedHLists(vl)
			line := lines[len(lines)-1]
			if g.first != line.Height {
				t.Errorf("first baseline %s, the line has %s", g.first, line.Height)
			}
			cb2, _ := newFlowBuilder(t, tc.css)
			tr := flow(t, cb2, strings.ReplaceAll(tc.html, "x", charLines("A", 2)), wide("500pt"))
			placed := boxLines(tr.filled[0])
			placed = placed[len(placed)-2:]
			if d := placed[1].top - placed[0].top; g.step != d {
				t.Errorf("step %s, the baselines are %s apart", g.step, d)
			}
		})
	}
}

// placedHLists returns the lines below n.
func placedHLists(n node.Node) []*node.HList {
	var lines []*node.HList
	for e := n; e != nil; e = e.Next() {
		switch v := e.(type) {
		case *node.VList:
			lines = append(lines, placedHLists(v.List)...)
		case *node.HList:
			if origin, _ := v.Attributes["origin"].(string); origin == "line" {
				lines = append(lines, v)
			} else {
				lines = append(lines, placedHLists(v.List)...)
			}
		}
	}
	return lines
}

func TestLineGridTakesTheRunningText(t *testing.T) {
	const html = `<html><body><h1>A short heading</h1><p>The running text of this document, which is longer.</p></body></html>`
	for _, tc := range []struct{ name, css string }{
		{"the switch", ``},
		{"the mark on the body", `body{-bag-trace:linegrid}`},
		{"a line-height on the body as well", `body{line-height:30pt}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cb, _ := newLineModelBuilder(t, `h1{line-height:40pt}p{line-height:14pt;font-size:10pt}`+tc.css, func(cb *CSSBuilder) { cb.TraceLineGrid = true })
			te, err := cb.HTMLToText(html)
			if err != nil {
				t.Fatal(err)
			}
			if g := cb.lineGridFor(te); g == nil || g.step != bag.MustSP("14pt") {
				t.Errorf("line grid %+v, want a step of 14pt", g)
			}
		})
	}
}

func TestLineGridWithoutText(t *testing.T) {
	cb, _ := newLineModelBuilder(t, ``, func(cb *CSSBuilder) { cb.TraceLineGrid = true })
	te, err := cb.HTMLToText(`<html><body></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if g := cb.lineGridFor(te); g != nil {
		t.Errorf("a line grid without text: %+v", g)
	}
}

func TestLineGridNeedsAskingFor(t *testing.T) {
	cb, _ := newFlowBuilder(t, `p{line-height:14pt}`)
	te, err := cb.HTMLToText(`<html><body><p>x</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if g := cb.lineGridFor(te); g != nil {
		t.Errorf("a line grid without the trace: %+v", g)
	}
}

var lineGridY = regexp.MustCompile(`-3(?:\.0+)? (-?[0-9.]+) m`)

// lineGridLines returns every line grid rule below n with the distances of
// its lines from the top in points.
func lineGridLines(n node.Node) [][]float64 {
	var rules [][]float64
	for e := n; e != nil; e = e.Next() {
		switch v := e.(type) {
		case *node.VList:
			rules = append(rules, lineGridLines(v.List)...)
		case *node.HList:
			rules = append(rules, lineGridLines(v.List)...)
		case *node.Rule:
			if origin, _ := v.Attributes["origin"].(string); origin == "linegrid trace" {
				lines := []float64{}
				for _, m := range lineGridY.FindAllStringSubmatch(v.Pre, -1) {
					y, _ := strconv.ParseFloat(m[1], 64)
					lines = append(lines, -y)
				}
				rules = append(rules, lines)
			}
		}
	}
	return rules
}

func pageLineGrids(pages []*document.Page) [][]float64 {
	var rules [][]float64
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				rules = append(rules, lineGridLines(obj.Vlist)...)
			}
		}
	}
	return rules
}

func pagedLineGrids(t *testing.T, css, body string) [][]float64 {
	t.Helper()
	cb, fe := newFlowBuilder(t, css)
	cb.TraceLineGrid = true
	if err := cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	return pageLineGrids(fe.Doc.Pages)
}

func TestLineGridOnEveryPage(t *testing.T) {
	rules := pagedLineGrids(t, `@page{size:200pt 200pt;margin:20pt}p{line-height:12pt}`, `<p>`+charLines("A", 39)+`</p>`)
	if len(rules) != 3 {
		t.Fatalf("%d grids on 3 pages", len(rules))
	}
	for i, lines := range rules {
		if len(lines) != 13 {
			t.Errorf("page %d: %d lines in 160pt at 12pt, want 13", i+1, len(lines))
		}
		if !slices.Equal(lines, rules[0]) {
			t.Errorf("page %d has another grid than page 1", i+1)
		}
	}
}

// The grid belongs to the page: columns, a spanner and balancing must leave
// one grid of the whole page area, as a region of theirs reaches down over
// what comes below it.
func TestLineGridOnePerPageWithColumns(t *testing.T) {
	const page = `@page{size:400pt 400pt;margin:20pt}body{column-count:2}p,h1{line-height:12pt}h1{column-span:all}`
	for _, tc := range []struct{ name, css string }{
		{"balanced", ``},
		{"column-fill auto", `body{column-fill:auto}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := pagedLineGrids(t, page+tc.css, `<p>`+charLines("A", 6)+`</p><h1>A heading</h1><p>`+charLines("B", 6)+`</p>`)
			if len(rules) != 1 {
				t.Fatalf("%d grids on one page", len(rules))
			}
			if len(rules[0]) != 30 {
				t.Errorf("%d lines in 360pt at 12pt, want 30", len(rules[0]))
			}
			for i := 1; i < len(rules[0]); i++ {
				if d := rules[0][i] - rules[0][i-1]; d < 11.99 || d > 12.01 {
					t.Errorf("lines %d and %d are %gpt apart", i, i+1, d)
				}
			}
		})
	}
}

func TestLineGridOnABlankPage(t *testing.T) {
	cb, fe := newFlowBuilder(t, `p{line-height:12pt}.r{break-before:right}`)
	cb.TraceLineGrid = true
	if err := cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<html><body><p>a</p><p>b</p><p class="r">c</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if got, want := len(pageLineGrids(fe.Doc.Pages)), len(fe.Doc.Pages); got != want {
		t.Errorf("%d grids on %d pages", got, want)
	}
}

func TestLineGridInACallersRegion(t *testing.T) {
	cb, _ := newFlowBuilder(t, `p{line-height:12pt;margin:0}`)
	cb.TraceLineGrid = true
	tr := flow(t, cb, `<p>`+charLines("A", 3)+`</p>`, wide("500pt"))
	rules := lineGridLines(tr.filled[0].Box)
	if len(rules) != 1 {
		t.Fatalf("%d grids in one region", len(rules))
	}
	// Only as far as the box goes: the rest may go to another flow.
	if len(rules[0]) != 3 {
		t.Errorf("%d lines for 3 lines of text in a high region", len(rules[0]))
	}
	if h := tr.filled[0].Box.Height; h != bag.MustSP("36pt") {
		t.Errorf("the trace changes the box's height to %s", h)
	}
}

func TestLineGridBelowTheMarginBefore(t *testing.T) {
	at := func(margin string) float64 {
		cb, _ := newFlowBuilder(t, `p{line-height:12pt;margin:0}`)
		cb.TraceLineGrid = true
		r := wide("500pt")
		r.MarginBefore = bag.MustSP(margin)
		rules := lineGridLines(flow(t, cb, `<p>`+charLines("A", 2)+`</p>`, r).filled[0].Box)
		if len(rules) != 1 || len(rules[0]) == 0 {
			t.Fatalf("margin %s: grids %v", margin, rules)
		}
		return rules[0][0]
	}
	if d := at("4pt") - at("0pt"); d < 3.99 || d > 4.01 {
		t.Errorf("a margin of 4pt moves the grid by %gpt", d)
	}
}

func TestLineGridMarkStaysOutOfTheParagraph(t *testing.T) {
	rules := pagedLineGrids(t, `p{line-height:12pt}`, `<p>a <span style="-bag-trace:linegrid">b</span> c</p><table><tr><td style="-bag-trace:linegrid">d</td></tr></table>`)
	if len(rules) != 1 {
		t.Fatalf("%d grids on one page", len(rules))
	}
}

func TestLineGridOverlay(t *testing.T) {
	cb, _ := newFlowBuilder(t, ``)
	g := lineGrid{cb: cb, step: bag.MustSP("12pt"), first: bag.MustSP("9pt")}
	vl := g.overlay(bag.MustSP("100pt"), bag.MustSP("150pt"))
	if vl.Height != 0 || vl.Depth != 0 {
		t.Errorf("the overlay takes up room: %s, %s", vl.Height, vl.Depth)
	}
	if _, ok := vl.Attributes["artifact"].(document.ArtifactType); !ok {
		t.Error("the overlay is not an artifact")
	}
	var numbers []string
	var cursor bag.ScaledPoint
	for n := node.Node(vl.List); n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.Kern:
			cursor += v.Kern
		case *node.HList:
			cursor += v.Height
			if cursor != g.first+bag.ScaledPoint(len(numbers))*g.step {
				t.Errorf("number %d stands on %s", len(numbers)+1, cursor)
			}
			cursor += v.Depth
			numbers = append(numbers, glyphText(v.List))
		}
	}
	if want := "1 2 3 4 5 6 7 8 9 10 11 12"; strings.Join(numbers, " ") != want {
		t.Errorf("numbers %q, want %q", strings.Join(numbers, " "), want)
	}
}

// glyphText returns the characters below n.
func glyphText(n node.Node) string {
	var b strings.Builder
	for e := n; e != nil; e = e.Next() {
		switch v := e.(type) {
		case *node.Glyph:
			b.WriteString(v.Components)
		case *node.HList:
			b.WriteString(glyphText(v.List))
		case *node.VList:
			b.WriteString(glyphText(v.List))
		}
	}
	return b.String()
}

func TestLineGridStepOfThePage(t *testing.T) {
	const page = `@page{size:200pt 200pt;margin:20pt}p{line-height:12pt}`
	pages := `<p>a</p><p class="n">b</p>`
	for _, tc := range []struct {
		name, css string
		want      []int
	}{
		{"none", ``, []int{13, 13}},
		{"a length", `@page{-bag-line-grid:20pt}`, []int{8, 8}},
		{"the first page", `@page :first{-bag-line-grid:20pt}`, []int{8, 13}},
		{"a side", `@page :left{-bag-line-grid:40pt}`, []int{13, 4}},
		{"not a length", `@page{-bag-line-grid:auto}`, []int{13, 13}},
		{"zero", `@page{-bag-line-grid:0pt}`, []int{13, 13}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := pagedLineGrids(t, page+tc.css+`.n{break-before:page}`, pages)
			if len(rules) != len(tc.want) {
				t.Fatalf("%d grids, want %d", len(rules), len(tc.want))
			}
			for i, want := range tc.want {
				if len(rules[i]) != want {
					t.Errorf("page %d has %d lines, want %d", i+1, len(rules[i]), want)
				}
			}
		})
	}
}

// With a distance of its own, the first line is where a line of the running
// text would stand at that leading.
func TestLineGridStepMovesTheFirstBaseline(t *testing.T) {
	first := func(css string) float64 {
		rules := pagedLineGrids(t, `@page{size:200pt 200pt;margin:20pt}p{line-height:12pt}`+css, `<p>a</p>`)
		if len(rules) != 1 || len(rules[0]) == 0 {
			t.Fatalf("grids %v", rules)
		}
		return rules[0][0]
	}
	got := first(`@page{-bag-line-grid:20pt}`)
	if want := first(`p{line-height:20pt}`); got != want {
		t.Errorf("first baseline %g, a line at that leading has %g", got, want)
	}
}

func TestLineGridInvalidStepWarnsOnce(t *testing.T) {
	logs, restore := captureLog()
	defer restore()
	pagedLineGrids(t, `@page{size:200pt 200pt;margin:20pt;-bag-line-grid:auto}p{line-height:12pt}`, `<p>`+charLines("A", 39)+`</p>`)
	if n := strings.Count(logs.String(), "-bag-line-grid"); n != 1 {
		t.Errorf("%d warnings:\n%s", n, logs)
	}
}
