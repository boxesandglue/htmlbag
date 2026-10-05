package htmlbag

import (
	"strings"
	"testing"
)

// A page that ends exactly full leaves the margin-bottom of its last block
// for the next page, where it is truncated, as CSS Fragmentation truncates
// a margin at an unforced break. It does not make that page hold content: a
// block taller than a page goes there and not one page further (#76).
func TestAMarginAloneDoesNotHoldAPage(t *testing.T) {
	const css = `@page { size: 240pt 200pt; margin: 16pt; }
body { font-size: 8pt; line-height: 1.3; }
.full { height: 168pt; margin: 0 0 6pt 0; }
.tall { height: 200pt; }
.float { float: left; width: 70pt; height: 200pt; }`
	for _, c := range []struct{ name, tall string }{
		{"block", `<div class="tall">TALL</div>`},
		{"float", `<div class="float">TALL</div><p>Beside the float.</p>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			html := `<!DOCTYPE html><html><body><div class="full">FULL</div>` + c.tall +
				`<p>After the tall block.</p></body></html>`
			pages, _ := renderHTMLPagesCB(t, css, html)
			var texts []string
			for _, pg := range pages {
				texts = append(texts, pageText(pg))
			}
			at := -1
			for i, s := range texts {
				if strings.Contains(s, "TALL") {
					at = i + 1
				}
			}
			if at != 2 {
				t.Errorf("the tall %s is on page %d, want 2; pages: %q", c.name, at, texts)
			}
		})
	}
}
