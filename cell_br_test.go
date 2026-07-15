package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// collectGlyphs walks a node list recursively (descending into HList/VList
// children) and returns the concatenation of every glyph's source
// Components in visual order. Inter-word spaces are glue, not glyphs, so
// they do not appear — assertions below use space-free cell content.
func collectGlyphs(n node.Node, out *strings.Builder) {
	for ; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.Glyph:
			out.WriteString(t.Components)
		case *node.HList:
			collectGlyphs(t.List, out)
		case *node.VList:
			collectGlyphs(t.List, out)
		}
	}
}

// vlistText builds a VList from HTML through the real htmlbag pipeline and
// returns the extractable glyph text. CreateVlist runs the same table
// layout as a full page render, so a <br> inside a cell exercises the
// multi-pass cell measurement (min-width, max-width, build).
func vlistText(t *testing.T, html string) string {
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
	vl, err := cb.CreateVlist(te, bag.MustSP("16cm"))
	if err != nil {
		t.Fatalf("CreateVlist: %v", err)
	}
	var sb strings.Builder
	collectGlyphs(vl, &sb)
	return sb.String()
}

// TestCellHardBreak guards the regression where a <br> inside a <td>/<th>
// quadrupled the first glyph of each following line and truncated the rest
// ("DE89 3704" rendered as "DDDDE89 …"). Root cause: the table layout
// formats each cell's Text several times (min/max width, then build), and
// Mknodes' raw node.Node branch spliced the SAME shared HardBreak object
// into every pass by reference. InsertAfter then dragged the previous
// pass's stale Prev/Next chain back into the list. A <br> in normal block
// flow never hit this because a paragraph is formatted exactly once. The
// fix inserts a fresh Copy() of the node on every pass.
func TestCellHardBreak(t *testing.T) {
	// Baseline: <br> in block flow must yield each line exactly once.
	block := `<!DOCTYPE html><html><body>
		<p>Aaa<br>Bbb<br>Ccc</p>
	</body></html>`
	if got := vlistText(t, block); got != "AaaBbbCcc" {
		t.Fatalf("block <br>: glyph text = %q, want %q (test harness broken)", got, "AaaBbbCcc")
	}

	// The same <br> lines inside a table cell must render identically —
	// no duplicated line-initial glyph, no truncation.
	cell := `<!DOCTYPE html><html><body>
		<table><tr><td>Aaa<br>Bbb<br>Ccc</td><td>Label</td></tr></table>
	</body></html>`
	got := vlistText(t, cell)

	if strings.Count(got, "Bbb") != 1 || strings.Count(got, "Ccc") != 1 {
		t.Errorf("cell <br>: glyph text = %q, want each of \"Bbb\"/\"Ccc\" exactly once", got)
	}
	// The quadrupling produced runs of the line-initial glyph ("BBBB",
	// "CCCC"). Assert none survived.
	for _, bad := range []string{"BB", "CC"} {
		if strings.Contains(got, bad) {
			t.Errorf("cell <br>: glyph text = %q contains duplicated run %q (quadrupling regression)", got, bad)
		}
	}
	// And the full cell content must be present in order.
	if !strings.Contains(got, "AaaBbbCcc") {
		t.Errorf("cell <br>: glyph text = %q, want it to contain %q", got, "AaaBbbCcc")
	}
}
