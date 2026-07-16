package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// lineTopY returns the top edge (PDF user space, y grows upward) of the
// first line (HList) whose glyph components contain needle, or -1. Unlike
// paragraphY (which returns the enclosing object's top), this walks into
// the vertical lists, so margin/padding/height kerns inside a box are
// accounted for.
func lineTopY(pg *document.Page, needle string) bag.ScaledPoint {
	var walk func(n node.Node, y bag.ScaledPoint) (bag.ScaledPoint, bool)
	walk = func(n node.Node, y bag.ScaledPoint) (bag.ScaledPoint, bool) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				var sb strings.Builder
				collectComponents(v.List, &sb)
				if strings.Contains(sb.String(), needle) {
					return y, true
				}
				y -= v.Height + v.Depth
			case *node.VList:
				if found, ok := walk(v.List, y); ok {
					return found, true
				}
				y -= v.Height + v.Depth
			case *node.Kern:
				y -= v.Kern
			case *node.Glue:
				y -= v.Width
			}
		}
		return 0, false
	}
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if y, ok := walk(obj.Vlist.List, obj.Y); ok {
			return y
		}
	}
	return -1
}

const flowReservationCSS = `@page { size: a4; margin: 20mm; }
p { margin: 0; }`

// TestHeightSpacerMatchesMarginTopWorkaround: the §5 acceptance pair. A
// height:85mm spacer must push the following flow text to exactly the
// position the margin-top:85mm workaround produces.
func TestHeightSpacerMatchesMarginTopWorkaround(t *testing.T) {
	body := `<p>FLIESSTEXT Zeile eins nach dem Freiraum.</p><p>Zweite Zeile.</p>`
	withMargin := `<html><body><div style="margin-top: 85mm">` + body + `</div></body></html>`
	withSpacer := `<html><body><div style="height: 85mm"></div><div>` + body + `</div></body></html>`
	control := `<html><body><div>` + body + `</div></body></html>`

	yMargin := lineTopY(renderHTMLPages(t, flowReservationCSS, withMargin)[0], "FLIESSTEXT")
	ySpacer := lineTopY(renderHTMLPages(t, flowReservationCSS, withSpacer)[0], "FLIESSTEXT")
	yControl := lineTopY(renderHTMLPages(t, flowReservationCSS, control)[0], "FLIESSTEXT")
	if yMargin == -1 || ySpacer == -1 || yControl == -1 {
		t.Fatal("marker line not found")
	}
	if ySpacer != yMargin {
		t.Errorf("height spacer places text at y=%s, margin-top workaround at y=%s, want identical", ySpacer, yMargin)
	}
	if diff := yControl - ySpacer; diff != bag.MustSP("85mm") {
		t.Errorf("spacer shifted the text by %s, want exactly 85mm", diff)
	}
}

// TestHeightOnBlockWithContentReservesFlowSpace: a block whose content is
// shorter than its declared CSS height must occupy the full height, so the
// following flow starts exactly height below the block's top.
func TestHeightOnBlockWithContentReservesFlowSpace(t *testing.T) {
	html := `<html><body>
<div style="height: 85mm"><p>KOPFZEILE im hohen Block</p></div>
<p>FLIESSTEXT nach dem Block</p>
</body></html>`
	pages := renderHTMLPages(t, flowReservationCSS, html)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	yHead := lineTopY(pages[0], "KOPFZEILE")
	yAfter := lineTopY(pages[0], "FLIESSTEXT")
	if yHead == -1 || yAfter == -1 {
		t.Fatal("marker line not found")
	}
	if diff := yHead - yAfter; diff != bag.MustSP("85mm") {
		t.Errorf("text after the block starts %s below the block content, want exactly 85mm", diff)
	}
}

// TestHeightSmallerThanContentDoesNotClip: CSS height smaller than the
// natural content acts as min-height. Nothing is clipped and the following
// flow starts right below the natural content, identical to a block
// without any height.
func TestHeightSmallerThanContentDoesNotClip(t *testing.T) {
	long := `<p>LANGTEXT eins</p><p>LANGTEXT zwei</p><p>LANGTEXT drei</p><p>LANGTEXT vier</p>`
	withHeight := `<html><body><div style="height: 5mm">` + long + `</div><p>DANACH kommt dieser Text</p></body></html>`
	control := `<html><body><div>` + long + `</div><p>DANACH kommt dieser Text</p></body></html>`

	pagesA := renderHTMLPages(t, flowReservationCSS, withHeight)
	pagesB := renderHTMLPages(t, flowReservationCSS, control)
	txt := pageText(pagesA[0])
	for _, marker := range []string{"eins", "zwei", "drei", "vier", "DANACH"} {
		if !strings.Contains(txt, marker) {
			t.Errorf("marker %q missing: content was clipped", marker)
		}
	}
	yA := lineTopY(pagesA[0], "DANACH")
	yB := lineTopY(pagesB[0], "DANACH")
	if yA == -1 || yB == -1 {
		t.Fatal("marker line not found")
	}
	if yA != yB {
		t.Errorf("following text at y=%s with small height, y=%s without, want identical (min-height semantics)", yA, yB)
	}
}

// TestPaddingTopReservesFlowSpace: padding-top on a plain flow block (no
// border/background, where HTMLBorder does not run) must reserve vertical
// space, identical to the margin-top equivalent for a first element. Both
// the leaf branch (<p>) and the box branch (<div> container) are covered.
func TestPaddingTopReservesFlowSpace(t *testing.T) {
	cases := []struct {
		name            string
		padding, margin string
	}{
		{"leaf paragraph",
			`<p style="padding-top: 20mm">ABSATZTEXT mit Abstand oben</p>`,
			`<p style="margin-top: 20mm">ABSATZTEXT mit Abstand oben</p>`},
		{"box container",
			`<div style="padding-top: 20mm"><p>ABSATZTEXT mit Abstand oben</p></div>`,
			`<div style="margin-top: 20mm"><p>ABSATZTEXT mit Abstand oben</p></div>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yPad := lineTopY(renderHTMLPages(t, flowReservationCSS, `<html><body>`+tc.padding+`</body></html>`)[0], "ABSATZTEXT")
			yMar := lineTopY(renderHTMLPages(t, flowReservationCSS, `<html><body>`+tc.margin+`</body></html>`)[0], "ABSATZTEXT")
			if yPad == -1 || yMar == -1 {
				t.Fatal("marker line not found")
			}
			if yPad != yMar {
				t.Errorf("padding-top places text at y=%s, margin-top at y=%s, want identical", yPad, yMar)
			}
		})
	}
}
