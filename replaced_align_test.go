package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// vertical-align on an image or inline svg moves its box in the line
// (issue #114). The box stands on the baseline; the alignment splits its
// height into a part above and a part below the baseline.

func TestReplacedAlignApply(t *testing.T) {
	sp := bag.MustSP
	metrics := replacedAlign{ascent: sp("8pt"), descent: sp("2pt"), xHeight: sp("5pt")}
	tests := []struct {
		name       string
		keyword    string
		shift      string
		wantHeight string
	}{
		{"baseline", "", "0pt", "20pt"},
		{"length lowers", "", "-3pt", "17pt"},
		{"length raises", "", "4pt", "24pt"},
		{"text-top", "text-top", "0pt", "8pt"},
		{"top", "top", "0pt", "8pt"},
		{"text-bottom", "text-bottom", "0pt", "18pt"},
		{"middle", "middle", "0pt", "12.5pt"},
		{"middle in a raised span", "middle", "3pt", "15.5pt"},
	}
	for _, tc := range tests {
		a := metrics
		a.keyword, a.shift = tc.keyword, sp(tc.shift)
		ht, dp := sp("20pt"), bag.ScaledPoint(0)
		a.apply(&ht, &dp)
		if ht != sp(tc.wantHeight) || ht+dp != sp("20pt") {
			t.Errorf("%s: height %s depth %s, want height %s and the total 20pt", tc.name, ht, dp, tc.wantHeight)
		}
	}
}

// replacedBox returns the height and depth of the first image or svg box
// in te, resolving a deferred box at a width of 400pt.
func replacedBox(t *testing.T, te *frontend.Text) (ht, dp bag.ScaledPoint) {
	t.Helper()
	var find func(*frontend.Text) (bag.ScaledPoint, bag.ScaledPoint, bool)
	find = func(te *frontend.Text) (bag.ScaledPoint, bag.ScaledPoint, bool) {
		for _, itm := range te.Items {
			switch v := itm.(type) {
			case *frontend.Text:
				if ht, dp, ok := find(v); ok {
					return ht, dp, true
				}
			case *node.Image:
				return v.Height, v.Depth, true
			case *node.VList:
				if ftv := getDeferredFormatter(v); ftv != nil {
					resolveDeferredSizing([]any{v}, bag.MustSP("400pt"))
				}
				return v.Height, v.Depth, true
			}
		}
		return 0, 0, false
	}
	ht, dp, ok := find(te)
	if !ok {
		t.Fatal("no image or svg box in the text")
	}
	return ht, dp
}

func TestReplacedElementVerticalAlign(t *testing.T) {
	const icon = `viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`
	tests := []struct {
		name  string
		html  string
		depth string
	}{
		{"svg on the baseline", `<svg style="height: 20pt" ` + icon, "0pt"},
		{"svg lowered by a length", `<svg style="height: 20pt; vertical-align: -5pt" ` + icon, "5pt"},
		{"svg lowered by em", `<svg style="height: 20pt; vertical-align: -0.2em" ` + icon, "2pt"},
		{"svg with a percentage width", `<svg width="5%" style="vertical-align: -5pt" ` + icon, "5pt"},
		{"svg in a subscript", `<sub><svg style="height: 20pt" ` + icon + `</sub>`, "2pt"},
		{"svg as a float", `<svg style="height: 20pt; float: left; vertical-align: -5pt" ` + icon, "0pt"},
		{"img svg lowered", `<img src="testdata/float.svg" style="height: 20pt; vertical-align: -5pt">`, "5pt"},
		{"img svg with a percentage width", `<img src="testdata/float.svg" style="width: 5%; vertical-align: -5pt">`, "5pt"},
		{"img png lowered", `<img src="testdata/float.png" style="height: 20pt; vertical-align: -5pt">`, "5pt"},
		{"img png raised", `<img src="testdata/float.png" style="height: 20pt; vertical-align: 5pt">`, "-5pt"},
		{"img png with a percentage max-width", `<img src="testdata/float.png" style="max-width: 50%; vertical-align: -5pt">`, "5pt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			te := htmlToText(t, newCSSBuilderForTest(t), `<html><body><p style="font-size: 10pt">x `+tc.html+` x</p></body></html>`)
			_, dp := replacedBox(t, te)
			if want := bag.MustSP(tc.depth); dp < want-2 || dp > want+2 {
				t.Errorf("depth = %s, want %s", dp, want)
			}
		})
	}
}

// The vertical-align of a table cell places the cell's content, it does not
// move an image in the cell's lines.
func TestReplacedElementCellVerticalAlign(t *testing.T) {
	te := htmlToText(t, newCSSBuilderForTest(t), `<html><body><table><tr><td style="vertical-align: middle">x <svg style="height: 20pt" viewBox="0 0 10 10"><rect width="10" height="10"/></svg></td></tr></table></body></html>`)
	if _, dp := replacedBox(t, te); dp != 0 {
		t.Errorf("depth = %s, want 0: the image took the alignment of the cell", dp)
	}
}

// A keyword places the box against the font of the line: text-top puts its
// top at the ascent, text-bottom its bottom at the descent.
func TestReplacedElementVerticalAlignKeywords(t *testing.T) {
	const icon = `viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`
	cb := newCSSBuilderForTest(t)
	te := htmlToText(t, cb, `<html><body><p style="font-size: 10pt">x <svg style="height: 4pt; vertical-align: text-top" `+icon+`</p></body></html>`)
	ht, _ := replacedBox(t, te)
	if ht < bag.MustSP("6pt") || ht > bag.MustSP("11pt") {
		t.Errorf("text-top: height above the baseline %s, want about the ascent of a 10pt font", ht)
	}
	te = htmlToText(t, cb, `<html><body><p style="font-size: 10pt">x <svg style="height: 4pt; vertical-align: text-bottom" `+icon+`</p></body></html>`)
	if _, dp := replacedBox(t, te); dp < bag.MustSP("1pt") || dp > bag.MustSP("4pt") {
		t.Errorf("text-bottom: depth %s, want about the descent of a 10pt font", dp)
	}
	te = htmlToText(t, cb, `<html><body><p style="font-size: 10pt">x <svg style="height: 4pt; vertical-align: middle" `+icon+`</p></body></html>`)
	if ht, dp := replacedBox(t, te); ht-dp < bag.MustSP("3pt") || ht-dp > bag.MustSP("6pt") {
		t.Errorf("middle: height %s depth %s, want the middle at about half the x-height", ht, dp)
	}
}
