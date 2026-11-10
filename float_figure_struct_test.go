package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// An image that floats beside the text of a container, as one a paragraph
// begins with or one between the blocks of a div, is a Figure with its Alt
// and BBox, read before the text beside it, as in the source (issue #117).
// Before, the float box was an artifact and its alt text was not in the
// file. An image without an alt text stays an artifact.
func TestFloatedImageIsFigure(t *testing.T) {
	dir := figureImages(t)
	const html = `<!DOCTYPE html><html><body>
<p><img src="DIR/photo.png" alt="Leading left" style="float: left; height: 40px">Text next to a floated photo, long enough to run beside it for a line or two.</p>
<div><img src="DIR/box.svg" alt="Div float" style="float: right; width: 20px"><p>Paragraph after a float in a div.</p></div>
<p><img src="DIR/box.svg" alt="" style="float: left; width: 20px">Beside a decorative float.</p>
</body></html>`
	for _, format := range []document.Format{document.FormatPDFUA, document.FormatPDFUA2} {
		root, pdf := renderTagged(t, dir, html, format)
		if got := roles(root); !sameRolesFold(got, "Figure", "P", "Div", "P") {
			t.Fatalf("%s: document children %v, want Figure P Div P", format, got)
		}
		checkOneFigure(t, root.Children()[0], "Leading left")
		div := root.Children()[2]
		if got := roles(div); !sameRolesFold(got, "Figure", "P") {
			t.Fatalf("%s: Div children %v, want Figure P", format, got)
		}
		checkOneFigure(t, div.Children()[0], "Div float")
		if n := strings.Count(pdf, "igure <</MCID"); n != 2 {
			t.Errorf("%s: %d Figure sequences in the page, want 2", format, n)
		}
		checkMarkedContent(t, pdf)
	}
}
