package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestPreEmitsPCodeHierarchy guards the PDF/UA-1 contract for fenced code
// blocks: <pre><code>…</code></pre> must produce a P > Code structure
// element subtree, never a top-level Code (which would be a misuse of an
// inline-only structure type at the block level — what PAC catches as
// "Possibly inappropriate use of a Code structure element").
//
// The fix mirrors the LI/LBody pattern: tagging stays at the VList level,
// but for <pre> blocks we synthesize a Code child of the P container and
// attach the glyph-level marked content under Code.
func TestPreEmitsPCodeHierarchy(t *testing.T) {
	html := `<!DOCTYPE html><html><body>` +
		`<p>plain paragraph</p>` +
		`<pre><code>fenced block</code></pre>` +
		`</body></html>`

	root := renderForStructTree(t, html)
	if root == nil {
		t.Fatal("structure root is nil — PDF/UA tagging did not activate")
	}

	var preP *document.StructureElement
	var plainP *document.StructureElement
	walkStruct(root, func(se *document.StructureElement) {
		if se.Role != "P" {
			return
		}
		// preP: P containing a Code child. plainP: P with no Code child.
		hasCode := false
		for _, child := range se.Children() {
			if child.Role == "Code" {
				hasCode = true
				break
			}
		}
		if hasCode {
			preP = se
		} else {
			plainP = se
		}
	})

	if preP == nil {
		t.Fatal(`no P structure element with a Code child found; ` +
			`expected <pre> to render as P > Code`)
	}
	if plainP == nil {
		t.Fatal(`no plain P (without Code child) found; ` +
			`<p>plain paragraph</p> must not gain a synthetic Code`)
	}

	// Top-level invariant: Code must NEVER be a direct child of Document.
	for _, child := range root.Children() {
		if child.Role == "Code" {
			t.Errorf("Code structure element found at document top level " +
				"(role-misuse PDF/UA-1 forbids); must be nested in a block " +
				"container like P or Div")
		}
	}
}

// renderForStructTree runs the htmlbag pipeline far enough that the
// structure tree is populated, then returns its root. The returned tree
// reflects whatever role decisions vlistbuilder.go made during VList
// construction — that's the single source of truth for PDF/UA tagging.
func renderForStructTree(t *testing.T, html string) *document.StructureElement {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	// Enable PDF/UA so htmlbag.New flips on enableTagging.
	fe.Doc.Format = document.FormatPDFUA
	fe.Doc.DefaultLanguageTag = "en"
	if l, err := frontend.GetLanguage("en"); err == nil {
		fe.Doc.DefaultLanguage = l
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
	return cb.structureRoot
}

// walkStruct visits every StructureElement in pre-order.
func walkStruct(se *document.StructureElement, visit func(*document.StructureElement)) {
	if se == nil {
		return
	}
	visit(se)
	for _, child := range se.Children() {
		walkStruct(child, visit)
	}
}
