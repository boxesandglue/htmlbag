package htmlbag

import (
	"strings"
	"testing"
)

const documentWithStyle = `<!DOCTYPE html><html><head><style>
@page { size: 300pt 400pt; }
p { color: red; }
</style></head><body><p>x</p></body></html>`

func TestCSSAfterDocumentOverridesTheDocument(t *testing.T) {
	c := NewCSSParser()
	c.AddCSSAfterDocument("@page { size: 100pt 200pt; }\np { color: green; }\n", ".")
	doc, err := c.ProcessHTMLChunk(documentWithStyle)
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Find("p").Nodes[0]
	if got := c.ComputedStyles(p).Get("color"); got != "green" {
		t.Errorf("color = %q, want green", got)
	}
	if got := c.Pages[""].Papersize; got != "100pt 200pt" {
		t.Errorf("page size = %q, want 100pt 200pt", got)
	}
}

// Without the deferral the document wins, in the rules and in @page.
func TestCSSBeforeDocumentLoses(t *testing.T) {
	c := NewCSSParser()
	if err := c.AddCSSText("@page { size: 100pt 200pt; }\np { color: green; }\n"); err != nil {
		t.Fatal(err)
	}
	doc, err := c.ProcessHTMLChunk(documentWithStyle)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ComputedStyles(doc.Find("p").Nodes[0]).Get("color"); got != "red" {
		t.Errorf("color = %q, want red", got)
	}
}

func TestCSSAfterDocumentIsAddedOnce(t *testing.T) {
	c := NewCSSParser()
	c.AddCSSAfterDocument("p { color: green; }", ".")
	for i := 0; i < 2; i++ {
		if _, err := c.ProcessHTMLChunk("<p>x</p>"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(c.stylesheet); n != 1 {
		t.Errorf("%d stylesheets, want 1", n)
	}
}

func TestCSSAfterDocumentReportsBrokenCSS(t *testing.T) {
	c := NewCSSParser()
	c.AddCSSAfterDocument("p:nosuchpseudo { color: green; }", ".")
	_, err := c.ProcessHTMLChunk("<p>x</p>")
	if err == nil || !strings.Contains(err.Error(), "nosuchpseudo") {
		t.Errorf("err = %v, want it to name the selector", err)
	}
}
