package htmlbag

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// renderMathForStructTree runs the full htmlbag pipeline on html with PDF/UA
// tagging enabled and a "math" font-family backed by Latin Modern Math,
// returning the populated structure root plus the raw PDF content bytes. The
// test is skipped when the LMM font is absent (fresh checkout / CI without
// the math testdata).
func renderMathForStructTree(t *testing.T, html string, format document.Format) (*document.StructureElement, string) {
	t.Helper()
	mathPath := filepath.Join("..", "boxesandglue", "frontend", "math", "testdata", "latinmodern-math.otf")
	if _, err := os.Stat(mathPath); os.IsNotExist(err) {
		t.Skipf("math font not available at %s", mathPath)
	}
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	fe.Doc.Format = format
	fe.Doc.CompressLevel = 0 // keep page content streams readable for assertions
	fe.Doc.DefaultLanguageTag = "en"
	if l, err := frontend.GetLanguage("en"); err == nil {
		fe.Doc.DefaultLanguage = l
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
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	if err := fe.Doc.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := fe.Doc.PDFWriter.FinishAndClose(); err != nil {
		t.Fatalf("FinishAndClose: %v", err)
	}
	return cb.structureRoot, buf.String()
}

// findSE returns the first structure element with the given role, pre-order.
func findSE(se *document.StructureElement, role string) *document.StructureElement {
	if se == nil {
		return nil
	}
	if se.Role == role {
		return se
	}
	for _, c := range se.Children() {
		if got := findSE(c, role); got != nil {
			return got
		}
	}
	return nil
}

// TestInlineFormulaStructure verifies that an inline <math> mid-paragraph
// produces a Formula structure element that is a child of the containing P,
// carries /Alt fallback text, and that the P drops its ActualText (which
// would otherwise mask the formula for assistive technology).
func TestInlineFormulaStructure(t *testing.T) {
	const html = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p>The relation <math><msup><mi>a</mi><mn>2</mn></msup></math> matters.</p></body>
	</html>`
	root, _ := renderMathForStructTree(t, html, document.FormatPDFUA)

	p := findSE(root, "P")
	if p == nil {
		t.Fatal("no P structure element found")
	}
	if p.ActualText != "" {
		t.Errorf("P carries ActualText %q — masks the Formula child for AT; want empty", p.ActualText)
	}
	var formula *document.StructureElement
	for _, c := range p.Children() {
		if c.Role == "Formula" {
			formula = c
		}
	}
	if formula == nil {
		t.Fatal("P has no Formula child — inline <math> was not tagged")
	}
	if formula.Alt == "" {
		t.Error("Formula /Alt is empty; expected a text fallback derived from the MathML tokens")
	}
	if !strings.Contains(formula.Alt, "a") || !strings.Contains(formula.Alt, "2") {
		t.Errorf("Formula /Alt %q does not contain the token text a, 2", formula.Alt)
	}
}

// TestFormulaMathMLAssociatedFile verifies that under PDF/UA-2 a Formula gets
// the MathML source embedded as an associated file: an EmbeddedFile stream
// with the MathML mime type, a Filespec with /AFRelationship /Supplement, and
// an /AF reference on the Formula structure element.
func TestFormulaMathMLAssociatedFile(t *testing.T) {
	const html = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p>See <math><msup><mi>e</mi><mi>x</mi></msup></math> here.</p></body>
	</html>`
	_, content := renderMathForStructTree(t, html, document.FormatPDFUA2)

	for _, want := range []string{
		"/EmbeddedFile",
		// The mime type is written as a PDF name, so "/" is escaped to #2f:
		// /application#2fmathml+xml.
		"mathml+xml",
		"/AFRelationship /Supplement",
		"/AF [",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("PDF output missing %q (MathML associated file not wired up)", want)
		}
	}
}

// TestFormulaNoAssociatedFileUA1 verifies that under PDF/UA-1 (PDF 1.7) the
// formula still gets a Formula element with /Alt, but no associated file —
// /AF is a PDF 2.0 feature.
func TestFormulaNoAssociatedFileUA1(t *testing.T) {
	const html = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p>See <math><mi>x</mi></math> here.</p></body>
	</html>`
	root, content := renderMathForStructTree(t, html, document.FormatPDFUA)
	if findSE(root, "Formula") == nil {
		t.Error("UA-1: no Formula structure element")
	}
	if strings.Contains(content, "application/mathml+xml") {
		t.Error("UA-1: MathML associated file embedded, but /AF is PDF 2.0 only")
	}
}

// TestInlineFormulaMarkedContentSplit verifies the page content stream splits
// the paragraph's marked content around the formula: a /Formula BDC sits
// between the two /P (or namespaced) runs, each balanced by an EMC.
func TestInlineFormulaMarkedContentSplit(t *testing.T) {
	const html = `<html>
		<head><style>math { font-family: math }</style></head>
		<body><p>before <math><mi>x</mi></math> after</p></body>
	</html>`
	_, content := renderMathForStructTree(t, html, document.FormatPDFUA)

	// The Formula BDC must appear, and there must be at least two BDCs whose
	// role is the paragraph role (P) — one before and one after the formula.
	if !strings.Contains(content, "/Formula <</MCID") {
		t.Errorf("no /Formula marked-content sequence in page stream")
	}
	pRuns := strings.Count(content, "/P <</MCID")
	if pRuns < 2 {
		t.Errorf("paragraph marked content not split around the formula: found %d /P runs, want >= 2", pRuns)
	}
	// Marked content must stay balanced. Opens are both BDC (tagged
	// content) and BMC (artifacts, e.g. the page background); every open
	// needs a matching EMC.
	opens := strings.Count(content, " BDC\n") + strings.Count(content, " BMC\n")
	emc := strings.Count(content, "EMC\n")
	if opens != emc {
		t.Errorf("unbalanced marked content: %d opens (BDC+BMC) vs %d EMC", opens, emc)
	}
}

// TestEnsureMathMLNamespace checks that the MathML associated file gets a
// namespaced root <math> (required by PDF/UA-2 MathML AF, pdfa11y MH-17-004),
// while existing xmlns declarations are preserved.
func TestEnsureMathMLNamespace(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "adds namespace when missing",
			in:   `<math><mi>x</mi></math>`,
			want: `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>`,
		},
		{
			name: "preserves existing namespace",
			in:   `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>`,
			want: `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>`,
		},
		{
			name: "adds namespace alongside other attributes",
			in:   `<math display="block"><mi>x</mi></math>`,
			want: `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><mi>x</mi></math>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ensureMathMLNamespace(tc.in); got != tc.want {
				t.Errorf("ensureMathMLNamespace(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
