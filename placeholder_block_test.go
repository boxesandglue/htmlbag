package htmlbag

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// pendingBox registers a pre-rendered box of three 12pt lines Bq1 to Bq3,
// 40pt wide, under id, and returns it.
func pendingBox(t *testing.T, cb *CSSBuilder, id string) *node.VList {
	t.Helper()
	te, err := cb.HTMLToText(`<html><body><p>Bq1<br>Bq2<br>Bq3</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, sp("40pt"))
	if err != nil {
		t.Fatal(err)
	}
	cb.PendingVLists[id] = vl
	return vl
}

// placeholderPages renders body on charCSS pages plus css, with the box of
// pendingBox under "b".
func placeholderPages(t *testing.T, css, body string) ([]*document.Page, *node.VList) {
	t.Helper()
	cb, fe := newFlowBuilder(t, css)
	pending := pendingBox(t, cb, "b")
	te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	return fe.Doc.Pages, pending
}

// placeholderShifts lists the ShiftX of the placeholder boxes in the lists.
func placeholderShifts(lists ...*node.VList) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				if isPlaceholderBox(v) {
					out = append(out, v.ShiftX)
				}
				walk(v.List)
			case *node.HList:
				walk(v.List)
			}
		}
	}
	for _, l := range lists {
		walk(l)
	}
	return out
}

// requireUntouched fails when the registered box itself, not a copy of it,
// is in the output lists, or when it was linked into a list.
func requireUntouched(t *testing.T, pending *node.VList, out ...*node.VList) {
	t.Helper()
	var found bool
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil && !found; n = n.Next() {
			if n == node.Node(pending) {
				found = true
				return
			}
			switch v := n.(type) {
			case *node.VList:
				walk(v.List)
			case *node.HList:
				walk(v.List)
			}
		}
	}
	for _, l := range out {
		walk(l)
	}
	if found {
		t.Error("the registered box itself is in the output, not a copy")
	}
	if pending.Prev() != nil || pending.Next() != nil {
		t.Error("the registered box was linked into a list")
	}
	var sb strings.Builder
	collectGlyphs(pending.List, &sb)
	if sb.String() != "Bq1Bq2Bq3" {
		t.Errorf("the registered box now reads %q", sb.String())
	}
}

// pageLists returns the lists placed on the pages.
func pageLists(pages []*document.Page) []*node.VList {
	var out []*node.VList
	for _, pg := range pages {
		for _, o := range pg.Objects {
			if o.Vlist != nil {
				out = append(out, o.Vlist)
			}
		}
	}
	return out
}

// A placeholder among blocks is set as one block of the box's own width,
// moved by its margin-left. Its margins collapse with the neighbours'.
func TestPlaceholderIsABlockOnAPage(t *testing.T) {
	body := `<p style="margin-bottom: 4pt">A</p><div data-vlist-id="b" style="margin: 10pt 0 6pt 20pt"></div><p style="margin-top: 8pt">C</p>`
	pages, pending := placeholderPages(t, "", body)
	lines := placedLines(pages)
	a, b1, b3, c := firstLine(t, lines, "A"), firstLine(t, lines, "Bq1"), firstLine(t, lines, "Bq3"), firstLine(t, lines, "C")
	if gap := a.bottom - b1.top; gap != sp("10pt") {
		t.Errorf("A to the box: %s, want the collapsed 10pt", gap)
	}
	if gap := b3.bottom - c.top; gap != sp("8pt") {
		t.Errorf("the box to C: %s, want the collapsed 8pt", gap)
	}
	if b1.width != sp("40pt") {
		t.Errorf("the box is %s wide, want its own 40pt", b1.width)
	}
	if got := placeholderShifts(pageLists(pages)...); len(got) != 1 || got[0] != sp("20pt") {
		t.Errorf("placeholder boxes shifted by %v, want one by 20pt", got)
	}
	requireUntouched(t, pending, pageLists(pages)...)
}

// The box is never broken: with room for two of its lines left, it moves on
// to the next page whole, also when it is the only block. A box taller than
// a page is placed anyway.
func TestPlaceholderMovesOnWhole(t *testing.T) {
	for name, body := range map[string]string{
		"after a paragraph": `<p>` + charLines("A", 11) + `</p><div data-vlist-id="b"></div>`,
		"in a div":          `<p>` + charLines("A", 11) + `</p><div><div data-vlist-id="b"></div><p>C</p></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			pages, _ := placeholderPages(t, "", body)
			lines := placedLines(pages)
			for _, m := range []string{"Bq1", "Bq3"} {
				if l := firstLine(t, lines, m); l.page != 2 {
					t.Errorf("%s is on page %d, want 2", m, l.page)
				}
			}
		})
	}
	t.Run("the only block in a region", func(t *testing.T) {
		cb, _ := newFlowBuilder(t, "")
		pendingBox(t, cb, "b")
		tr := flow(t, cb, `<div data-vlist-id="b"></div>`, wide("24pt"), wide("1000pt"))
		if b1, b3 := regionOf(t, tr, "Bq1"), regionOf(t, tr, "Bq3"); b1 != 1 || b3 != 1 {
			t.Errorf("the box is in regions %d to %d, want region 1 whole", b1, b3)
		}
	})
	t.Run("taller than a page", func(t *testing.T) {
		pages, _ := placeholderPages(t, "@page { size: 200pt 60pt; margin: 10pt }", `<div data-vlist-id="b"></div><p>C</p>`)
		lines := placedLines(pages)
		if b1, b3 := firstLine(t, lines, "Bq1"), firstLine(t, lines, "Bq3"); b1.page != 1 || b3.page != 1 {
			t.Errorf("the box is on pages %d to %d, want page 1 whole", b1.page, b3.page)
		}
	})
}

// A page of another width rebuilds the blocks not placed yet, the box among
// them: it is set again from a copy and keeps its own width.
func TestPlaceholderOnAPageOfAnotherWidth(t *testing.T) {
	css := `@page :first { margin-left: 60pt }`
	pages, pending := placeholderPages(t, css, `<p>`+charLines("A", 12)+`</p><div data-vlist-id="b"></div><p>C</p>`)
	lines := placedLines(pages)
	b1 := firstLine(t, lines, "Bq1")
	if b1.page != 2 || b1.width != sp("40pt") {
		t.Errorf("the box is on page %d, %s wide; want page 2, 40pt", b1.page, b1.width)
	}
	requireUntouched(t, pending, pageLists(pages)...)
}

// In regions: the box is a flow child with its id in Fragments and on its
// box; a region of another width after it rebuilds it from a copy.
func TestPlaceholderInRegions(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	pending := pendingBox(t, cb, "b")
	body := `<p>` + charLines("A", 4) + `</p><div id="box" data-vlist-id="b"></div><p>C</p>`
	tr := flow(t, cb, body, wide("60pt"), Region{Width: sp("100pt"), Height: sp("1000pt")})
	if len(tr.filled) != 2 {
		t.Fatalf("filled %d regions, want 2", len(tr.filled))
	}
	want := "{\"box\" 1 0+36 continued=false continues=false} {\"\" 2 36+12 continued=false continues=false}"
	if got := fragmentsString(tr.filled[1].Fragments); got != want {
		t.Errorf("region 2 fragments\n got %s\nwant %s", got, want)
	}
	if n := boxIDs(tr.filled[1])["box"]; n != 1 {
		t.Errorf("%d boxes carry the id, want 1", n)
	}
	if lines := boxLines(tr.filled[1]); len(lines) < 3 || lines[0].text != "Bq1" || lines[0].width != sp("40pt") {
		t.Errorf("region 2 starts with %+v, want the box's three lines, 40pt wide", lines)
	}
	requireUntouched(t, pending, tr.filled[0].Box, tr.filled[1].Box)
}

// break-before, break-after and break-after: avoid work on the box as on any
// block.
func TestPlaceholderBreaks(t *testing.T) {
	cases := []struct {
		name, body string
		brks       []string
		// region and line: where C lands.
		region int
	}{
		{"break-before", `<p>A</p><div data-vlist-id="b" style="break-before: column"></div><p>C</p>`, []string{"", "column"}, 2},
		{"break-after", `<p>A</p><div data-vlist-id="b" style="break-after: page"></div><p>C</p>`, []string{"", "page"}, 2},
		// The box fits under A, but with C it does not: both move on.
		{"break-after: avoid", `<p>` + charLines("A", 2) + `</p><div data-vlist-id="b" style="break-after: avoid"></div><p>C</p>`, []string{"", ""}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			pendingBox(t, cb, "b")
			tr := flow(t, cb, c.body, wide("62pt"), wide("1000pt"))
			if strings.Join(tr.brks, ",") != strings.Join(c.brks, ",") {
				t.Errorf("Next got %q, want %q", tr.brks, c.brks)
			}
			if r := regionOf(t, tr, "C"); r != c.region {
				t.Errorf("C is in region %d, want %d", r, c.region)
			}
			if c.name == "break-after: avoid" {
				if r := regionOf(t, tr, "Bq1"); r != 2 {
					t.Errorf("the box is in region %d, want 2 with C", r)
				}
			}
		})
	}
}

// A second placeholder with the same id is a caller's error, and one in the
// text of a paragraph is not set: both are a warning naming the id.
func TestPlaceholderWarnings(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"same id twice", `<div data-vlist-id="b"></div><div data-vlist-id="b"></div>`, "used more than once"},
		{"in a paragraph", `<p>A <span data-vlist-id="b"></span> B</p>`, "inside the text of a paragraph"},
		{"unknown id", `<div data-vlist-id="nope"></div>`, "has no pre-rendered box"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			old := bag.Logger
			bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
			defer func() { bag.Logger = old }()
			cb, _ := newFlowBuilder(t, "")
			pendingBox(t, cb, "b")
			flow(t, cb, c.body, wide("1000pt"))
			if log := buf.String(); !strings.Contains(log, c.want) || !strings.Contains(log, "id=") {
				t.Errorf("log %q, want a warning with %q and the id", log, c.want)
			}
		})
	}
}

// regionOf returns the region, counted from 1, of the first line that
// contains mark.
func regionOf(t *testing.T, tr *testRegions, mark string) int {
	t.Helper()
	for i, f := range tr.filled {
		for _, l := range boxLines(f) {
			if strings.Contains(l.text, mark) {
				return i + 1
			}
		}
	}
	t.Fatalf("no region holds %q", mark)
	return 0
}

// An occupied region too small for the box is passed over once, so the box and
// the paragraph after it go to the next region; one with room keeps the box.
func TestPlaceholderInAnOccupiedRegion(t *testing.T) {
	for _, c := range []struct {
		name, first string
		region      int
	}{
		{"too small", "24pt", 2},
		{"with room", "100pt", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			pendingBox(t, cb, "b")
			tr := flow(t, cb, `<div data-vlist-id="b"></div><p>C</p>`, occupied(wide(c.first)), wide("1000pt"))
			for _, m := range []string{"Bq1", "Bq3", "C"} {
				if r := regionOf(t, tr, m); r != c.region {
					t.Errorf("%s is in region %d, want %d", m, r, c.region)
				}
			}
		})
	}
}
