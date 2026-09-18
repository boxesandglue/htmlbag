package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

const borderModelCSS = `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 10pt }
td { border: 1pt solid black; padding: 2pt }`

const borderModelHTML = `<table class="t"><tr><td>one</td><td>two</td></tr><tr><td>three</td><td>four</td></tr></table>`

// tableRows returns the row HLists of the first table on the pages.
func tableRows(pages []*document.Page) []*node.HList {
	var rows []*node.HList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if origin, _ := v.GetAttribute("origin"); origin == "table row" {
					rows = append(rows, v)
					continue
				}
				walk(v.List)
			case *node.VList:
				walk(v.List)
			}
		}
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist.List)
			}
		}
	}
	return rows
}

// spacingKerns counts the border spacing kerns in a row.
func spacingKerns(hl *node.HList) int {
	n := 0
	for e := hl.List; e != nil; e = e.Next() {
		if k, ok := e.(*node.Kern); ok {
			if origin, _ := k.GetAttribute("origin"); origin == "border spacing" {
				n++
			}
		}
	}
	return n
}

// TestBorderModelSeparate checks that border-collapse: separate keeps the
// cells apart by border-spacing: a kern before the first cell and after
// every cell, and the spacing below every row.
func TestBorderModelSeparate(t *testing.T) {
	css := borderModelCSS + "\ntable.t { border-collapse: separate; border-spacing: 6pt 3pt }"
	rows := tableRows(renderHTMLPages(t, css, borderModelHTML))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if got := spacingKerns(row); got != 3 {
			t.Errorf("row %d: got %d spacing kerns, want 3 (two columns)", i, got)
		}
		if row.Depth != bag.MustSP("3pt") {
			t.Errorf("row %d: depth %s, want the vertical spacing of 3pt", i, row.Depth)
		}
	}
}

// TestBorderModelSeparateDefault checks that a table without a declaration
// uses the separated model with the UA spacing of 2pt, as in CSS.
func TestBorderModelSeparateDefault(t *testing.T) {
	rows := tableRows(renderHTMLPages(t, borderModelCSS, borderModelHTML))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if got := spacingKerns(row); got != 3 {
			t.Errorf("row %d: got %d spacing kerns, want 3 (UA border-spacing)", i, got)
		}
		if row.Depth != bag.MustSP("2pt") {
			t.Errorf("row %d: depth %s, want the UA spacing of 2pt", i, row.Depth)
		}
	}
}

// TestBorderModelCollapse checks that border-collapse: collapse removes the
// spacing: the border between the two columns is drawn once, half by each
// cell.
func TestBorderModelCollapse(t *testing.T) {
	css := borderModelCSS + "\ntable.t { border-collapse: collapse }"
	rows := tableRows(renderHTMLPages(t, css, borderModelHTML))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if got := spacingKerns(row); got != 0 {
			t.Errorf("row %d: got %d spacing kerns, want none", i, got)
		}
		if row.Depth != 0 {
			t.Errorf("row %d: depth %s, want 0", i, row.Depth)
		}
	}
}

// TestBorderModelCollapseTableBorder checks that in the collapsing model the
// table's own border merges into the edge cells and is not drawn a second
// time by a wrapper around the table.
func TestBorderModelCollapseTableBorder(t *testing.T) {
	css := borderModelCSS + "\ntable.t { border: 4pt solid red; border-collapse: collapse }"
	pages := renderHTMLPages(t, css, borderModelHTML)
	rows := tableRows(pages)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// The first row's height includes the 4pt top border of the table
	// (merged into its cells) instead of the cells' own 1pt.
	if rows[0].Height <= rows[1].Height {
		t.Errorf("first row (%s) should be taller than the second (%s) by the merged table border", rows[0].Height, rows[1].Height)
	}
	// No wrapper: the border rule of HTMLBorder must not appear, the edge
	// cells draw the border.
	if hasHTMLBorderRule(pages) {
		t.Error("the table is wrapped in a border box although the collapsing model draws the border in the cells")
	}
	// The separated model keeps the wrapper.
	css = borderModelCSS + "\ntable.t { border: 4pt solid red }"
	if !hasHTMLBorderRule(renderHTMLPages(t, css, borderModelHTML)) {
		t.Error("separated model: the table border should be drawn by the wrapper")
	}
}

// hasHTMLBorderRule reports whether a rule drawn by HTMLBorder is on the pages.
func hasHTMLBorderRule(pages []*document.Page) bool {
	found := false
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.Rule:
				if origin, _ := v.GetAttribute("origin"); origin == "html border + clipping" {
					found = true
				}
			case *node.VList:
				walk(v.List)
			case *node.HList:
				walk(v.List)
			}
		}
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist.List)
			}
		}
	}
	return found
}
