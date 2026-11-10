package htmlbag

import (
	"regexp"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// sameRolesFold is sameRoles for the roles of both formats: PDF/UA-2 writes
// them in lower case.
func sameRolesFold(got []string, want ...string) bool {
	return strings.EqualFold(strings.Join(got, "|"), strings.Join(want, "|"))
}

// checkOneFigure fails unless fig is a Figure with the Alt alt and a BBox,
// with no Figure inside.
func checkOneFigure(t *testing.T, fig *document.StructureElement, alt string) {
	t.Helper()
	if !strings.EqualFold(fig.Role, "Figure") || fig.Alt != alt {
		t.Errorf("got %s %q, want Figure %q", fig.Role, fig.Alt, alt)
		return
	}
	if !fig.HasBBox {
		t.Errorf("Figure %q has no BBox", alt)
	}
	if got := childFigures(fig); len(got) > 0 {
		t.Errorf("Figure %q holds the Figures %q", alt, got)
	}
}

// An image with display: block in a paragraph or a heading is one Figure in
// it, and the text around it is content of the paragraph or heading, as in
// the accessibility tree of a browser (issue #118). Before, the image was a
// Figure in a Figure with the same Alt, and the text an anonymous P inside
// the P or H1, which PDF/UA-2 does not allow. An image with display: block
// alone is one Figure too.
func TestBlockImageInParagraph(t *testing.T) {
	dir := figureImages(t)
	const html = `<!DOCTYPE html><html><body>
<p><img src="DIR/box.svg" alt="Issue box" style="display: block; width: 40px">Text after a block image.</p>
<p>Before <img src="DIR/box.svg" alt="Middle box" style="display: block; width: 20px"> and after.</p>
<img src="DIR/photo.png" alt="Alone" style="display: block; height: 30px">
<h1>Heading <img src="DIR/box.svg" alt="Heading box" style="display: block; width: 20px"> text</h1>
</body></html>`
	for _, format := range []document.Format{document.FormatPDFUA, document.FormatPDFUA2} {
		root, pdf := renderTagged(t, dir, html, format)
		if got := roles(root); !sameRolesFold(got, "P", "P", "Figure", "H1") {
			t.Fatalf("%s: document children %v, want P P Figure H1", format, got)
		}
		kids := root.Children()
		for i, alt := range map[int]string{0: "Issue box", 1: "Middle box", 3: "Heading box"} {
			el := kids[i]
			if got := roles(el); !sameRolesFold(got, "Figure") {
				t.Errorf("%s: %s with %q holds %v, want one Figure", format, el.Role, alt, got)
				continue
			}
			checkOneFigure(t, el.Children()[0], alt)
			if el.ActualText != "" {
				t.Errorf("%s: %s /ActualText %q hides its Figure", format, el.Role, el.ActualText)
			}
		}
		checkOneFigure(t, kids[2], "Alone")
		if n := strings.Count(pdf, "igure <</MCID"); n != 4 {
			t.Errorf("%s: %d Figure sequences in the page, want 4", format, n)
		}
		checkMarkedContent(t, pdf)
	}
}

// A heading that holds a block is the destination of its outline entry, as
// a heading without one, which PDF/UA-2 wants (§8.8). Before, the entry
// pointed at the page.
func TestBlockImageInHeadingOutline(t *testing.T) {
	dir := figureImages(t)
	_, pdf := renderTagged(t, dir, `<!DOCTYPE html><html><body>
<h1>Heading <img src="DIR/box.svg" alt="Heading box" style="display: block; width: 20px"> text</h1>
</body></html>`, document.FormatPDFUA2)
	entry := pdfObjectWith(t, pdf, "/Title (Heading text)")
	m := regexp.MustCompile(`/Dest \[\s*(\d+) 0 R`).FindStringSubmatch(entry)
	if m == nil {
		t.Fatalf("the outline entry has no destination:\n%s", entry)
	}
	if target := pdfObjectNumber(t, pdf, m[1]); !strings.Contains(target, "/StructElem") || !strings.Contains(target, "/S /h1") {
		t.Errorf("the outline entry does not lead to the h1 element:\n%s", target)
	}
}

// pdfObjectWith returns the object of the uncompressed PDF pdf that contains
// marker.
func pdfObjectWith(t *testing.T, pdf, marker string) string {
	t.Helper()
	at := strings.Index(pdf, marker)
	if at < 0 {
		t.Fatalf("no %q in the PDF", marker)
	}
	start := strings.LastIndex(pdf[:at], " obj")
	return pdf[start : at+strings.Index(pdf[at:], "endobj")]
}

// pdfObjectNumber returns object num of the uncompressed PDF pdf.
func pdfObjectNumber(t *testing.T, pdf, num string) string {
	t.Helper()
	at := strings.Index(pdf, "\n"+num+" 0 obj")
	if at < 0 {
		t.Fatalf("no object %s in the PDF", num)
	}
	return pdf[at : at+strings.Index(pdf[at:], "endobj")]
}
