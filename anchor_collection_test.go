package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// renderForAnchors drives the same render pipeline glu uses, up to and
// including page shipout, and returns the populated CSSBuilder so the
// caller can inspect Anchors. Page-number assignment only happens at
// shipout, so we must call fe.Finish() before reading cb.Anchors.
func renderForAnchors(t *testing.T, html string) *CSSBuilder {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cssParser := csshtml.NewCSSParserWithDefaults()
	cb, err := New(fe, cssParser)
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
	return cb
}

// TestAnchorCollection_BlockIDsTracked covers the basic contract: every
// block element with an id attribute lands in cb.Anchors with the page
// it was painted on.
func TestAnchorCollection_BlockIDsTracked(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<div id="alpha">first</div>
		<div id="beta">second</div>
	</body></html>`
	cb := renderForAnchors(t, html)
	if len(cb.Anchors) != 2 {
		t.Fatalf("Anchors = %d, want 2: %#v", len(cb.Anchors), cb.Anchors)
	}
	want := map[string]bool{"alpha": true, "beta": true}
	for _, a := range cb.Anchors {
		if !want[a.ID] {
			t.Errorf("unexpected anchor id %q", a.ID)
		}
		if a.Page <= 0 {
			t.Errorf("anchor %q has Page=%d; want positive", a.ID, a.Page)
		}
		delete(want, a.ID)
	}
	if len(want) > 0 {
		t.Errorf("missing anchor ids: %v", want)
	}
}

// TestAnchorCollection_HeadingWithIDDoubleTracked verifies a heading
// with an id ends up in both Headings and Anchors and that both carry
// the same page number — that overlap is the load-bearing case for
// TOC entries that reference heading anchors.
func TestAnchorCollection_HeadingWithIDDoubleTracked(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<h1 id="chap1">Chapter 1</h1>
		<p>body</p>
	</body></html>`
	cb := renderForAnchors(t, html)
	if len(cb.Headings) != 1 {
		t.Fatalf("Headings = %d, want 1", len(cb.Headings))
	}
	if len(cb.Anchors) != 1 {
		t.Fatalf("Anchors = %d, want 1", len(cb.Anchors))
	}
	if cb.Headings[0].Text != "Chapter 1" {
		t.Errorf("Heading text = %q, want %q", cb.Headings[0].Text, "Chapter 1")
	}
	if cb.Anchors[0].ID != "chap1" {
		t.Errorf("Anchor id = %q, want %q", cb.Anchors[0].ID, "chap1")
	}
	if cb.Headings[0].Page != cb.Anchors[0].Page {
		t.Errorf("page mismatch: heading=%d, anchor=%d",
			cb.Headings[0].Page, cb.Anchors[0].Page)
	}
}

// TestAnchorCollection_NoIDsEmpty makes sure the path adds no spurious
// entries for documents without id attributes.
func TestAnchorCollection_NoIDsEmpty(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<p>plain</p>
		<div>no id</div>
	</body></html>`
	cb := renderForAnchors(t, html)
	if len(cb.Anchors) != 0 {
		t.Errorf("Anchors = %d (%#v); want 0", len(cb.Anchors), cb.Anchors)
	}
}

// TestAnchorCollection_BlockTextCaptured exercises the v2 contract that
// block-level anchors carry their textual content alongside the page.
// target-text() reads this map; without text capture it would always
// resolve to "?".
func TestAnchorCollection_BlockTextCaptured(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<h1 id="chap1">Introduction</h1>
		<p>body</p>
	</body></html>`
	cb := renderForAnchors(t, html)
	if len(cb.Anchors) != 1 {
		t.Fatalf("Anchors = %d, want 1", len(cb.Anchors))
	}
	if cb.Anchors[0].Text != "Introduction" {
		t.Errorf("Anchor.Text = %q, want %q", cb.Anchors[0].Text, "Introduction")
	}
}

// TestAnchorCollection_InlineSpan covers the v2 inline-anchor path:
// an id on a <span> sitting inside a paragraph must land in cb.Anchors
// with the paragraph's page and the span's text.
func TestAnchorCollection_InlineSpan(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<p>see <span id="formula3">x² + 1 = 0</span> for details</p>
	</body></html>`
	cb := renderForAnchors(t, html)
	var found *AnchorEntry
	for i := range cb.Anchors {
		if cb.Anchors[i].ID == "formula3" {
			found = &cb.Anchors[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Anchor id=formula3 not found in %#v", cb.Anchors)
	}
	if found.Text != "x² + 1 = 0" {
		t.Errorf("Anchor.Text = %q, want %q", found.Text, "x² + 1 = 0")
	}
	if found.Page <= 0 {
		t.Errorf("Anchor.Page = %d, want positive", found.Page)
	}
}

// TestAnchorCollection_MultipleInlineInOneParagraph guards the multi-
// index path: a single paragraph carrying several <span id> elements
// has to stamp the same page on each.
func TestAnchorCollection_MultipleInlineInOneParagraph(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<p><span id="a">alpha</span>, <span id="b">beta</span></p>
	</body></html>`
	cb := renderForAnchors(t, html)
	got := map[string]string{}
	for _, a := range cb.Anchors {
		got[a.ID] = a.Text
		if a.Page <= 0 {
			t.Errorf("anchor %q has Page=%d; want positive", a.ID, a.Page)
		}
	}
	if got["a"] != "alpha" || got["b"] != "beta" {
		t.Errorf("inline anchor texts = %v; want a=alpha, b=beta", got)
	}
}

// TestAnchorCollection_InlineALink covers `<a id="x">` — the same
// generic inline-id path the <span> case uses, just on the anchor
// element itself. This is the load-bearing form for TOC links that
// also want to be jump targets (rare but legitimate).
func TestAnchorCollection_InlineALink(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<p>see <a id="link1">here</a></p>
	</body></html>`
	cb := renderForAnchors(t, html)
	var found bool
	for _, a := range cb.Anchors {
		if a.ID == "link1" && a.Text == "here" && a.Page > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("Anchor id=link1 with text=here not found in %#v", cb.Anchors)
	}
}

// TestSetAnchorPages just covers the public setter contract. The
// anchorPages map gets read by the evaluator (Phase 4), but the setter
// itself is part of Phase 2's API surface and must accept nil cleanly.
func TestSetAnchorPages(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	cb.SetAnchorPages(map[string]int{"x": 5})
	if cb.anchorPages["x"] != 5 {
		t.Errorf("SetAnchorPages: anchorPages[x] = %d, want 5", cb.anchorPages["x"])
	}
	cb.SetAnchorPages(nil)
	if cb.anchorPages != nil {
		t.Errorf("SetAnchorPages(nil): anchorPages = %v, want nil", cb.anchorPages)
	}
}
