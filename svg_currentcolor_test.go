package htmlbag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// currentColor in an inline <svg> is the CSS color of the svg element, so an
// icon takes the color of the text around it (svgreader#7). An SVG in <img>
// is a document of its own and stays black, as in a browser.

const currentColorIcon = `<svg width="20pt" height="20pt" viewBox="0 0 10 10"%s><rect width="10" height="10" fill="currentColor"/></svg>`

// findVListByOrigin returns the first VList in the Text tree whose origin
// attribute is origin.
func findVListByOrigin(te *frontend.Text, origin string) *node.VList {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case *node.VList:
			if o, _ := v.Attributes["origin"].(string); o == origin {
				return v
			}
		case *frontend.Text:
			if got := findVListByOrigin(v, origin); got != nil {
				return got
			}
		}
	}
	return nil
}

// svgStream returns the drawing of the first SVG with the origin in html. A
// percentage width is materialized at 100pt.
func svgStream(t *testing.T, html, origin string) string {
	t.Helper()
	te := htmlToText(t, newCSSBuilderForTest(t), html)
	vl := findVListByOrigin(te, origin)
	if vl == nil {
		t.Fatalf("no VList with origin %s in the Text tree", origin)
	}
	if ftv := getDeferredFormatter(vl); ftv != nil {
		var err error
		if vl, err = ftv(bag.MustSP("100pt")); err != nil {
			t.Fatal(err)
		}
	}
	rule := findFirstRule(node.Node(vl))
	if rule == nil {
		t.Fatal("no rule in the SVG")
	}
	return rule.Pre
}

func TestInlineSVGCurrentColor(t *testing.T) {
	tests := []struct {
		name, html, want string
	}{
		{"color of the paragraph", `<p style="color: red">Icon ` + sprintfIcon(``) + `</p>`, "1 0 0 rg"},
		{"color of the svg element", `<p style="color: red">` + sprintfIcon(` style="color: #00f"`) + `</p>`, "0 0 1 rg"},
		{"color attribute in the svg wins", `<p style="color: red">` + sprintfIcon(` color="#0f0"`) + `</p>`, "0 1 0 rg"},
		{"percentage width", `<p style="color: red">` + strings.Replace(sprintfIcon(``), `width="20pt"`, `width="50%"`, 1) + `</p>`, "1 0 0 rg"},
		{"display block", `<div style="color: red">` + sprintfIcon(` style="display: block"`) + `</div>`, "1 0 0 rg"},
		{"black by default", `<p>` + sprintfIcon(``) + `</p>`, "0 0 0 rg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if stream := svgStream(t, `<html><body>`+tc.html+`</body></html>`, "inline-svg"); !strings.Contains(stream, tc.want) {
				t.Errorf("%q is missing in\n%s", tc.want, stream)
			}
		})
	}
}

func TestImgSVGIgnoresCurrentColor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.svg")
	if err := os.WriteFile(path, []byte(strings.Replace(sprintfIcon(``), "<svg ", `<svg xmlns="http://www.w3.org/2000/svg" `, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	pre := svgStream(t, `<html><body><p style="color: red"><img src="`+path+`" width="20pt"></p></body></html>`, "svg")
	if strings.Contains(pre, "1 0 0 rg") || !strings.Contains(pre, "0 0 0 rg") {
		t.Errorf("an SVG in <img> took the color of the text:\n%s", pre)
	}
}

func sprintfIcon(attrs string) string {
	return strings.Replace(currentColorIcon, "%s", attrs, 1)
}
