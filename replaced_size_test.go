package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// The CSS width and height of a replaced element override its width and
// height attributes, which are presentational hints (issue #113). Before,
// an inline <svg> read only the attributes, and <img> only the CSS width.

// replacedSize returns the size of the first image or SVG box in te.
func replacedSize(te *frontend.Text) (wd, ht bag.ScaledPoint, ok bool) {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case *frontend.Text:
			if wd, ht, ok := replacedSize(v); ok {
				return wd, ht, true
			}
		case *node.Image:
			return v.Width, v.Height, true
		case *node.VList:
			if o, _ := v.Attributes["origin"].(string); o == "inline-svg" || o == "svg" {
				return v.Width, v.Height + v.Depth, true
			}
		}
	}
	return 0, 0, false
}

func TestReplacedElementCSSSize(t *testing.T) {
	const box = `viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`
	const wide = `viewBox="0 0 20 10"><rect width="20" height="10"/></svg>`
	tests := []struct {
		name   string
		html   string
		wd, ht string
	}{
		{"svg css width", `<svg style="width: 50pt" ` + box, "50pt", "50pt"},
		{"svg css height", `<svg style="height: 50pt" ` + box, "50pt", "50pt"},
		{"svg css height keeps the aspect ratio", `<svg style="height: 50pt" ` + wide, "100pt", "50pt"},
		{"svg css width beats the attribute", `<svg width="20pt" style="width: 50pt" ` + box, "50pt", "50pt"},
		{"svg css height beats the attribute", `<svg height="20pt" style="height: 50pt" ` + box, "50pt", "50pt"},
		{"svg css auto drops the attribute", `<svg width="20pt" height="20pt" style="height: auto" ` + box, "20pt", "20pt"},
		{"svg em is the font size of the svg", `<span style="font-size: 20pt"><svg style="width: 1.5em" ` + box + `</span>`, "30pt", "30pt"},
		{"svg em from a style sheet", `<svg class="icon" ` + box, "12pt", "12pt"},
		{"svg max-width caps", `<svg width="100pt" style="max-width: 40pt" ` + box, "40pt", "40pt"},
		{"svg attributes without css", `<svg width="20pt" ` + box, "20pt", "20pt"},
		{"img svg css height", `<img src="testdata/float.svg" style="height: 50pt">`, "75pt", "50pt"},
		{"img png css height", `<img src="testdata/float.png" style="height: 50pt">`, "183.33333pt", "50pt"},
		{"img css width auto keeps the natural size", `<img src="testdata/float.svg" style="width: auto">`, "45pt", "30pt"},
		{"img css width beats a percentage attribute", `<img src="testdata/float.png" width="50%" style="width: 110pt">`, "110pt", "30pt"},
		// A width or height attribute without a unit is in CSS pixels (#115).
		{"img width attribute in pixels", `<img src="testdata/float.png" width="200">`, "150pt", "40.90909pt"},
		{"img height attribute in pixels", `<img src="testdata/float.png" height="40">`, "110pt", "30pt"},
		{"img svg width attribute in pixels", `<img src="testdata/float.svg" width="200">`, "150pt", "100pt"},
		{"svg attributes in pixels", `<svg width="96" height="96" ` + box, "72pt", "72pt"},
		{"svg width attribute stays beside a css height", `<svg width="96" style="height: 50pt" ` + box, "72pt", "50pt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			te := htmlToText(t, newCSSBuilderForTest(t), `<html><head><style>.icon { width: 1.2em }</style></head><body><p style="font-size: 10pt">`+tc.html+`</p></body></html>`)
			wd, ht, ok := replacedSize(te)
			if !ok {
				t.Fatal("no image or svg box in the text")
			}
			wantWd, wantHt := bag.MustSP(tc.wd), bag.MustSP(tc.ht)
			if diff := wd - wantWd; diff < -2 || diff > 2 {
				t.Errorf("width = %s, want %s", wd, wantWd)
			}
			if diff := ht - wantHt; diff < -2 || diff > 2 {
				t.Errorf("height = %s, want %s", ht, wantHt)
			}
		})
	}
}

// A height alone gives the width through the aspect ratio, also on the
// deferred path that a percentage max-width takes.
func TestResolveWidthFromHeight(t *testing.T) {
	sp := bag.MustSP
	d := imageDims{ht: sp("50pt"), maxPct: 100}
	if got, want := d.resolveWidth(sp("400pt"), sp("200pt"), sp("100pt")), sp("100pt"); got != want {
		t.Errorf("resolveWidth = %s, want %s", got, want)
	}
	if got, want := d.resolveWidth(sp("60pt"), sp("200pt"), sp("100pt")), sp("60pt"); got != want {
		t.Errorf("capped resolveWidth = %s, want %s", got, want)
	}
}

func TestReplacedLength(t *testing.T) {
	fs, root := bag.MustSP("10pt"), bag.MustSP("12pt")
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"20pt", "20pt", true},
		{"2em", "20pt", true},
		{"2rem", "24pt", true},
		{"0", "0pt", true},
		{"auto", "0pt", false},
		{"50%", "0pt", false},
		{"2", "0pt", false},
		{"max-content", "0pt", false},
		{"xem", "0pt", false},
	} {
		got, ok := replacedLength(tc.in, fs, root)
		if ok != tc.ok || got != bag.MustSP(tc.want) {
			t.Errorf("replacedLength(%q) = %s, %v, want %s, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAttrLength(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"96", "72pt", true},
		{" 200 ", "150pt", true},
		{"1.5", "1.125pt", true},
		{"20pt", "20pt", true},
		{"2cm", "2cm", true},
		{"auto", "0pt", false},
		{"12em", "0pt", false},
	} {
		got, err := attrLength(tc.in)
		if (err == nil) != tc.ok || (tc.ok && got != bag.MustSP(tc.want)) {
			t.Errorf("attrLength(%q) = %s, %v, want %s", tc.in, got, err, tc.want)
		}
	}
}
