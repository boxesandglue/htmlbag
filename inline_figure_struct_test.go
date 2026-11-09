package htmlbag

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// renderTagged runs html through htmlbag in format with tagging on and
// returns the structure root and the uncompressed PDF. Relative image paths
// resolve against dir.
func renderTagged(t *testing.T, dir, html string, format document.Format) (*document.StructureElement, string) {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	fe.Doc.Format = format
	fe.Doc.CompressLevel = 0
	fe.Doc.DefaultLanguageTag = "en"
	if l, err := frontend.GetLanguage("en"); err == nil {
		fe.Doc.DefaultLanguage = l
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	html = strings.ReplaceAll(html, "DIR/", dir+"/")
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

// figureImages writes the images the tests show into a temporary directory
// and returns it: box.svg and the raster photo.png.
func figureImages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "box.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="blue"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	png, err := os.ReadFile(filepath.Join("testdata", "float.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

var markedContentOp = regexp.MustCompile(`(?m)(<</MCID \d+>> BDC|BDC|BMC|EMC)\s*$`)

// checkMarkedContent fails if a marked-content sequence with an MCID opens
// inside another one, which ISO 32000 does not allow, or if the operators do
// not balance.
func checkMarkedContent(t *testing.T, pdf string) {
	t.Helper()
	var stack []bool // true: the sequence has an MCID
	for _, m := range markedContentOp.FindAllStringSubmatch(pdf, -1) {
		switch op := m[1]; {
		case op == "EMC":
			if len(stack) == 0 {
				t.Fatal("EMC without an open marked-content sequence")
			}
			stack = stack[:len(stack)-1]
		case strings.HasPrefix(op, "<</MCID"):
			for _, mcid := range stack {
				if mcid {
					t.Fatal("a marked-content sequence with an MCID opens inside another")
				}
			}
			stack = append(stack, true)
		default:
			stack = append(stack, false)
		}
	}
	if len(stack) != 0 {
		t.Fatalf("%d marked-content sequences left open", len(stack))
	}
}

// childFigures returns the Alt texts of the Figure children of se, in order.
func childFigures(se *document.StructureElement) []string {
	var alts []string
	for _, c := range se.Children() {
		if strings.EqualFold(c.Role, "Figure") {
			alts = append(alts, c.Alt)
		}
	}
	return alts
}

func sameStrings(got []string, want ...string) bool {
	return strings.Join(got, "|") == strings.Join(want, "|")
}

// A paragraph with images among its text stays a P with its text, and each
// image is a Figure of its own inside it, in its own marked-content sequence
// (issue #116). Before, the paragraph was one Figure with the alt of the
// first image: its text and the other images were lost.
func TestInlineImagesAreFigures(t *testing.T) {
	dir := figureImages(t)
	const html = `<!DOCTYPE html><html><body>
<p>An icon <svg alt="Red square" width="12" height="12" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>
and an image <img src="DIR/box.svg" alt="Blue box" width="12">, a photo <img src="DIR/photo.png" alt="Photo" height="12">
and a sized one <img src="DIR/box.svg" alt="Sized box" style="width: 5%"> in text.</p>
</body></html>`
	for _, format := range []document.Format{document.FormatPDFUA, document.FormatPDFUA2} {
		root, pdf := renderTagged(t, dir, html, format)
		if got := roles(root); len(got) != 1 || !strings.EqualFold(got[0], "P") {
			t.Fatalf("document children %v, want one P", got)
		}
		p := root.Children()[0]
		if p.ActualText != "" {
			t.Errorf("P /ActualText %q hides its Figures", p.ActualText)
		}
		if got := childFigures(p); !sameStrings(got, "Red square", "Blue box", "Photo", "Sized box") {
			t.Errorf("P Figures %q, want one per image in order", got)
		}
		for _, fig := range p.Children() {
			if !fig.HasBBox {
				t.Errorf("Figure %q has no BBox", fig.Alt)
			}
		}
		if n := strings.Count(pdf, "igure <</MCID"); n != 4 {
			t.Errorf("%d Figure sequences in the page, want 4", n)
		}
		checkMarkedContent(t, pdf)
	}
}

// A block that holds nothing but one image is still the Figure; a block with
// two images and no text is a P with two Figures.
func TestImageBlocks(t *testing.T) {
	dir := figureImages(t)
	root, pdf := renderTagged(t, dir, `<!DOCTYPE html><html><body>
<p><img src="DIR/photo.png" alt="Lone photo" height="40"></p>
<p><img src="DIR/box.svg" alt="First" width="20"><img src="DIR/box.svg" alt="Second" width="20"></p>
</body></html>`, document.FormatPDFUA)
	if got := roles(root); !sameRoles(got, "Figure", "P") {
		t.Fatalf("document children %v, want Figure P", got)
	}
	if alt := root.Children()[0].Alt; alt != "Lone photo" {
		t.Errorf("Figure Alt %q, want the image's", alt)
	}
	if got := childFigures(root.Children()[1]); !sameStrings(got, "First", "Second") {
		t.Errorf("P Figures %q, want First and Second", got)
	}
	checkMarkedContent(t, pdf)
}

// An image in a heading, a list item or a table cell is a Figure below the
// element that holds the text, which drops its /ActualText. A cell owns all
// of its content, also that of a block in it and an image alone in it.
func TestInlineFiguresInElements(t *testing.T) {
	dir := figureImages(t)
	root, pdf := renderTagged(t, dir, `<!DOCTYPE html><html><body>
<h1>Heading <img src="DIR/box.svg" alt="Heading icon" width="10"> with icon</h1>
<ul><li>Item <img src="DIR/photo.png" alt="Item photo" height="10"> with photo</li></ul>
<table><tr>
<td>Cell <img src="DIR/box.svg" alt="Cell box" width="10"> text</td>
<td><img src="DIR/photo.png" alt="Only photo" height="20"></td>
<td><img src="DIR/box.svg" alt="Percent" style="width: 50%"></td>
<td><div>Div <img src="DIR/photo.png" alt="Div photo" height="10"> text</div></td>
</tr></table>
</body></html>`, document.FormatPDFUA)

	h1 := findSE(root, "H1")
	if h1 == nil {
		t.Fatal("no H1")
	}
	if got := childFigures(h1); !sameStrings(got, "Heading icon") || h1.ActualText != "" {
		t.Errorf("H1 Figures %q, ActualText %q; want the icon and no ActualText", got, h1.ActualText)
	}
	lbody := findSE(root, "LBody")
	if lbody == nil {
		t.Fatal("no LBody")
	}
	if got := childFigures(lbody); !sameStrings(got, "Item photo") || lbody.ActualText != "" {
		t.Errorf("LBody Figures %q, ActualText %q; want the photo and no ActualText", got, lbody.ActualText)
	}
	tr := findSE(root, "TR")
	if tr == nil {
		t.Fatal("no TR")
	}
	want := []string{"Cell box", "Only photo", "Percent", "Div photo"}
	if len(tr.Children()) != len(want) {
		t.Fatalf("TR has %d cells, want %d", len(tr.Children()), len(want))
	}
	for i, td := range tr.Children() {
		if got := childFigures(td); !sameStrings(got, want[i]) || td.ActualText != "" {
			t.Errorf("cell %d: Figures %q, ActualText %q; want %q and no ActualText", i, got, td.ActualText, want[i])
		}
	}
	if n := strings.Count(pdf, "/Figure <</MCID"); n != 6 {
		t.Errorf("%d Figure sequences in the page, want 6", n)
	}
	checkMarkedContent(t, pdf)
}
