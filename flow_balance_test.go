package htmlbag

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// pagedRegions hands out pages of regions, the last layout as often as
// needed, and names the rest of the current page in Remaining.
type pagedRegions struct {
	testRegions
	layouts [][]Region
	balance bool
	page    int // 1-based page of the current region
	idx     int // index of the current region on its page
	// pages is the page of every region handed out.
	pages []int
	// remaining counts the calls of Remaining.
	remaining int
}

func (pr *pagedRegions) layout(page int) []Region {
	return pr.layouts[min(page, len(pr.layouts))-1]
}

func (pr *pagedRegions) Next(brk string) (Region, error) {
	pr.brks = append(pr.brks, brk)
	pr.order = append(pr.order, "next")
	switch {
	case pr.page == 0:
		pr.page, pr.idx = 1, 0
	case brk == "page" || pr.idx+1 >= len(pr.layout(pr.page)):
		pr.page, pr.idx = pr.page+1, 0
	default:
		pr.idx++
	}
	if pr.page > 50 {
		return Region{}, fmt.Errorf("the flow does not end")
	}
	rg := pr.layout(pr.page)[pr.idx]
	rg.PageNum = pr.page
	pr.pages = append(pr.pages, pr.page)
	return rg, nil
}

func (pr *pagedRegions) Remaining() []Region {
	pr.remaining++
	var out []Region
	for _, rg := range pr.layout(pr.page)[pr.idx+1:] {
		rg.PageNum = pr.page
		out = append(out, rg)
	}
	return out
}

func (pr *pagedRegions) BalanceEnd() bool { return pr.balance }

// plainRegions is a pagedRegions without Remaining.
type plainRegions struct{ pr *pagedRegions }

func (p plainRegions) Next(brk string) (Region, error) { return p.pr.Next(brk) }
func (p plainRegions) Filled(f Filled) error           { return p.pr.Filled(f) }

// columns is a page of n regions of width w and height h side by side, their
// tops at 1000pt.
func columns(n int, w, h string) []Region {
	var out []Region
	for i := range n {
		out = append(out, Region{Width: sp(w), Height: sp(h), Left: bag.ScaledPoint(i) * (sp(w) + sp("10pt")), Top: sp("1000pt")})
	}
	return out
}

// numbered is a paragraph body of the lines prefix00, prefix01, … from from on.
func numbered(prefix string, from, n int) string {
	var s []string
	for i := from; i < from+n; i++ {
		s = append(s, fmt.Sprintf("%s%02d", prefix, i))
	}
	return strings.Join(s, "<br>")
}

// flowPaged pours body into pr.
func flowPaged(t *testing.T, cb *CSSBuilder, body string, r Regions) {
	t.Helper()
	te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.FlowText(te, r); err != nil {
		t.Fatalf("FlowText: %v", err)
	}
}

// balanced flows body with css into pages of layouts, balanced or not.
func balanced(t *testing.T, css, body string, balance bool, layouts ...[]Region) *pagedRegions {
	t.Helper()
	cb, _ := newFlowBuilder(t, css)
	pr := &pagedRegions{layouts: layouts, balance: balance}
	flowPaged(t, cb, body, pr)
	return pr
}

// regionTexts lists the texts of the lines in each region.
func regionTexts(pr *pagedRegions) [][]string {
	var out [][]string
	for _, f := range pr.filled {
		var texts []string
		for _, l := range boxLines(f) {
			texts = append(texts, l.text)
		}
		out = append(out, texts)
	}
	return out
}

// dump describes everything FlowText handed back, to compare two runs.
func dump(pr *pagedRegions) string {
	var sb strings.Builder
	for i, f := range pr.filled {
		fmt.Fprintf(&sb, "region %d: used %d margin %d box %dx%d\n", i+1, f.Used, f.MarginAfter, f.Box.Width, f.Box.Height)
		for _, l := range boxLines(f) {
			fmt.Fprintf(&sb, "  %d %d %d %q\n", l.top, l.bottom, l.width, l.text)
		}
		fmt.Fprintf(&sb, "  %s\n", fragmentsString(f.Fragments))
	}
	fmt.Fprintf(&sb, "brks %q\n", pr.brks)
	return sb.String()
}

// lineCounts is the number of lines in each region.
func lineCounts(pr *pagedRegions) []int {
	var out []int
	for _, texts := range regionTexts(pr) {
		out = append(out, len(texts))
	}
	return out
}

// allLines is the texts of every region in order.
func allLines(pr *pagedRegions) []string {
	var out []string
	for _, texts := range regionTexts(pr) {
		out = append(out, texts...)
	}
	return out
}

func requireCounts(t *testing.T, pr *pagedRegions, want ...int) {
	t.Helper()
	if got := lineCounts(pr); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("lines per region %v, want %v", got, want)
	}
}

// requireOrder checks that the lines prefix00 … are all placed once, in
// order.
func requireOrder(t *testing.T, pr *pagedRegions, prefix string, n int) {
	t.Helper()
	var got []string
	for _, l := range allLines(pr) {
		if strings.HasPrefix(l, prefix) && len(l) >= len(prefix)+2 {
			got = append(got, l[:len(prefix)+2])
		}
	}
	var want []string
	for i := range n {
		want = append(want, fmt.Sprintf("%s%02d", prefix, i))
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("lines\n%v\nwant\n%v", got, want)
	}
}

// Two equal regions on the last page share its lines: the first page fills
// as before, the last one ends level.
func TestFlowTextBalancesTheLastPage(t *testing.T) {
	two := columns(2, "160pt", "120pt")
	body := `<p>` + numbered("L", 0, 26) + `</p>`
	pr := balanced(t, "", body, true, two)
	requireCounts(t, pr, 10, 10, 3, 3)
	requireOrder(t, pr, "L", 26)
	if a, b := pr.filled[2].Used, pr.filled[3].Used; a-b > sp("0.5pt") || b-a > sp("0.5pt") {
		t.Errorf("last page ends at %s and %s, want level", a, b)
	}
	for _, f := range pr.filled[2:] {
		if f.Used > 3*charLine {
			t.Errorf("a balanced region is filled to %s, more than three lines", f.Used)
		}
	}
	off := balanced(t, "", body, false, two)
	requireCounts(t, off, 10, 10, 6)

	t.Run("three regions", func(t *testing.T) {
		pr := balanced(t, "", `<p>`+numbered("L", 0, 39)+`</p>`, true, columns(3, "100pt", "120pt"))
		requireCounts(t, pr, 10, 10, 10, 3, 3, 3)
		requireOrder(t, pr, "L", 39)
	})
	t.Run("several blocks", func(t *testing.T) {
		body := `<p>` + numbered("L", 0, 12) + `</p><p>` + numbered("M", 0, 4) + `</p>`
		pr := balanced(t, "", body, true, two)
		requireCounts(t, pr, 8, 8)
		requireOrder(t, pr, "L", 12)
		requireOrder(t, pr, "M", 4)
	})
}

// Where there is nothing to spread, the flow ends as it does without
// balancing: a full last page, a single line, a page of one region.
func TestFlowTextBalanceLeavesWhatItCannotSpread(t *testing.T) {
	cases := []struct {
		name, body string
		layouts    [][]Region
	}{
		{"a full last page", `<p>` + numbered("L", 0, 40) + `</p>`, [][]Region{columns(2, "160pt", "120pt")}},
		{"a single line", `<p>Lq</p>`, [][]Region{columns(2, "160pt", "120pt")}},
		{"one region per page", `<p>` + numbered("L", 0, 14) + `</p>`, [][]Region{columns(1, "160pt", "120pt")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			on := balanced(t, "", c.body, true, c.layouts...)
			off := balanced(t, "", c.body, false, c.layouts...)
			if a, b := dump(on), dump(off); a != b {
				t.Errorf("balanced\n%s\nwant as filled\n%s", a, b)
			}
		})
	}
}

// The balance runs the paginator, so widows and orphans hold at the height
// it chooses.
func TestFlowTextBalanceKeepsWidowsAndOrphans(t *testing.T) {
	two := columns(2, "160pt", "120pt")
	body := `<p>` + numbered("L", 0, 27) + `</p>`
	requireCounts(t, balanced(t, "", body, true, two), 10, 10, 4, 3)
	pr := balanced(t, "p { widows: 4; orphans: 3 }", body, true, two)
	requireCounts(t, pr, 10, 10, 3, 4)
	requireOrder(t, pr, "L", 27)
}

// A table that crosses the start of the last page is balanced like any other
// block: its rows spread over the regions, each once, the header repeated.
func TestFlowTextBalancesATableAcrossThePageStart(t *testing.T) {
	var rows strings.Builder
	for i := range 16 {
		fmt.Fprintf(&rows, `<tr><td>R%02d</td></tr>`, i)
	}
	body := `<p>` + numbered("L", 0, 6) + `</p><table><thead><tr><th>Hq</th></tr></thead><tbody>` + rows.String() + `</tbody></table>`
	two := columns(2, "160pt", "120pt")
	pr := balanced(t, "", body, true, two)
	// Rows are 14pt high here. Page 1 as filled, page 2 the remaining
	// seven rows, under a header in each region.
	requireCounts(t, pr, 9, 8, 5, 4)
	requireCounts(t, balanced(t, "", body, false, two), 9, 8, 8)
	requireOrder(t, pr, "R", 16)
	for _, i := range []int{1, 2, 3} {
		if texts := regionTexts(pr)[i]; texts[0] != "Hq" {
			t.Errorf("region %d starts with %q, want the header", i+1, texts[0])
		}
	}
}

// A block that moves into a region of another width on the last page
// restarts the paginator, which gives the balance up: the page fills as
// without it. A paragraph split there is re-broken in place, in a trial as
// in the fill, so that page is balanced. So is a last page narrower than the
// ones before with regions of one width, after the rest is rebuilt at it.
func TestFlowTextBalanceAndWidthChanges(t *testing.T) {
	text := strings.Repeat("alpha beta gamma delta ", 4)
	var paras []string
	for i := range 14 {
		paras = append(paras, fmt.Sprintf(`<p>P%02d %s</p>`, i, text))
	}
	body := strings.Join(paras, "")
	two := columns(2, "160pt", "120pt")
	mixed := []Region{
		{Width: sp("160pt"), Height: sp("120pt"), Top: sp("1000pt")},
		{Width: sp("100pt"), Height: sp("120pt"), Left: sp("170pt"), Top: sp("1000pt")},
	}
	t.Run("a block moving into another width", func(t *testing.T) {
		const css = `p { break-inside: avoid }`
		on := balanced(t, css, body, true, two, mixed)
		off := balanced(t, css, body, false, two, mixed)
		if a, b := dump(on), dump(off); a != b {
			t.Errorf("balanced\n%s\nwant as filled\n%s", a, b)
		}
	})
	t.Run("a paragraph re-broken at another width", func(t *testing.T) {
		on := balanced(t, "", body, true, two, mixed)
		requireOrder(t, on, "P", 14)
		// P11 to P13 on the last page: two in the wide region, the third
		// re-broken in the narrow one.
		requireCounts(t, on, 9, 9, 9, 10, 6, 5)
		if w := boxLines(on.filled[5])[0].width; w != sp("100pt") {
			t.Errorf("the narrow region's lines are %s wide, want 100pt", w)
		}
		requireCounts(t, balanced(t, "", body, false, two, mixed), 9, 9, 9, 10, 9)
	})
	t.Run("a narrower last page", func(t *testing.T) {
		narrow := columns(2, "120pt", "120pt")
		on := balanced(t, "", body, true, two, narrow)
		off := balanced(t, "", body, false, two, narrow)
		requireOrder(t, on, "P", 14)
		requireCounts(t, on, 9, 9, 10, 10, 6, 6)
		requireCounts(t, off, 9, 9, 10, 10, 10, 2)
	})
}

// Regions of different heights end on one line, or at their own foot above
// it.
func TestFlowTextBalanceFramesOfDifferentHeights(t *testing.T) {
	last := []Region{
		{Width: sp("160pt"), Height: sp("120pt"), Top: sp("1000pt")},
		{Width: sp("160pt"), Height: sp("60pt"), Left: sp("170pt"), Top: sp("1000pt")},
	}
	body := `<p>` + numbered("L", 0, 32) + `</p>`
	pr := balanced(t, "", body, true, columns(2, "160pt", "120pt"), last)
	// Twelve lines on the last page: the short region takes its five, the
	// tall one seven.
	requireCounts(t, pr, 10, 10, 7, 5)
	requireOrder(t, pr, "L", 32)
}

// An occupied region that starts lower ends level with its neighbour.
func TestFlowTextBalanceOccupiedRegion(t *testing.T) {
	last := []Region{
		{Width: sp("160pt"), Height: sp("96pt"), Top: sp("976pt"), Occupied: true},
		{Width: sp("160pt"), Height: sp("120pt"), Left: sp("170pt"), Top: sp("1000pt")},
	}
	body := `<p>` + numbered("L", 0, 26) + `</p>`
	pr := balanced(t, "", body, true, columns(2, "160pt", "120pt"), last)
	requireCounts(t, pr, 10, 10, 2, 4)
	requireOrder(t, pr, "L", 26)
	a := last[0].Top - pr.filled[2].Used
	b := last[1].Top - pr.filled[3].Used
	if a != b {
		t.Errorf("the regions end at %s and %s, want level", a, b)
	}
}

// A side float is laid out inside the trials as in the fill: it stays with
// the block beside it and is placed once.
func TestFlowTextBalanceSideFloat(t *testing.T) {
	const css = `.f { float: left; width: 40pt; height: 30pt; line-height: 12pt; font-size: 10pt }`
	body := `<p>` + numbered("L", 0, 20) + `</p><div class="f">Fq</div><p>` + numbered("M", 0, 6) + `</p>`
	pr := balanced(t, css, body, true, columns(2, "160pt", "120pt"))
	requireOrder(t, pr, "L", 20)
	requireOrder(t, pr, "M", 6)
	floats := 0
	for i, texts := range regionTexts(pr) {
		for _, s := range texts {
			if s == "Fq" {
				floats++
				if i != 2 {
					t.Errorf("the float is in region %d, want 3", i+1)
				}
			}
		}
	}
	if floats != 1 {
		t.Fatalf("the float is placed %d times, want once", floats)
	}
	if n := len(pr.filled); n != 4 || pr.filled[3].Used == 0 {
		t.Fatalf("the last page is not balanced:\n%s", dump(pr))
	}
	if pr.filled[2].Used > sp("36pt") {
		t.Errorf("the first region of the last page is filled to %s, want at most 36pt", pr.filled[2].Used)
	}
}

// Headings and anchors are recorded once, where the real run places them,
// the element callback runs as often as without balancing, and a dropped
// footnote is warned about once: a trial records, calls and logs nothing.
func TestFlowTextBalanceMarksOnce(t *testing.T) {
	body := `<p>` + numbered("L", 0, 23) + `</p><h1 id="h">Hq</h1><p>` + numbered("M", 0, 2) + `<span style="float: top">Nq</span></p>`
	run := func(balance bool) (*CSSBuilder, *pagedRegions, int, string) {
		cb, _ := newFlowBuilder(t, "")
		calls := 0
		cb.ElementCallback = func(ElementEvent) { calls++ }
		var logs bytes.Buffer
		old := bag.Logger
		bag.Logger = slog.New(slog.NewTextHandler(&logs, nil))
		defer func() { bag.Logger = old }()
		pr := &pagedRegions{layouts: [][]Region{columns(2, "160pt", "120pt")}, balance: balance}
		flowPaged(t, cb, body, pr)
		return cb, pr, calls, logs.String()
	}
	cb, pr, calls, logs := run(true)
	_, _, offCalls, offLogs := run(false)
	requireCounts(t, pr, 10, 10, 3, 3)
	if calls != offCalls {
		t.Errorf("element callback ran %d times, %d without balancing", calls, offCalls)
	}
	const warning = "does not support float: top"
	if n, m := strings.Count(logs, warning), strings.Count(offLogs, warning); n != 1 || m != 1 {
		t.Errorf("float: top warned about %d times, %d without balancing, want once", n, m)
	}
	if len(cb.Headings) != 1 || cb.Headings[0].Page != 2 {
		t.Fatalf("headings %+v, want one on page 2", cb.Headings)
	}
	// The heading is the fourth line of the last page: the first of its
	// second region, where filling would set it below three lines.
	if want := pr.layouts[0][1].Top; cb.Headings[0].Y != want {
		t.Errorf("heading at %s, want the top of the second region %s", cb.Headings[0].Y, want)
	}
	if len(cb.Anchors) != 1 || cb.Anchors[0].Page != 2 {
		t.Errorf("anchors %+v, want one on page 2", cb.Anchors)
	}
}

// Without the opt-in, and for a supplier without Remaining, FlowText fills
// as it always has.
func TestFlowTextBalanceOff(t *testing.T) {
	bodies := []string{
		`<p>` + numbered("L", 0, 26) + `</p>`,
		`<p>` + numbered("L", 0, 8) + `</p><p style="break-before: page">` + numbered("M", 0, 30) + `</p>`,
	}
	for i, body := range bodies {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			layouts := [][]Region{columns(2, "160pt", "120pt")}
			cb, _ := newFlowBuilder(t, "")
			plain := &testRegions{sizes: layouts[0]}
			plainPaged := &pagedRegions{layouts: layouts}
			flowPaged(t, cb, body, plainRegions{plainPaged})
			off := balanced(t, "", body, false, layouts...)
			if off.remaining != 0 {
				t.Errorf("Remaining called %d times without the opt-in", off.remaining)
			}
			if a, b := dump(off), dump(plainPaged); a != b {
				t.Errorf("opt-in off\n%s\nwant as without Remaining\n%s", a, b)
			}
			_ = plain
		})
	}
}

// The real run goes on from its own cursor with the regions capped at the
// line: it comes out exactly as a fill into regions that end there, here
// after a rest rebuilt at the narrower width of the last page, with ids and
// a side float. A serial, a rebuild mark or a carry left behind by a trial
// would part the two.
func TestFlowTextBalanceRealRunIsAFill(t *testing.T) {
	text := strings.Repeat("alpha beta gamma delta ", 4)
	var paras []string
	for i := range 9 {
		if i == 7 {
			paras = append(paras, `<div style="float: left; width: 30pt; height: 20pt">Fq</div>`)
		}
		paras = append(paras, fmt.Sprintf(`<p id="p%d">P%02d %s</p>`, i, i, text))
	}
	body := strings.Join(paras, "")
	two, narrow := columns(2, "160pt", "120pt"), columns(2, "120pt", "120pt")
	on := balanced(t, "", body, true, two, narrow)
	var line bag.ScaledPoint
	for i, f := range on.filled {
		if on.pages[i] == 2 {
			line = max(line, f.Used)
		}
	}
	if line == 0 || line >= sp("100pt") {
		t.Fatalf("the last page is not balanced:\n%s", dump(on))
	}
	capped := columns(2, "120pt", "120pt")
	for i := range capped {
		capped[i].Height = line
	}
	fill := balanced(t, "", body, false, two, capped)
	if a, b := dump(on), dump(fill); a != b {
		t.Errorf("balanced\n%s\nwant as a fill to the line\n%s", a, b)
	}
}

// A trial runs from a copy of the cursor and the page of the group's build:
// when one runs and gives up, here at a forced break inside a block of the
// last page, the flow comes out exactly as without balancing, including
// after a block rebuilt at the narrower width of the last page and beside a
// float.
func TestFlowTextBalanceTrialLeavesNoState(t *testing.T) {
	text := strings.Repeat("alpha beta gamma delta ", 4)
	var paras []string
	for i := range 12 {
		paras = append(paras, fmt.Sprintf(`<p id="p%d">P%02d %s</p>`, i, i, text))
	}
	body := strings.Join(paras, "") +
		`<div><p>` + numbered("M", 0, 2) + `</p><div style="float: left; width: 30pt; height: 20pt">Fq</div>` +
		`<p style="break-before: column">` + numbered("N", 0, 2) + `</p></div>`
	layouts := [][]Region{columns(2, "160pt", "120pt"), columns(2, "120pt", "120pt")}
	on := balanced(t, "", body, true, layouts...)
	off := balanced(t, "", body, false, layouts...)
	if on.remaining == 0 {
		t.Fatal("no trial ran")
	}
	if a, b := dump(on), dump(off); a != b {
		t.Errorf("after a trial\n%s\nwant as without\n%s", a, b)
	}
}
