package htmlbag

import (
	"strings"
	"testing"
)

const stringsCSS = `@page { size: a6; margin: 15mm }
body { counter-reset: ch }
h1 { counter-increment: ch; string-set: chapter "Ch. " counter(ch) ": " content(), tag attr(data-tag); margin-top: 0 }
.b { page-break-before: always }
`

// The assignments on a page and the value it carries in decide what string()
// shows at each position.
func TestNamedStringPositions(t *testing.T) {
	const html = `<h1 data-tag="T1">One</h1><p>Text</p>
<p class="b">Plain page</p>
<p class="b">Above</p><h1 data-tag="T2">Two <em>nested</em></h1><h1 data-tag="T3">Three</h1>
<h1 class="b" data-tag="T4">Four</h1>`
	pages, cb := renderHTMLPagesCB(t, stringsCSS, html)
	if len(pages) != 4 {
		t.Fatalf("got %d pages, want 4", len(pages))
	}
	for _, tc := range []struct {
		name, position string
		page           int
		want           string
	}{
		{"chapter", "first", 1, "Ch. 1: One"},
		{"chapter", "start", 1, "Ch. 1: One"},
		{"chapter", "first-except", 1, ""},
		{"chapter", "first", 2, "Ch. 1: One"},
		{"chapter", "first-except", 2, "Ch. 1: One"},
		{"chapter", "last", 2, "Ch. 1: One"},
		{"chapter", "first", 3, "Ch. 2: Two nested"},
		{"chapter", "last", 3, "Ch. 3: Three"},
		// The heading is not the first thing on the page.
		{"chapter", "start", 3, "Ch. 1: One"},
		{"chapter", "first-except", 3, ""},
		{"chapter", "start", 4, "Ch. 4: Four"},
		{"tag", "first", 3, "T2"},
		{"unknown", "first", 1, ""},
	} {
		if got := cb.namedString(tc.name, tc.position, tc.page); got != tc.want {
			t.Errorf("string(%s, %s) on page %d = %q, want %q", tc.name, tc.position, tc.page, got, tc.want)
		}
	}
}

// A value can come from any element that ends up on a page: an inline one, a
// block of blocks or a table. A hidden element assigns nothing.
func TestNamedStringCarriers(t *testing.T) {
	for _, tc := range []struct{ name, html, want string }{
		{"inline", `<p>A <span style="string-set: x content()">word</span></p>`, "word"},
		{"block of blocks", `<div style="string-set: x 'v'"><p>A</p><p>B</p></div>`, "v"},
		{"table", `<table style="string-set: x 'v'"><tr><td>A</td></tr></table>`, "v"},
		{"row", `<table><tr style="string-set: x 'v'"><td></td><td>A</td></tr></table>`, "v"},
		{"list", `<ul><li style="string-set: x 'v'">A</li></ul>`, "v"},
		{"attribute", `<p title="t" style="string-set: x attr(title)">A</p>`, "t"},
		{"mixed", `<p style="string-set: x 'a' content() 'b'">T</p>`, "aTb"},
		// An empty block makes no box, so nothing of it reaches a page.
		{"empty element", `<p>A</p><div style="string-set: x 'v'"></div><p>B</p>`, ""},
		{"hidden element", `<p style="display: none; string-set: x 'v'">A</p>`, ""},
		{"none", `<p style="string-set: none">A</p>`, ""},
	} {
		_, cb := renderHTMLPagesCB(t, `@page { size: a6 }`, tc.html)
		if got := cb.namedString("x", "first", 1); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A paragraph across a page break assigns once, on the page it starts on.
func TestNamedStringSplitElement(t *testing.T) {
	long := strings.Repeat("word word word word word word word ", 120)
	pages, cb := renderHTMLPagesCB(t, `@page { size: a6 }`, `<p style="string-set: x 'v'">`+long+`</p>`)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want more", len(pages))
	}
	if got := len(cb.pageStrings[1]) + len(cb.pageStrings[2]); got != 1 {
		t.Errorf("%d assignments, want 1", got)
	}
	if got := cb.namedString("x", "first-except", 2); got != "v" {
		t.Errorf("page 2: %q, want the value carried in", got)
	}
}

// A rebuild for a page of another width must not lose or repeat the assignment.
func TestNamedStringPageWidthChange(t *testing.T) {
	long := strings.Repeat("word word word word word word word ", 120)
	css := `@page { size: a6; margin: 10mm } @page :first { margin-left: 30mm }`
	_, cb := renderHTMLPagesCB(t, css, `<p>`+long+`</p><p style="string-set: x 'v'">After</p>`)
	got := 0
	for _, strs := range cb.pageStrings {
		got += len(strs)
	}
	if got != 1 {
		t.Errorf("%d assignments, want 1", got)
	}
}

func TestStringSetWarns(t *testing.T) {
	const doc = `<html><body><p class="x">Text</p></body></html>`
	for _, tc := range []struct{ name, css, want string }{
		{"no value", `.x { string-set: alone }`, "needs a name and a value"},
		{"before", `.x { string-set: a content(before) }`, "only content()"},
	} {
		got := strings.Join(warningsOf(t, tc.css, doc), "\n")
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: got warnings %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Only a break to the side of the page that comes next leaves no page empty,
// and the empty page gets the :blank rule on top of its side's rule.
func TestBlankPageRule(t *testing.T) {
	const css = `@page { size: a6; margin: 10mm }
@page :blank { margin-top: 44mm; background-color: #ffe0e0 }`
	pages := renderHTMLPages(t, css, `<p>One</p><p style="break-before: right">Two</p>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	for i, want := range []bool{false, true, false} {
		if got := pageBackgroundColor(pages[i]) != ""; got != want {
			t.Errorf("page %d: background %t, want %t", i+1, got, want)
		}
	}
	for _, l := range placedLines(pages) {
		if l.page == 2 {
			t.Errorf("text on the blank page: %q", l.text)
		}
	}
}

func TestPagesSkipped(t *testing.T) {
	for _, tc := range []struct {
		brk  string
		next int
		want bool
	}{
		{"", 2, false},
		{"page", 2, false},
		{"always", 3, false},
		{"right", 2, true},
		{"recto", 2, true},
		{"right", 3, false},
		{"left", 3, true},
		{"verso", 3, true},
		{"left", 2, false},
	} {
		if got := pagesSkipped(tc.brk, tc.next); got != tc.want {
			t.Errorf("pagesSkipped(%q, %d) = %t, want %t", tc.brk, tc.next, got, tc.want)
		}
	}
}
