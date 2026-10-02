package htmlbag

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"golang.org/x/net/html"
)

// testRegions hands out regions of the given sizes, the last one as often
// as needed, and records what FlowText asks and hands back.
type testRegions struct {
	sizes  []Region
	brks   []string
	filled []Filled
	// order records "next" and "filled" calls in sequence.
	order []string
}

func (tr *testRegions) Next(brk string) (Region, error) {
	tr.brks = append(tr.brks, brk)
	tr.order = append(tr.order, "next")
	i := min(len(tr.brks)-1, len(tr.sizes)-1)
	return tr.sizes[i], nil
}

func (tr *testRegions) Filled(f Filled) error {
	tr.filled = append(tr.filled, f)
	tr.order = append(tr.order, "filled")
	return nil
}

func sp(s string) bag.ScaledPoint { return bag.MustSP(s) }

// newFlowBuilder is a builder with charCSS plus css.
func newFlowBuilder(t *testing.T, css string) (*CSSBuilder, *frontend.Document) {
	t.Helper()
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
	if err := cb.AddCSS(charCSS + css); err != nil {
		t.Fatal(err)
	}
	return cb, fe
}

// flow pours body into regions of the given sizes.
func flow(t *testing.T, cb *CSSBuilder, body string, sizes ...Region) *testRegions {
	t.Helper()
	te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	tr := &testRegions{sizes: sizes}
	if err := cb.FlowText(te, tr); err != nil {
		t.Fatalf("FlowText: %v", err)
	}
	return tr
}

// boxLines lists the lines and table rows of a region's box, with top and
// bottom measured down from the box's top edge.
func boxLines(f Filled) []placedLine {
	pages := []*document.Page{{Objects: []document.Object{{Vlist: f.Box}}}}
	lines := placedLines(pages)
	for i := range lines {
		lines[i].top, lines[i].bottom = -lines[i].top, -lines[i].bottom
	}
	return lines
}

func wide(h string) Region { return Region{Width: sp("160pt"), Height: sp(h)} }

func TestFlowTextFillsRegionsOfDifferentSizes(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<p>`+charLines("A", 20)+`</p>`, wide("60pt"), wide("36pt"), wide("1000pt"))
	if len(tr.filled) != 3 {
		t.Fatalf("filled %d regions, want 3", len(tr.filled))
	}
	for i, want := range []int{5, 3, 12} {
		f := tr.filled[i]
		lines := boxLines(f)
		if len(lines) != want {
			t.Errorf("region %d holds %d lines, want %d", i+1, len(lines), want)
		}
		if used := bag.ScaledPoint(want) * charLine; f.Used != used || f.Box.Height != used {
			t.Errorf("region %d: Used %s, Box.Height %s, want %s", i+1, f.Used, f.Box.Height, used)
		}
		if len(lines) > 0 && lines[0].top != 0 {
			t.Errorf("region %d starts at %s, want its top edge", i+1, lines[0].top)
		}
	}
	if want := "next filled next filled next filled"; strings.Join(tr.order, " ") != want {
		t.Errorf("calls %q, want %q", strings.Join(tr.order, " "), want)
	}
}

// A paragraph split into a narrower region is re-broken at its width where a
// page of another @page width re-breaks it: among other blocks. The lines of
// a paragraph that is the flow's only block keep their width, as they do on
// pages.
func TestFlowTextNarrowerRegion(t *testing.T) {
	text := strings.Repeat("alpha beta gamma delta epsilon ", 30)
	widest := func(f Filled) bag.ScaledPoint {
		var w bag.ScaledPoint
		for _, l := range boxLines(f) {
			w = max(w, l.width)
		}
		return w
	}
	cases := []struct {
		name, body string
		rebroken   bool
	}{
		{"among other blocks", `<p>Aq</p><p>` + text + `</p>`, true},
		{"the only block", `<p>` + text + `</p>`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, c.body, wide("48pt"), Region{Width: sp("80pt"), Height: sp("1000pt")})
			if len(tr.filled) != 2 {
				t.Fatalf("filled %d regions, want 2", len(tr.filled))
			}
			if w := widest(tr.filled[0]); w <= sp("80pt") {
				t.Errorf("region 1 lines are %s wide, want the 160pt measure", w)
			}
			if w := widest(tr.filled[1]); (w <= sp("80pt")) != c.rebroken {
				t.Errorf("region 2 lines are %s wide, re-broken to its 80pt: %v, want %v", w, !c.rebroken, c.rebroken)
			}
		})
	}
}

// A table split by rows repeats its header in the next region, and every row
// lands once.
func TestFlowTextSplitsATableAcrossRegions(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	var rows strings.Builder
	for i := range 8 {
		rows.WriteString(`<tr><td>R` + string(rune('a'+i)) + `q</td></tr>`)
	}
	tr := flow(t, cb, `<table><thead><tr><th>Hq</th></tr></thead><tbody>`+rows.String()+`</tbody></table>`,
		wide("60pt"), wide("1000pt"))
	if len(tr.filled) != 2 {
		t.Fatalf("filled %d regions, want 2", len(tr.filled))
	}
	seen := map[string]int{}
	for i, f := range tr.filled {
		lines := boxLines(f)
		if len(lines) == 0 || !strings.Contains(lines[0].text, "Hq") {
			t.Errorf("region %d does not start with the header: %v", i+1, lines)
		}
		var bottom bag.ScaledPoint
		for _, l := range lines {
			seen[l.text]++
			bottom = max(bottom, l.bottom)
		}
		if f.Used != bottom || f.Box.Height != f.Used {
			t.Errorf("region %d: Used %s, Box.Height %s, want the last row's bottom %s", i+1, f.Used, f.Box.Height, bottom)
		}
		if f.Used > wide("60pt").Height && i == 0 {
			t.Errorf("region 1 is filled to %s, beyond its 60pt", f.Used)
		}
	}
	for i := range 8 {
		if r := "R" + string(rune('a'+i)) + "q"; seen[r] != 1 {
			t.Errorf("row %s placed %d times, want once", r, seen[r])
		}
	}
}

func TestFlowTextTwiceLeavesNothingBehind(t *testing.T) {
	const css = `p { widows: 3; orphans: 3 }`
	body := `<p>` + charLines("A", 8) + `</p><p>` + charLines("B", 8) + `</p>`
	sizes := []Region{wide("60pt"), wide("1000pt")}

	fresh, _ := newFlowBuilder(t, css)
	want := flow(t, fresh, body, sizes...)

	cb, fe := newFlowBuilder(t, css)
	// A page in progress, which FlowText must keep as it is.
	held := node.NewVList()
	cb.bufferBody(held, sp("10pt"), -1, nil)
	for run := 1; run <= 2; run++ {
		got := flow(t, cb, body, sizes...)
		if len(got.filled) != len(want.filled) {
			t.Fatalf("run %d filled %d regions, want %d", run, len(got.filled), len(want.filled))
		}
		for i := range want.filled {
			if g, w := boxLines(got.filled[i]), boxLines(want.filled[i]); len(g) != len(w) || got.filled[i].Used != want.filled[i].Used {
				t.Errorf("run %d region %d: %d lines, Used %s; want %d lines, Used %s", run, i+1, len(g), got.filled[i].Used, len(w), want.filled[i].Used)
			}
		}
		if cb.fragLines != nil {
			t.Errorf("run %d left the widows/orphans map: %v", run, cb.fragLines)
		}
		if len(cb.pageBuf) != 1 || cb.pageBuf[0].box != held || cb.pageBufHeight != sp("10pt") {
			t.Errorf("run %d changed the page in progress: %d entries, %s", run, len(cb.pageBuf), cb.pageBufHeight)
		}
		for class, ins := range cb.pageInserts {
			if len(ins) > 0 {
				t.Errorf("run %d left %d inserts of class %d", run, len(ins), class)
			}
		}
		if len(cb.positionedItems) > 0 {
			t.Errorf("run %d left %d positioned items", run, len(cb.positionedItems))
		}
	}
	if len(fe.Doc.Pages) != 0 {
		t.Errorf("FlowText made %d pages", len(fe.Doc.Pages))
	}
}

func TestFlowTextUsedAndMarginAfter(t *testing.T) {
	p := func(mark string, lines int, style string) string {
		return `<p style="` + style + `">` + charLines(mark, lines) + `</p>`
	}
	cases := []struct {
		name        string
		body        string
		sizes       []Region
		used, after []string
	}{
		{"end of the flow", p("A", 3, "margin-bottom: 7pt"), []Region{wide("1000pt")},
			[]string{"36pt"}, []string{"7pt"}},
		{"no margin", p("A", 3, ""), []Region{wide("1000pt")},
			[]string{"36pt"}, []string{"0pt"}},
		{"margin spent at the foot", p("A", 3, "margin-bottom: 7pt") + p("B", 2, ""), []Region{wide("45pt"), wide("1000pt")},
			[]string{"36pt", "24pt"}, []string{"7pt", "0pt"}},
		{"margin that does not fit", p("A", 3, "margin-bottom: 7pt") + p("B", 2, ""), []Region{wide("40pt"), wide("1000pt")},
			[]string{"36pt", "24pt"}, []string{"0pt", "0pt"}},
		{"collapsed with the body", p("A", 3, "margin-bottom: 4pt"), []Region{wide("1000pt")},
			[]string{"36pt"}, []string{"9pt"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			css := ""
			if c.name == "collapsed with the body" {
				css = `body { margin-bottom: 9pt }`
			}
			cb, _ := newFlowBuilder(t, css)
			tr := flow(t, cb, c.body, c.sizes...)
			if len(tr.filled) != len(c.used) {
				t.Fatalf("filled %d regions, want %d", len(tr.filled), len(c.used))
			}
			for i, f := range tr.filled {
				if f.Used != sp(c.used[i]) || f.Box.Height != f.Used || f.MarginAfter != sp(c.after[i]) {
					t.Errorf("region %d: Used %s, Box.Height %s, MarginAfter %s; want %s, %s", i+1, f.Used, f.Box.Height, f.MarginAfter, c.used[i], c.after[i])
				}
			}
		})
	}
}

// MarginBefore collapses with the first block's margin-top at the top of the
// flow and after a forced break. After an automatic break it is dropped, and
// so is the margin that moves to the region's top with its block.
func TestFlowTextMarginBefore(t *testing.T) {
	withBefore := func(r Region, m string) Region { r.MarginBefore = sp(m); return r }
	cases := []struct {
		name  string
		body  string
		sizes []Region
		// region and top of the first line holding B.
		region int
		top    string
	}{
		{"top of the flow", `<p>` + charLines("B", 2) + `</p>`,
			[]Region{withBefore(wide("1000pt"), "10pt")}, 1, "10pt"},
		{"top of the flow, smaller margin-top", `<p style="margin-top: 4pt">` + charLines("B", 2) + `</p>`,
			[]Region{withBefore(wide("1000pt"), "10pt")}, 1, "10pt"},
		{"top of the flow, larger margin-top", `<p style="margin-top: 15pt">` + charLines("B", 2) + `</p>`,
			[]Region{withBefore(wide("1000pt"), "10pt")}, 1, "15pt"},
		{"top of the flow, margin-top alone", `<p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt")}, 1, "5pt"},
		{"automatic break", `<p>` + charLines("A", 3) + `</p><p>` + charLines("B", 2) + `</p>`,
			[]Region{wide("36pt"), withBefore(wide("1000pt"), "10pt")}, 2, "0pt"},
		{"automatic break, margin moves with the block", `<p>` + charLines("A", 3) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("40pt"), withBefore(wide("1000pt"), "10pt")}, 2, "0pt"},
		{"automatic break inside a list", `<ul style="margin: 0; padding: 0"><li>` + charLines("A", 3) + `</li><li style="margin-top: 5pt">` + charLines("B", 2) + `</li></ul>`,
			[]Region{wide("40pt"), wide("1000pt")}, 2, "0pt"},
		{"forced break", `<p>` + charLines("A", 3) + `</p><p style="break-before: page; margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt"), withBefore(wide("1000pt"), "10pt")}, 2, "10pt"},
		{"forced break, margin-top alone", `<p>` + charLines("A", 3) + `</p><p style="break-before: page; margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt")}, 2, "5pt"},
		{"break-after", `<p style="break-after: column">` + charLines("A", 3) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt"), withBefore(wide("1000pt"), "3pt")}, 2, "5pt"},
		{"break-after, margin-bottom before it", `<p style="break-after: column; margin-bottom: 9pt">` + charLines("A", 3) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt"), withBefore(wide("1000pt"), "3pt")}, 2, "5pt"},
		{"break-after, float after it", `<p style="break-after: column; margin: 0 0 20pt 0">` + charLines("A", 3) + `</p><div style="float: left; width: 40pt">Fq</div><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`,
			[]Region{wide("1000pt"), wide("1000pt")}, 2, "5pt"},
		{"automatic break inside a split list", `<p>Zq</p><ul style="margin: 0; padding: 0; background: yellow; orphans: 1; widows: 1"><li>` + charLines("A", 3) + `</li><li style="margin-top: 5pt">` + charLines("B", 2) + `</li></ul>`,
			[]Region{wide("52pt"), wide("1000pt")}, 2, "0pt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, c.body, c.sizes...)
			if len(tr.filled) < c.region {
				t.Fatalf("filled %d regions, want at least %d", len(tr.filled), c.region)
			}
			b := firstLine(t, boxLines(tr.filled[c.region-1]), "B")
			if b.top != sp(c.top) {
				t.Errorf("B starts at %s in region %d, want %s", b.top, c.region, c.top)
			}
		})
	}
}

func TestFlowTextPassesForcedBreaksToNext(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	body := `<p>` + charLines("A", 2) + `</p>` +
		`<p style="break-before: page">` + charLines("B", 2) + `</p>` +
		`<p style="break-before: column">` + charLines("C", 2) + `</p>` +
		`<p style="break-after: column">` + charLines("D", 2) + `</p>` +
		`<p>` + charLines("E", 6) + `</p>`
	tr := flow(t, cb, body, wide("48pt"))
	want := []string{"", "page", "column", "column", ""}
	if strings.Join(tr.brks, ",") != strings.Join(want, ",") {
		t.Errorf("Next got %q, want %q", tr.brks, want)
	}
}

// The first Next gets the forced break-before of the first block, so the
// caller can start the flow on a page or in a column of that kind.
func TestFlowTextPassesFirstBlocksBreakToNext(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{`<p>A</p><p>B</p>`, ""},
		{`<p style="break-before: page">A</p><p>B</p>`, "page"},
		{`<p style="break-before: column">A</p><p>B</p>`, "column"},
		{"\n  " + `<p style="break-before: left">A</p><p>B</p>`, "left"},
		{`<div style="break-before: right"><p>A</p></div><p>B</p>`, "right"},
		{`<p style="break-before: avoid">A</p><p>B</p>`, ""},
	} {
		cb, _ := newFlowBuilder(t, "")
		tr := flow(t, cb, c.body, wide("1000pt"))
		if len(tr.brks) != 1 || tr.brks[0] != c.want || len(tr.filled) != 1 {
			t.Errorf("%s: Next got %q and filled %d regions, want [%q] and 1", c.body, tr.brks, len(tr.filled), c.want)
		}
	}
}

func TestFlowTextHeadingsAndAnchorsTakeTheRegionsPage(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	r1 := Region{Width: sp("160pt"), Height: sp("30pt"), PageNum: 7, Left: sp("30pt"), Top: sp("500pt")}
	r2 := Region{Width: sp("160pt"), Height: sp("1000pt"), PageNum: 8, Left: sp("30pt"), Top: sp("300pt")}
	body := `<p>` + charLines("A", 2) + `</p><h1 id="head">Hq</h1><p id="para">` + charLines("B", 3) + `</p>`
	tr := flow(t, cb, body, r1, r2)
	if len(tr.filled) != 2 {
		t.Fatalf("filled %d regions, want 2", len(tr.filled))
	}
	if len(cb.Headings) != 1 {
		t.Fatalf("got %d headings, want 1", len(cb.Headings))
	}
	h := cb.Headings[0]
	top := firstLine(t, boxLines(tr.filled[1]), "Hq").top
	if h.Page != 8 || h.Y != r2.Top-top {
		t.Errorf("heading on page %d at %s, want page 8 at %s", h.Page, h.Y, r2.Top-top)
	}
	for _, a := range cb.Anchors {
		if a.Page != 8 {
			t.Errorf("anchor %q on page %d, want 8", a.ID, a.Page)
		}
	}
	if len(cb.Anchors) != 2 {
		t.Errorf("got %d anchors, want 2", len(cb.Anchors))
	}
}

func TestFlowTextWarnsAboutPageLevelContent(t *testing.T) {
	cases := []struct {
		name, css, body, warning string
	}{
		{"footnote", "", `<p>Aq<fn>Nq</fn></p>`, "does not support footnotes"},
		{"float top", "", `<p>Aq<span style="float: top">Tq</span></p>`, "does not support float: top"},
		{"float bottom", "", `<p>Aq<span style="float: bottom">Uq</span></p>`, "does not support float: bottom"},
		{"position absolute", `.abs { position: absolute; top: 10pt; left: 10pt; width: 50pt }`, `<p>Aq</p><div class="abs">Pq</div>`, "does not support position: absolute"},
		{"running element", `.foot { position: running(foot) }`, `<p>Aq</p><div class="foot">Fq</div>`, "does not support running elements"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			old := bag.Logger
			bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
			defer func() { bag.Logger = old }()

			cb, fe := newFlowBuilder(t, c.css)
			te, err := cb.HTMLToText(`<html><body>` + c.body + `</body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			pages := len(fe.Doc.Pages)
			tr := &testRegions{sizes: []Region{wide("1000pt")}}
			if err := cb.FlowText(te, tr); err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(buf.String(), c.warning); n != 1 {
				t.Errorf("logged %q %d times, want once:\n%s", c.warning, n, buf.String())
			}
			if len(tr.filled) != 1 {
				t.Fatalf("filled %d regions, want 1", len(tr.filled))
			}
			for _, l := range boxLines(tr.filled[0]) {
				if !strings.Contains(l.text, "Aq") {
					t.Errorf("the region holds %q, want only the paragraph", l.text)
				}
			}
			if len(fe.Doc.Pages) != pages {
				t.Errorf("FlowText made %d pages", len(fe.Doc.Pages)-pages)
			}
			if len(cb.runningElements) > 0 || len(cb.positionedItems) > 0 {
				t.Errorf("left %d running elements and %d positioned items", len(cb.runningElements), len(cb.positionedItems))
			}
		})
	}
}

// Side floats are laid out inside the region, without a warning.
func TestFlowTextKeepsSideFloats(t *testing.T) {
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()

	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<p><span style="float: left; width: 40pt">Fq</span>`+strings.Repeat("alpha beta ", 20)+`</p>`, wide("1000pt"))
	if strings.Contains(buf.String(), "FlowText") {
		t.Errorf("warned about a side float:\n%s", buf.String())
	}
	found := false
	for _, l := range boxLines(tr.filled[0]) {
		found = found || strings.Contains(l.text, "Fq")
	}
	if !found {
		t.Error("the side float is not in the region")
	}
}

// A side float that reaches below the last line of a region is part of what
// the region holds.
func TestFlowTextUsedCoversASideFloat(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	body := `<div style="float: left; width: 80pt; height: 50pt">Fq</div><p>` + strings.Repeat("alpha beta ", 4) + `</p>` +
		`<table><tr><td>` + charLines("T", 3) + `</td></tr></table>`
	tr := flow(t, cb, body, wide("60pt"), wide("1000pt"))
	if len(tr.filled) != 2 {
		t.Fatalf("filled %d regions, want 2", len(tr.filled))
	}
	if f := tr.filled[0]; f.Used != sp("50pt") || f.Box.Height != f.Used {
		t.Errorf("region 1: Used %s, Box.Height %s, want the float's bottom 50pt", f.Used, f.Box.Height)
	}
}

type failingRegions struct {
	testRegions
	failAt int
}

func (fr *failingRegions) Next(brk string) (Region, error) {
	if len(fr.brks)+1 == fr.failAt {
		return Region{}, errors.New("no more regions")
	}
	return fr.testRegions.Next(brk)
}

// A Next that fails midway ends the flow with its error, after the regions
// before it were handed back, and leaves nothing behind.
func TestFlowTextNextFails(t *testing.T) {
	cb, _ := newFlowBuilder(t, `p { widows: 3 }`)
	te, err := cb.HTMLToText(`<html><body><p>` + charLines("A", 20) + `</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	fr := &failingRegions{testRegions: testRegions{sizes: []Region{wide("48pt")}}, failAt: 3}
	if err := cb.FlowText(te, fr); err == nil || err.Error() != "no more regions" {
		t.Fatalf("FlowText returned %v, want the error from Next", err)
	}
	if want := "next filled next filled"; strings.Join(fr.order, " ") != want {
		t.Errorf("calls %q, want %q", strings.Join(fr.order, " "), want)
	}
	if cb.fragLines != nil || len(cb.pageBuf) > 0 || cb.pageBufHeight != 0 {
		t.Errorf("left %d page entries (%s) and widows/orphans %v", len(cb.pageBuf), cb.pageBufHeight, cb.fragLines)
	}
}

// nestingRegions runs nested from its second Next.
type nestingRegions struct {
	testRegions
	nested    func() error
	nestedErr error
}

func (nr *nestingRegions) Next(brk string) (Region, error) {
	if len(nr.brks) == 1 {
		nr.nestedErr = nr.nested()
	}
	return nr.testRegions.Next(brk)
}

// Neither FlowText nor OutputPagesFromText runs inside a flow on the same
// builder, and the flow it was called from goes on unharmed.
func TestFlowTextRefusesToNest(t *testing.T) {
	body := `<html><body><p>` + charLines("A", 6) + `</p></body></html>`
	nested := map[string]func(cb *CSSBuilder, te *frontend.Text) error{
		"FlowText": func(cb *CSSBuilder, te *frontend.Text) error {
			return cb.FlowText(te, &testRegions{sizes: []Region{wide("1000pt")}})
		},
		"OutputPagesFromText": func(cb *CSSBuilder, te *frontend.Text) error {
			return cb.OutputPagesFromText(te)
		},
	}
	for name, run := range nested {
		t.Run("from Regions, "+name, func(t *testing.T) {
			cb, fe := newFlowBuilder(t, "")
			te, err := cb.HTMLToText(body)
			if err != nil {
				t.Fatal(err)
			}
			inner, err := cb.HTMLToText(`<html><body><p>Zq</p></body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			nr := &nestingRegions{testRegions: testRegions{sizes: []Region{wide("36pt"), wide("1000pt")}}}
			nr.nested = func() error { return run(cb, inner) }
			if err := cb.FlowText(te, nr); err != nil {
				t.Fatalf("FlowText: %v", err)
			}
			if nr.nestedErr == nil {
				t.Errorf("nested %s ran", name)
			}
			if len(fe.Doc.Pages) != 0 {
				t.Errorf("made %d pages", len(fe.Doc.Pages))
			}
			if lines := len(boxLines(nr.filled[0])) + len(boxLines(nr.filled[1])); lines != 6 {
				t.Errorf("placed %d lines, want 6", lines)
			}
		})
	}
	t.Run("FlowText from a PageInitCallback", func(t *testing.T) {
		cb, _ := newFlowBuilder(t, "")
		te, err := cb.HTMLToText(`<html><body><p>` + charLines("A", 20) + `</p></body></html>`)
		if err != nil {
			t.Fatal(err)
		}
		inner, err := cb.HTMLToText(`<html><body><p>Zq</p></body></html>`)
		if err != nil {
			t.Fatal(err)
		}
		var nestedErr error
		calls := 0
		cb.PageInitCallback = func() {
			if calls++; calls == 2 {
				nestedErr = cb.FlowText(inner, &testRegions{sizes: []Region{wide("1000pt")}})
			}
		}
		if err := cb.OutputPagesFromText(te); err != nil {
			t.Fatal(err)
		}
		if calls < 2 || nestedErr == nil {
			t.Errorf("nested FlowText ran (%d page inits)", calls)
		}
		// Not flowing any more.
		if err := cb.FlowText(inner, &testRegions{sizes: []Region{wide("1000pt")}}); err != nil {
			t.Errorf("FlowText after the pages: %v", err)
		}
	})
}

// A block set beside a float that stays in the region before is rebuilt at
// full width, also when both regions have the same page number or none.
func TestFlowTextRebuildsABlockPartedFromItsFloat(t *testing.T) {
	body := `<div style="float: left; width: 80pt; height: 90pt">Fq</div><p>` + strings.Repeat("alpha beta ", 4) + `</p>` +
		`<div style="break-inside: avoid"><p>` + strings.Repeat("gamma delta ", 3) + `</p><p>Zq</p></div>`
	for _, samePage := range []bool{true, false} {
		t.Run(fmt.Sprintf("same page number %v", samePage), func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			r1, r2 := wide("40pt"), wide("1000pt")
			if samePage {
				r1.PageNum, r2.PageNum = 1, 1
			}
			tr := flow(t, cb, body, r1, r2)
			if len(tr.filled) != 2 {
				t.Fatalf("filled %d regions, want 2", len(tr.filled))
			}
			n := 0
			for _, l := range boxLines(tr.filled[1]) {
				if strings.Contains(l.text, "gamma") {
					n++
				}
			}
			if n != 1 {
				t.Errorf("the block beside the float takes %d lines in region 2, want 1 at full width", n)
			}
		})
	}
}

// An inside float takes its side from the region's page, not from the pages
// of the document: on a right (odd) page it sits at the left edge and the
// lines beside it are indented, on a left (even) one at the right edge.
// A mismatch between the two used to rebuild the block without end.
func TestFlowTextInsideFloatTakesTheRegionsParity(t *testing.T) {
	for _, pn := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint("PageNum ", pn), func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			body := `<div><div style="float: inside; width: 60pt; height: 40pt">Fq</div><p>` + strings.Repeat("alpha beta ", 30) + `</p></div><p>Zq</p>`
			r := wide("1000pt")
			r.PageNum = pn
			te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			tr := &testRegions{sizes: []Region{r}}
			done := make(chan error, 1)
			go func() { done <- cb.FlowText(te, tr) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("FlowText: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("FlowText did not finish")
			}
			indented := false
			for _, ind := range lineIndents(tr.filled[0].Box) {
				indented = indented || ind > 0
			}
			if right := pn%2 == 1; indented != right {
				t.Errorf("lines beside the float indented: %v, want %v", indented, right)
			}
		})
	}
}

// FlowText drops the running elements of its own Text only: those an earlier
// Text set for its pages stay, even when the flow names them again.
func TestFlowTextKeepsEarlierRunningElements(t *testing.T) {
	cb, _ := newFlowBuilder(t, runningFooterCSS+`.foot { position: running(foot) }`)
	te, err := cb.HTMLToText(`<html><body>` + runningFooterHTML + fillerParagraphs(3) + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	footer := cb.runningElements["pagefooter"]
	if footer == nil {
		t.Fatal("OutputPagesFromText's Text left no running footer")
	}
	flow(t, cb, `<p>Aq</p><div class="foot">Fq</div><div class="pagefooter">Pq</div>`, wide("1000pt"))
	if cb.runningElements["pagefooter"] != footer {
		t.Error("FlowText dropped the footer of the earlier Text")
	}
	if _, ok := cb.runningElements["foot"]; ok {
		t.Error("FlowText kept the running element of its own Text")
	}
}

// A Text built by ParseHTMLFromNode, the path XTS takes, drops its running
// elements in FlowText as HTMLToText's does.
func TestFlowTextParseHTMLFromNodeRunning(t *testing.T) {
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()

	cb, _ := newFlowBuilder(t, `.foot { position: running(foot) }`)
	doc, err := html.Parse(strings.NewReader(`<html><body><p>Aq</p><div class="foot">Fq</div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	te, err := cb.ParseHTMLFromNode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.FlowText(te, &testRegions{sizes: []Region{wide("1000pt")}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "does not support running elements") || len(cb.runningElements) > 0 {
		t.Errorf("warning logged: %v, running elements left: %d", strings.Contains(buf.String(), "does not support running elements"), len(cb.runningElements))
	}
}
