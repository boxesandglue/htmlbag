package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

const snapCSS = charCSS + `body { line-snap: baseline; }`

func pt(s string) bag.ScaledPoint { return bag.MustSP(s) }

// snapPlaced renders body with the styles of snapCSS and css.
func snapPlaced(t *testing.T, css, body string) ([]*document.Page, []placedLine) {
	t.Helper()
	pages, _ := renderHTMLPagesCB(t, snapCSS+css, `<html><body>`+body+`</body></html>`)
	return pages, placedLines(pages)
}

// onGrid fails unless the line with mark stands level with the first line of
// a page, at a whole number of steps below it.
func onGrid(t *testing.T, lines []placedLine, mark string, step bag.ScaledPoint) {
	t.Helper()
	if off := (charTop - firstLine(t, lines, mark).top) % step; off != 0 {
		t.Errorf("line %q is %s off the grid", mark, off)
	}
}

func TestLineSnapValues(t *testing.T) {
	for value, want := range map[string]bool{"none": false, "baseline": true, "BaseLine": true} {
		if ih := resolveOnto(t, baseStyles(), resolveCSSText(`line-snap: `+value)); ih.lineSnap != want {
			t.Errorf("line-snap: %s gives %t", value, ih.lineSnap)
		}
	}
	base := baseStyles()
	base.lineSnap = true
	if ih := resolveOnto(t, base.Clone(), resolveCSSText(`line-snap: contain`)); !ih.lineSnap {
		t.Error("the unsupported contain changes the inherited value")
	}
}

func TestNearestGridLine(t *testing.T) {
	g := &lineGrid{step: pt("12pt"), first: pt("9pt")}
	for _, tc := range []struct {
		name       string
		y, margins bag.ScaledPoint
		want       bag.ScaledPoint
	}{
		{"on a line", pt("21pt"), 0, 0},
		{"a rounding error", pt("21pt") + lineSnapTolerance, 0, 0},
		{"above the first line", pt("4pt"), 0, pt("5pt")},
		{"nearest is below", pt("25pt"), 0, pt("8pt")},
		{"nearest is above, margins allow it", pt("25pt"), pt("6pt"), pt("-4pt")},
		{"nearest is above, margins do not allow it", pt("25pt"), pt("3pt"), pt("8pt")},
		{"margins go exactly to zero", pt("25pt"), pt("4pt"), pt("-4pt")},
	} {
		if got := nearestGridLine(tc.y, g, tc.margins); got != tc.want {
			t.Errorf("%s: rest %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestGridMultiple(t *testing.T) {
	step := pt("12pt")
	for d, want := range map[bag.ScaledPoint]bag.ScaledPoint{
		pt("5pt"):                  step,
		step:                       step,
		step + lineSnapTolerance:   step,
		step + 2*lineSnapTolerance: 2 * step,
		step - 1:                   step,
		pt("20pt"):                 2 * step,
	} {
		if got := gridMultiple(d, step); got != want {
			t.Errorf("gridMultiple(%s) = %s, want %s", d, got, want)
		}
	}
}

// The rests stand left of the column, clear of its text.
func TestLineSnapTraceMark(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.AddCSS(snapCSS); err != nil {
		t.Fatal(err)
	}
	g := lineGrid{cb: cb, step: charLine}
	hl, err := g.label("+12pt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if w := textWidth(hl); w <= 0 || w >= pt("30pt") {
		t.Errorf("the mark is %s wide", w)
	}
}

func TestFormatRest(t *testing.T) {
	for rest, want := range map[bag.ScaledPoint]string{
		pt("5pt"):    "+5pt",
		pt("-1.5pt"): "-1.5pt",
	} {
		if got := formatRest(rest); got != want {
			t.Errorf("formatRest(%s) = %q, want %q", rest, got, want)
		}
	}
}

// The piece between two lines goes to the nearest multiple of the grid: a
// margin shrinks down to zero or grows.
func TestLineSnapMargin(t *testing.T) {
	for _, tc := range []struct {
		margin, want string
	}{
		{"0", "12pt"},
		{"5pt", "12pt"},
		{"7pt", "24pt"},
		{"12pt", "24pt"},
		{"17pt", "24pt"},
	} {
		_, lines := snapPlaced(t, `p { margin-bottom: `+tc.margin+`; }`, `<p>A</p><p>B</p>`)
		if d := firstLine(t, lines, "A").top - firstLine(t, lines, "B").top; d != pt(tc.want) {
			t.Errorf("margin %s: lines %s apart, want %s", tc.margin, d, tc.want)
		}
	}
}

// Without line-snap, the same margin stays what it is.
func TestLineSnapOff(t *testing.T) {
	for name, css := range map[string]string{
		"on the element": `p { margin-bottom: 5pt; line-snap: none; }`,
		"not declared":   `body { line-snap: none; } p { margin-bottom: 5pt; }`,
	} {
		_, lines := snapPlaced(t, css, `<p>A</p><p>B</p>`)
		if d := firstLine(t, lines, "A").top - firstLine(t, lines, "B").top; d != pt("17pt") {
			t.Errorf("%s: lines %s apart", name, d)
		}
	}
}

// The rest goes into the margins of the piece in proportion to them, around a
// heading that stays free of the grid.
func TestLineSnapSharesTheRest(t *testing.T) {
	_, lines := snapPlaced(t, `h2 { font-size: 10pt; line-height: 12pt; margin: 6pt 0 2pt 0; line-snap: none; }`,
		`<p>A</p><h2>H</h2><p>B</p>`)
	a, h, b := firstLine(t, lines, "A"), firstLine(t, lines, "H"), firstLine(t, lines, "B")
	if d := a.bottom - h.top; d != pt("9pt") {
		t.Errorf("margin above the heading %s, want 9pt", d)
	}
	if d := h.bottom - b.top; d != pt("3pt") {
		t.Errorf("margin below the heading %s, want 3pt", d)
	}
}

// Without a margin, the rest stands above the line, and the first line of a
// taller paragraph finds the grid.
func TestLineSnapWithoutMargins(t *testing.T) {
	_, lines := snapPlaced(t, `.tall { line-height: 17pt; }`,
		`<p>Running text that is the longest of them all</p><p class="tall">B</p><p>C</p>`)
	if d := firstLine(t, lines, "Running").top - firstLine(t, lines, "C").top; d%charLine != 0 {
		t.Errorf("the line after a taller one is %s below the first", d)
	}
}

// The lines of a paragraph with another line-height go to the next multiple.
func TestLineSnapInsideParagraph(t *testing.T) {
	_, lines := snapPlaced(t, `.wide { line-height: 14pt; }`,
		`<p>Running text that is the longest of them all</p><p class="wide">B<br>C<br>D</p><p>E</p>`)
	if d := firstLine(t, lines, "B").top - firstLine(t, lines, "C").top; d != pt("24pt") {
		t.Errorf("lines of the paragraph %s apart, want 24pt", d)
	}
	onGrid(t, lines, "E", charLine)
}

func TestLineSnapPageGrid(t *testing.T) {
	_, lines := snapPlaced(t, `@page { -bag-line-grid: 15pt; } p { margin-bottom: 2pt; }`, `<p>A<br>B</p><p>C</p>`)
	if d := firstLine(t, lines, "A").top - firstLine(t, lines, "B").top; d != pt("15pt") {
		t.Errorf("lines %s apart, want 15pt", d)
	}
	if d := firstLine(t, lines, "B").top - firstLine(t, lines, "C").top; d != pt("15pt") {
		t.Errorf("paragraphs %s apart, want 15pt", d)
	}
}

// The margins inside a transparent container are reached, its own margin,
// shift and break are not lost, and nothing of it is lost at a page break.
func TestLineSnapInContainer(t *testing.T) {
	body := `<p>A</p><section style="margin-top: 7pt; padding-left: 10pt; break-after: page"><p style="margin-top: 7pt">B</p><p style="margin-top: 7pt">C</p></section><p>D</p>`
	_, lines := snapPlaced(t, ``, body)
	onGrid(t, lines, "B", charLine)
	onGrid(t, lines, "C", charLine)
	if d := firstLine(t, lines, "B").top - firstLine(t, lines, "C").top; d != pt("24pt") {
		t.Errorf("paragraphs in the container %s apart, want 24pt", d)
	}
	if l := firstLine(t, lines, "D"); l.page != 2 {
		t.Errorf("the break after the container is lost, D is on page %d", l.page)
	}
}

// A box with a border keeps its inside, the line after it finds the grid.
func TestLineSnapRigidBox(t *testing.T) {
	_, lines := snapPlaced(t, `div { border: 1pt solid black; padding: 2.5pt; }`,
		`<p>A</p><div><p>B</p><p>C</p></div><p>D</p>`)
	onGrid(t, lines, "D", charLine)
}

func TestLineSnapTable(t *testing.T) {
	_, lines := snapPlaced(t, `td { border: 1pt solid black; padding: 2.5pt; }`,
		`<p>A</p><table><tr><td>T</td></tr></table><p>D</p>`)
	onGrid(t, lines, "D", charLine)
}

// The lines of both columns stand level, also with different margins and
// below a spanner, in the rows the balancing trials fill as well.
func TestLineSnapColumns(t *testing.T) {
	css := `@page { size: 300pt 300pt; } body { column-count: 2; column-gap: 10pt; }
	h1 { column-span: all; line-height: 17pt; margin-bottom: 4pt; line-snap: none; }
	p { margin-bottom: 5pt; }`
	var body strings.Builder
	body.WriteString(`<h1>Title</h1>`)
	for _, m := range []string{"M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8"} {
		body.WriteString(`<p>` + charLines(m, 4) + `</p>`)
	}
	pages, lines := snapPlaced(t, css, body.String())
	if len(pages) != 1 {
		t.Fatalf("%d pages", len(pages))
	}
	lefts := map[bag.ScaledPoint]bool{}
	for _, l := range lines {
		if l.text == "Title" {
			continue
		}
		lefts[l.top%charLine] = true
	}
	if len(lefts) != 1 {
		t.Errorf("the lines stand in %d phases of the grid: %v", len(lefts), lefts)
	}
}

// A paragraph split by the page keeps its lines on the grid, and what fits
// counts the rest: the last line of the page is not pushed over its foot.
func TestLineSnapAcrossPages(t *testing.T) {
	_, lines := snapPlaced(t, `p { margin-bottom: 5pt; }`,
		`<p>`+charLines("A", 11)+`</p><p>`+charLines("B", 6)+`</p>`)
	for _, l := range lines {
		if l.page == 1 && l.bottom < charTop-pt("160pt") {
			t.Errorf("line %q reaches %s below the page area", l.text, charTop-pt("160pt")-l.bottom)
		}
		if off := (charTop - l.top) % charLine; off != 0 {
			t.Errorf("line %q on page %d is %s off the grid", l.text, l.page, off)
		}
	}
	if l := firstLine(t, lines, "B"); l.page != 1 || l.top != charTop-11*charLine {
		t.Errorf("B has room on page 1 and is at %s on page %d", l.top, l.page)
	}
	if lines[len(lines)-1].page != 2 {
		t.Error("the paragraph does not go on on page 2")
	}
}

// A top float in a later block moves the lines placed before it, and they
// stay on the grid.
func TestLineSnapTopFloatArrivesLate(t *testing.T) {
	float := `<div style="float: top; height: 17pt">F</div>`
	_, lines := snapPlaced(t, `p { margin-bottom: 5pt; }`,
		`<p>A</p><p>B</p><section>`+float+`<p>C</p></section><p>D</p>`)
	for _, mark := range []string{"A", "B", "C", "D"} {
		onGrid(t, lines, mark, charLine)
	}
	if l := firstLine(t, lines, "A"); l.top == charTop {
		t.Error("the float did not move the body down")
	}
}

// Without line-snap, the zone keeps its height.
func TestTopFloatZoneOffGrid(t *testing.T) {
	cb := &CSSBuilder{FloatTopInterSkip: pt("5pt")}
	box := node.NewVList()
	box.Height = pt("17pt")
	if got := cb.totalFloatTopHeight([]*Insert{{Body: box}}); got != pt("22pt") {
		t.Errorf("zone of %s", got)
	}
}

// FlowText leaves line snapping out.
func TestLineSnapFlowText(t *testing.T) {
	cb, _ := newFlowBuilder(t, `body { line-snap: baseline; } p { margin: 0 0 5pt; font-size: 10pt; line-height: 12pt; }`)
	tr := flow(t, cb, `<p>A</p><p>B</p>`, wide("500pt"))
	placed := boxLines(tr.filled[0])
	if d := placed[1].top - placed[0].top; d != pt("17pt") {
		t.Errorf("lines %s apart, want 17pt", d)
	}
}
