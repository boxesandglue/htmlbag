package htmlbag

import (
	"strings"
	"testing"
)

// TestStyleInBody: a style element in the body is a stylesheet wherever it
// stands, as in a browser. It sets no text and it does not change the layout.
func TestStyleInBody(t *testing.T) {
	_, te := runStylePassWithBuilder(t, `<!DOCTYPE html><html><body>
		<p>before</p>
		<style>.big { font-size: 20pt } </style>
		<p class="big">after</p>
		<script>var hidden = 1;</script>
		</body></html>`)
	all := strings.Join(collectStrings(te.Items), " ")
	for _, word := range []string{"font-size", "hidden"} {
		if strings.Contains(all, word) {
			t.Errorf("source of a hidden element is set as text: %q", all)
		}
	}
	for _, word := range []string{"before", "after"} {
		if !strings.Contains(all, word) {
			t.Errorf("%q is lost: %q", word, all)
		}
	}
}

// TestStyleInBodyApplies: the rules of a style element in the body count.
func TestStyleInBodyApplies(t *testing.T) {
	cssParser := NewCSSParserWithDefaults()
	doc, err := cssParser.ProcessHTMLChunk(`<!DOCTYPE html><html><head>
		<style>p { color: red; margin-left: 1pt }</style></head><body>
		<p>text</p><div><style>p { color: blue }</style></div>
		</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Find("p").Nodes[0]
	styles := cssParser.ComputedStyles(p)
	if got := styles.Get("color"); got != "blue" {
		t.Errorf("color = %q, want the later rule from the body", got)
	}
	if got := styles.Get("margin-left"); got == "" {
		t.Error("rule from the head is lost")
	}
}

// TestStyleInSVGStaysInSVG: a style element in an image belongs to the image.
func TestStyleInSVGStaysInSVG(t *testing.T) {
	cssParser := NewCSSParserWithDefaults()
	doc, err := cssParser.ProcessHTMLChunk(`<!DOCTYPE html><html><body>
		<p>text</p>
		<svg xmlns="http://www.w3.org/2000/svg"><style>p { color: red }</style></svg>
		</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if got := cssParser.ComputedStyles(doc.Find("p").Nodes[0]).Get("color"); got == "red" {
		t.Error("rule from inside an SVG image applies to the document")
	}
}

// TestHiddenElementHasNoStructure: an element that shows nothing adds nothing
// to the structure tree either, and its source is no alternate text.
func TestHiddenElementHasNoStructure(t *testing.T) {
	plain := structurePaths(t, `<div><p>para</p></div><div>text</div>`)
	styled := structurePaths(t, `<div><style>p { margin: 1pt }</style><p>para</p></div><div><script>var a;</script>text</div><style>p { margin: 1pt }</style>`)
	if strings.Join(plain, "|") != strings.Join(styled, "|") {
		t.Errorf("structure differs:\n%v\n%v", plain, styled)
	}
}
