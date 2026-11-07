package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// The block pass in Output() drew only img, so svg, math and barcode with
// display: block were dropped without a warning (issue #111). These tests
// check that each one reaches the Text tree as a box of its own.

func newCSSBuilderForTest(t *testing.T) *CSSBuilder {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	return cb
}

func htmlToText(t *testing.T, cb *CSSBuilder, html string) *frontend.Text {
	t.Helper()
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	return te
}

// findNodeOfType returns the first item in a Text tree that is a node of
// the given type.
func findNodeOfType(t *frontend.Text, typ node.Type) node.Node {
	for _, itm := range t.Items {
		switch v := itm.(type) {
		case *frontend.Text:
			if got := findNodeOfType(v, typ); got != nil {
				return got
			}
		case node.Node:
			if v.Type() == typ {
				return v
			}
		}
	}
	return nil
}

func TestSVGDisplayBlock(t *testing.T) {
	te := htmlToText(t, newCSSBuilderForTest(t), `<html><head><style>.blk svg { display: block }</style></head><body>
<p>before</p>
<div class="blk"><svg width="20pt" height="20pt" viewBox="0 0 10 10"><rect width="10" height="10" fill="blue"/></svg></div>
<p>after</p></body></html>`)
	vl := findInlineSVGVList(te)
	if vl == nil {
		t.Fatal("svg with display: block: no inline-svg VList in the Text tree")
	}
	if vl.Width != bag.MustSP("20pt") {
		t.Errorf("vl.Width = %s, want 20pt", vl.Width)
	}
}

func TestMathDisplayBlock(t *testing.T) {
	cb := loadHTMLBagWithMath(t)
	te := htmlToText(t, cb, `<html><head><style>math { font-family: math; display: block }</style></head>
<body><div><math><mi>x</mi><mo>=</mo><mn>1</mn></math></div></body></html>`)
	hl := findMathHList(te)
	if hl == nil {
		t.Fatal("math with display: block: no HList in the Text tree")
	}
	if hl.Width <= 0 {
		t.Errorf("math HList width = %s, want > 0", hl.Width)
	}
}

func TestBarcodeDisplayBlock(t *testing.T) {
	inline := findNodeOfType(htmlToText(t, newCSSBuilderForTest(t), `<p><barcode value="hello" type="qrcode" width="2cm"></barcode></p>`), node.TypeVList)
	if inline == nil {
		t.Fatal("inline barcode: no VList in the Text tree")
	}
	block := findNodeOfType(htmlToText(t, newCSSBuilderForTest(t), `<div><barcode value="hello" type="qrcode" width="2cm" style="display: block"></barcode></div>`), node.TypeVList)
	if block == nil {
		t.Fatal("barcode with display: block: no VList in the Text tree")
	}
	if bw, iw := block.(*node.VList).Width, inline.(*node.VList).Width; bw != iw {
		t.Errorf("barcode with display: block is %s wide, inline %s", bw, iw)
	}
}
