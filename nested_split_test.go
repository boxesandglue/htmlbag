package htmlbag

import "testing"

// A box inside a container that is not the body's only child splits across
// pages, whether the boxes around it are bordered or transparent. A page is
// 13 lines; Before takes one of them.
func TestNestedBoxesSplit(t *testing.T) {
	cases := []struct {
		name, html string
		minFirst   int // lines of the box on page 1 at least
	}{
		{"div in div", `<div><div>` + nestParas("Line", 20) + `</div></div>`, 12},
		{"div in div in div", `<div><div><div>` + nestParas("Line", 20) + `</div></div></div>`, 12},
		{"bordered in div", `<div><div class="b">` + nestParas("Line", 20) + `</div></div>`, 11},
		{"div in bordered", `<div class="b"><div>` + nestParas("Line", 20) + `</div></div>`, 11},
		{"bordered in bordered", `<div class="b"><div class="b">` + nestParas("Line", 20) + `</div></div>`, 11},
		{"div after a sibling in div", `<div><p>Zq</p><div>` + nestParas("Line", 20) + `</div></div>`, 11},
		{"bordered after a sibling in div", `<div><p>Zq</p><div class="b">` + nestParas("Line", 20) + `</div></div>`, 10},
		{"paragraph in main", `<main><p>` + nestLines("Line", 20) + `</p></main>`, 12},
		{"paragraph after a sibling in main", `<main><p>Zq</p><p>` + nestLines("Line", 20) + `</p></main>`, 11},
		{"list item after a sibling", `<ul><li>Zq</li><li>` + nestLines("Line", 20) + `</li></ul>`, 11},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pages, on := linePages(t, `<p>Before</p>`+c.html, "Line")
			if len(on) != 20 {
				t.Fatalf("placed %d lines, want 20", len(on))
			}
			if pages != 2 {
				t.Errorf("%d pages, want 2", pages)
			}
			if n := countOn(on, 1); n < c.minFirst {
				t.Errorf("page 1 holds %d lines of the box, want at least %d", n, c.minFirst)
			}
		})
	}
}

// A header before main is a sibling like any other: the paragraph in main
// still splits.
func TestNestedParagraphAfterHeaderSplits(t *testing.T) {
	pages, on := linePages(t, `<header><p>H</p></header><main><p>Zq</p><p>`+nestLines("Line", 20)+`</p></main>`, "Line")
	if pages != 2 || countOn(on, 1) < 11 {
		t.Errorf("%d pages, %d lines on page 1; want 2 pages and at least 11", pages, countOn(on, 1))
	}
}

// A paragraph of 6 lines after 8 lines of filler splits 4 and 2 at the top
// level (widows: 2); inside a container, a list item included, it does the
// same.
func TestNestedParagraphSplitsByLines(t *testing.T) {
	filler := `<p>` + nestLines("F", 8) + `</p>`
	cases := []struct{ name, html string }{
		{"top level", `<p>` + nestLines("L", 6) + `</p>`},
		{"in div", `<div><p>` + nestLines("L", 6) + `</p></div>`},
		{"in div after a sibling", `<div><p>Zq</p><p>` + nestLines("L", 6) + `</p></div>`},
		{"list item", `<ul><li>` + nestLines("L", 6) + `</li></ul>`},
		{"numbered list item", `<ol><li>` + nestLines("L", 6) + `</li></ol>`},
		{"list item without marker", `<ul style="list-style: none"><li>` + nestLines("L", 6) + `</li></ul>`},
		{"in div in div", `<div><div><p>` + nestLines("L", 6) + `</p></div></div>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := filler + c.html
			_, on := linePages(t, html, "L")
			if len(on) != 6 {
				t.Fatalf("placed %d lines, want 6", len(on))
			}
			want := 4
			if countOn(on, 1) != want || countOn(on, 2) != 6-want {
				t.Errorf("pages of the lines %v, want %d on page 1 and %d on page 2", on, want, 6-want)
			}
		})
	}
}

// orphans and widows count the lines of the paragraph they are set on, also
// inside a container.
func TestNestedParagraphKeepsItsWidows(t *testing.T) {
	html := `<p>` + nestLines("F", 8) + `</p><div><p style="widows: 3">` + nestLines("L", 6) + `</p></div>`
	_, on := linePages(t, html, "L")
	if countOn(on, 1) != 3 || countOn(on, 2) != 3 {
		t.Errorf("pages of the lines %v, want 3 on page 1 and 3 on page 2", on)
	}
}

// orphans and widows apply to lines, not to the blocks of a container (CSS
// Fragmentation 3 §4.4): a container whose first block fits does not move on
// whole because only one block of it would stay behind.
func TestContainerBlocksAreNotOrphans(t *testing.T) {
	html := `<p>Before</p><main><p>` + nestLines("F", 8) + `</p><p>` + nestLines("L", 6) + `</p></main>`
	pages, on := linePages(t, html, "L")
	if pages != 2 || countOn(on, 1) != 4 {
		t.Errorf("%d pages, lines on pages %v; want 2 pages and 4 lines on page 1", pages, on)
	}
	_, fon := linePages(t, html, "F")
	if countOn(fon, 1) != 8 {
		t.Errorf("filler lines on pages %v, want all 8 on page 1", fon)
	}
}

// break-inside: avoid on a nested container keeps it whole; the blocks before
// it stay.
func TestNestedBreakInsideAvoid(t *testing.T) {
	html := `<p>` + nestLines("F", 8) + `</p><div><p>Zq</p><div style="break-inside: avoid">` + nestParas("L", 6) + `</div></div>`
	_, on := linePages(t, html, "L")
	if countOn(on, 2) != 6 {
		t.Errorf("lines on pages %v, want all 6 on page 2", on)
	}
	if _, page := nestPageOf(t, html, "Zq"); page != 1 {
		t.Errorf("Zq on page %d, want 1", page)
	}
}
