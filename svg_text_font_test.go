package htmlbag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SVG text without a font-family is set: an inline <svg> takes the font of
// the element around it, an SVG in <img> serif. Before, the text was dropped
// with a warning.
func TestSVGTextWithoutFontFamily(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="30"><text x="5" y="20">Label</text></svg>`
	file := filepath.Join(t.TempDir(), "label.svg")
	if err := os.WriteFile(file, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"inline": svg,
		"img":    `<img src="` + file + `">`,
	} {
		t.Run(name, func(t *testing.T) {
			pdf := renderHTMLToPDF(t, `<!DOCTYPE html><html><body>`+body+`</body></html>`)
			if !strings.Contains(pdf, "/Type /Font") {
				t.Error("no font in the PDF, the SVG text was not set")
			}
		})
	}
}
