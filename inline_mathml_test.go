package htmlbag

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// loadHTMLBagWithMath builds a CSSBuilder with the default included fonts
// plus a "math" font-family backed by Latin Modern Math. Tests requiring
// math typesetting use this helper; if the LMM font isn't on disk (typical
// fresh checkout / CI without the math testdata) the test is skipped.
func loadHTMLBagWithMath(t *testing.T) *CSSBuilder {
	t.Helper()
	mathPath := filepath.Join("..", "boxesandglue", "frontend", "math", "testdata", "latinmodern-math.otf")
	if _, err := os.Stat(mathPath); os.IsNotExist(err) {
		t.Skipf("math font not available at %s — see ../boxesandglue/frontend/math/testdata/README.md", mathPath)
	}
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	mathFam := fe.NewFontFamily("math")
	if err := mathFam.AddMember(&frontend.FontSource{Location: mathPath, Name: "Latin Modern Math"}, 400, frontend.FontStyleNormal); err != nil {
		t.Fatalf("AddMember math: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	return cb
}

// findMathHList walks a frontend.Text tree looking for the *node.HList the
// math engine produces (signature: positive Width, no "origin" attribute
// since we didn't tag one, and we know it sits at a direct Items position
// after the surrounding text).
func findMathHList(t *frontend.Text) *node.HList {
	if t == nil {
		return nil
	}
	for _, itm := range t.Items {
		switch v := itm.(type) {
		case *node.HList:
			return v
		case *frontend.Text:
			if got := findMathHList(v); got != nil {
				return got
			}
		}
	}
	return nil
}

// TestInlineMathMLRoundtrip — end-to-end smoke test: HTML with an inline
// <math> element gets parsed, the subtree serialised, handed to the mathml
// reader, and rendered by the math engine. We assert the HList lands in
// te.Items with a positive width.
func TestInlineMathMLRoundtrip(t *testing.T) {
	cb := loadHTMLBagWithMath(t)
	const html = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p>Let <math><msup><mi>x</mi><mn>2</mn></msup></math> be a square.</p></body>
	</html>`
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	hl := findMathHList(te)
	if hl == nil {
		t.Fatal("no math HList found in Text tree — selection or render path didn't fire")
	}
	if hl.Width <= 0 {
		t.Errorf("math HList width: got %d, want > 0", int64(hl.Width))
	}
}

// TestDisplayMathMLRoutesToDisplayStyle — <math display="block"> must route
// to math.DisplayMath, which produces a visibly taller HList than the same
// formula in inline mode (different Style → larger FractionShifts etc.).
func TestDisplayMathMLRoutesToDisplayStyle(t *testing.T) {
	cb := loadHTMLBagWithMath(t)
	const tmpl = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p><math%s><mfrac><mn>1</mn><mn>2</mn></mfrac></math></p></body>
	</html>`
	inline, err := cb.HTMLToText(htmlFill(tmpl, ""))
	if err != nil {
		t.Fatalf("inline HTMLToText: %v", err)
	}
	block, err := cb.HTMLToText(htmlFill(tmpl, ` display="block"`))
	if err != nil {
		t.Fatalf("block HTMLToText: %v", err)
	}
	hInline := findMathHList(inline)
	hBlock := findMathHList(block)
	if hInline == nil || hBlock == nil {
		t.Fatalf("missing math HLists: inline=%v block=%v", hInline, hBlock)
	}
	// Display fraction shifts num up and den down further than inline.
	// The full HList height (height + depth) is the cleanest aggregate
	// signal that distinguishes the two styles.
	inlineExt := int64(hInline.Height + hInline.Depth)
	blockExt := int64(hBlock.Height + hBlock.Depth)
	if blockExt <= inlineExt {
		t.Errorf("display HList not taller than inline: inlineExt=%d, blockExt=%d", inlineExt, blockExt)
	}
}

// htmlFill is a tiny printf-replacement for the two-call test pattern above.
// strings.Replace with one %s placeholder keeps the template readable.
func htmlFill(s, repl string) string {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '%' && s[i+1] == 's' {
			return s[:i] + repl + s[i+2:]
		}
	}
	return s
}
