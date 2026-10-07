package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// trimFragments renders body on trimCSS pages, or with FlowText in regions
// of the same 160pt by 158pt, and returns the lines of each page or region,
// top and bottom measured down from its top edge.
func trimFragments(t *testing.T, css, body string, regions bool) [][]placedLine {
	t.Helper()
	return trimFragmentsIn(t, css, body, regions, "158pt")
}

// trimFragmentsIn is trimFragments for a content area h high, the page's
// margins being 20pt.
func trimFragmentsIn(t *testing.T, css, body string, regions bool, h string) [][]placedLine {
	t.Helper()
	var frags [][]placedLine
	if regions {
		cb, _ := newFlowBuilder(t, css)
		tr := flow(t, cb, body, wide(h))
		for _, f := range tr.filled {
			lines := textLines([]*document.Page{{Objects: []document.Object{{Vlist: f.Box}}}})
			for i := range lines {
				lines[i].top, lines[i].bottom = -lines[i].top, -lines[i].bottom
			}
			frags = append(frags, lines)
		}
		return frags
	}
	top := sp(h) + sp("20pt")
	for _, l := range textLines(renderHTMLPages(t, css, body)) {
		for len(frags) < l.page {
			frags = append(frags, nil)
		}
		l.top, l.bottom = top-l.top, top-l.bottom
		frags[l.page-1] = append(frags[l.page-1], l)
	}
	return frags
}

// downSize is lineSize for a line measured downwards.
func downSize(l placedLine) bag.ScaledPoint { return l.bottom - l.top }

// eachTarget runs f on pages and in FlowText regions.
func eachTarget(t *testing.T, f func(t *testing.T, regions bool)) {
	for _, regions := range []bool{false, true} {
		name := "pages"
		if regions {
			name = "FlowText"
		}
		t.Run(name, func(t *testing.T) { f(t, regions) })
	}
}

// Under box-decoration-break: clone every fragment is trimmed: with
// trim-end, the tenth 16pt line (160pt) fits the 158pt page by its text,
// so the page holds ten lines, the last one trimmed and inside the page.
// Under slice only the block's last line is trimmed, and nine lines fit.
func TestTextBoxTrimCloneEndAtBreak(t *testing.T) {
	full, room := sp("16pt"), sp("158pt")
	for _, c := range []struct {
		style   string
		lines   int
		trimmed bool
	}{
		{"text-box-trim: trim-end; box-decoration-break: clone", 10, true},
		{"text-box-trim: trim-both; box-decoration-break: clone", 10, true},
		{"text-box-trim: trim-end", 9, false},
		{"text-box-trim: trim-end; box-decoration-break: slice", 9, false},
		{"box-decoration-break: clone", 9, false},
	} {
		t.Run(c.style, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				frags := trimFragments(t, trimCSS, `<p style="`+c.style+`">`+charLines("A", 14)+`</p>`, regions)
				if len(frags) != 2 || len(frags[0]) != c.lines {
					t.Fatalf("%d fragments, %d lines on the first, want 2 and %d", len(frags), len(frags[0]), c.lines)
				}
				last := frags[0][c.lines-1]
				if got := downSize(last); c.trimmed == (got == full) {
					t.Errorf("the last line before the break is %s, trimmed %t", got, c.trimmed)
				}
				if last.bottom > room {
					t.Errorf("the last line before the break ends %s down, past the %s room", last.bottom, room)
				}
			})
		})
	}
}

// With trim-start under clone, the first line of each continued fragment is
// trimmed at its start too; under slice, and without text-box-trim, it is
// not.
func TestTextBoxTrimCloneStartAtBreak(t *testing.T) {
	full := sp("16pt")
	for _, c := range []struct {
		style   string
		trimmed bool
	}{
		{"text-box-trim: trim-start; box-decoration-break: clone", true},
		{"text-box-trim: trim-both; box-decoration-break: clone", true},
		{"text-box-trim: trim-end; box-decoration-break: clone", false},
		{"text-box-trim: trim-start", false},
		{"box-decoration-break: clone", false},
	} {
		t.Run(c.style, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				frags := trimFragments(t, trimCSS, `<p style="`+c.style+`">`+charLines("A", 24)+`</p>`, regions)
				if len(frags) != 3 {
					t.Fatalf("%d fragments, want 3", len(frags))
				}
				for i, f := range frags[1:] {
					first := f[0]
					if got := downSize(first); c.trimmed == (got == full) {
						t.Errorf("fragment %d: the first line is %s, trimmed %t", i+2, got, c.trimmed)
					}
					if first.top != 0 {
						t.Errorf("fragment %d: the first line starts %s down, want 0", i+2, first.top)
					}
				}
			})
		})
	}
}

// Under slice, as #80 has it, only the first fragment's start and the last
// fragment's end are trimmed, in FlowText regions as on pages.
func TestTextBoxTrimSliceAtBreak(t *testing.T) {
	full := sp("16pt")
	eachTarget(t, func(t *testing.T, regions bool) {
		frags := trimFragments(t, trimCSS, `<p style="text-box-trim: trim-both">`+charLines("A", 14)+`</p>`, regions)
		if len(frags) != 2 {
			t.Fatalf("%d fragments, want 2", len(frags))
		}
		one, two := frags[0], frags[1]
		if got := downSize(one[0]); got >= full {
			t.Errorf("the block's first line is %s, want it trimmed", got)
		}
		for _, l := range []placedLine{one[len(one)-1], two[0]} {
			if got := downSize(l); got != full {
				t.Errorf("a line at the break is %s, want %s", got, full)
			}
		}
		if got := downSize(two[len(two)-1]); got >= full {
			t.Errorf("the block's last line is %s, want it trimmed", got)
		}
	})
}

// Lines set tighter than the font's content area have negative trims: at a
// break under clone the last line grows to its text. Fifteen 10pt lines
// fit the 150.1pt page untrimmed, but not with the last one grown, so the
// break comes a line earlier and the grown line stays inside the page.
func TestTextBoxTrimCloneNegativeAtBreak(t *testing.T) {
	const css = `@page { size: 200pt 190.1pt; margin: 20pt; }
body { margin: 0 } p { margin: 0; font-family: serif; font-size: 10pt; line-height: 6pt }`
	room := sp("150.1pt")
	for _, c := range []struct {
		style string
		lines int
		grown bool
	}{
		{"text-box-trim: trim-end; box-decoration-break: clone", 14, true},
		{"text-box-trim: trim-end", 15, false},
	} {
		t.Run(c.style, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				frags := trimFragmentsIn(t, css, `<p style="`+c.style+`">`+charLines("A", 30)+`</p>`, regions, "150.1pt")
				if len(frags) < 2 || len(frags[0]) != c.lines {
					t.Fatalf("%d lines on the first of %d fragments, want %d", len(frags[0]), len(frags), c.lines)
				}
				one := frags[0]
				last, before := downSize(one[len(one)-1]), downSize(one[len(one)-2])
				if c.grown != (last > before) {
					t.Errorf("the last line before the break is %s, the one before it %s, grown %t", last, before, c.grown)
				}
				if b := one[len(one)-1].bottom; b > room {
					t.Errorf("the last line before the break ends %s down, past the %s room", b, room)
				}
			})
		})
	}
}

// A clone paragraph cut through inside a container that splits (planSplit
// and cutBlock) is trimmed at the break as well: after a 16pt line, nine
// lines fit the 142pt left only with the last one trimmed, and with
// trim-start the rest's first line is trimmed.
func TestTextBoxTrimCloneNestedAtBreak(t *testing.T) {
	for _, c := range []struct {
		style string
		lines int
	}{
		{"text-box-trim: trim-end; box-decoration-break: clone", 10},
		{"text-box-trim: trim-both; box-decoration-break: clone", 10},
		{"text-box-trim: trim-end", 9},
	} {
		t.Run(c.style, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				body := `<p>Intro</p><div><p style="` + c.style + `">` + charLines("A", 20) + `</p></div>`
				frags := trimFragments(t, trimCSS, body, regions)
				if len(frags) < 2 || len(frags[0]) != c.lines {
					t.Fatalf("%d lines on the first of %d fragments, want %d", len(frags[0]), len(frags), c.lines)
				}
				if c.style == "text-box-trim: trim-both; box-decoration-break: clone" {
					if got := downSize(frags[1][0]); got >= sp("16pt") {
						t.Errorf("the rest's first line is %s, want it trimmed", got)
					}
				}
				if last := frags[0][len(frags[0])-1]; last.bottom > sp("158pt") {
					t.Errorf("the last line before the break ends %s down, past the room", last.bottom)
				}
			})
		})
	}
}

// The block's own last line, trimmed when the block was built, is not
// trimmed a second time to fit at a break: ten lines with 3pt of padding
// below need more than the 158pt page even with the last line trimmed once.
func TestTextBoxTrimCloneNoDoubleTrim(t *testing.T) {
	eachTarget(t, func(t *testing.T, regions bool) {
		frags := trimFragments(t, trimCSS, `<p style="text-box-trim: trim-end; box-decoration-break: clone; padding-bottom: 3pt">`+charLines("A", 10)+`</p>`, regions)
		if len(frags) != 2 {
			t.Fatalf("%d fragments, want 2", len(frags))
		}
		if last := frags[0][len(frags[0])-1]; last.bottom > sp("155pt") {
			t.Errorf("the last line on the first fragment ends %s down, past the 155pt above the padding", last.bottom)
		}
	})
}

// A container's text-box-trim reaches only its own first and last line
// (#82): a clone paragraph in it, without a trim of its own, is not trimmed
// at its breaks.
func TestTextBoxTrimCloneContainerTrim(t *testing.T) {
	full := sp("16pt")
	eachTarget(t, func(t *testing.T, regions bool) {
		body := `<div style="text-box-trim: trim-both"><p style="box-decoration-break: clone">` + charLines("A", 14) + `</p></div>`
		frags := trimFragments(t, trimCSS, body, regions)
		if len(frags) != 2 {
			t.Fatalf("%d fragments, want 2", len(frags))
		}
		one, two := frags[0], frags[1]
		if got := downSize(one[0]); got >= full {
			t.Errorf("the container's first line is %s, want it trimmed", got)
		}
		for _, l := range []placedLine{one[len(one)-1], two[0]} {
			if got := downSize(l); got != full {
				t.Errorf("a line at the break is %s, want %s", got, full)
			}
		}
		if got := downSize(two[len(two)-1]); got >= full {
			t.Errorf("the container's last line is %s, want it trimmed", got)
		}
	})
}
