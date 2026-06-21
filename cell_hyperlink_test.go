package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// renderHTMLToPDF drives the full HTML → PDF pipeline (the same one glu
// uses) and returns the raw PDF bytes as a string so a test can grep for
// annotation dictionaries. Link annotations are plain dict objects, not
// compressed streams, so they appear verbatim in the output.
func renderHTMLToPDF(t *testing.T, html string) string {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
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
	if err := fe.Finish(); err != nil {
		t.Fatalf("frontend.Finish: %v", err)
	}
	return buf.String()
}

// TestCellHyperlinkAnnotation guards the regression where an <a> inside a
// <td> produced no Link annotation. Root cause: the table layout calls
// FormatParagraph several times per cell (min/max width measurement
// passes, then the real build) on the same Text objects, and Mknodes used
// to destructively delete SettingHyperlink from the child Text on the
// first pass — so the build pass found no hyperlink and emitted no
// annotation. The flow-paragraph anchor never hit this because it is
// formatted exactly once.
func TestCellHyperlinkAnnotation(t *testing.T) {
	// Baseline: an internal link in normal block flow must emit a Link
	// annotation with a GoTo action.
	flow := `<!DOCTYPE html><html><body>
		<p><a href="#target">flow link</a></p>
		<h2 id="target">Target heading</h2>
	</body></html>`
	pf := renderHTMLToPDF(t, flow)
	if got := strings.Count(pf, "/Link"); got != 1 {
		t.Fatalf("flow link: /Link = %d, want 1 (test harness broken)", got)
	}

	// The same anchor inside a table cell must emit the same annotation.
	cell := `<!DOCTYPE html><html><body>
		<table><tr><td><a href="#target">table-cell link</a></td><td>label</td></tr></table>
		<h2 id="target">Target heading</h2>
	</body></html>`
	pc := renderHTMLToPDF(t, cell)
	if got := strings.Count(pc, "/Link"); got != 1 {
		t.Errorf("table-cell link: /Link = %d, want 1", got)
	}
	if got := strings.Count(pc, "/GoTo"); got != 1 {
		t.Errorf("table-cell link: /GoTo = %d, want 1", got)
	}

	// Two links in two cells (one external, one internal) must both emit.
	multi := `<!DOCTYPE html><html><body>
		<table><tr>
			<td><a href="https://example.com/">ext</a></td>
			<td><a href="#target">int</a></td>
		</tr></table>
		<h2 id="target">Target heading</h2>
	</body></html>`
	pm := renderHTMLToPDF(t, multi)
	if got := strings.Count(pm, "/Link"); got != 2 {
		t.Errorf("two cell links: /Link = %d, want 2", got)
	}
	if got := strings.Count(pm, "/GoTo"); got != 1 {
		t.Errorf("two cell links: /GoTo = %d, want 1 (internal)", got)
	}
}
