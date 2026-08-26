package htmlbag

import (
	"slices"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// TestLangAttributeReachesStructElem guards the PDF/UA language contract:
// a lang= attribute that switches the language relative to the inherited
// one must surface as /Lang on the element's structure element, and
// elements that merely inherit a language (or re-declare the same one)
// must not be stamped — the structure tree inherits /Lang downwards.
func TestLangAttributeReachesStructElem(t *testing.T) {
	html := `<!DOCTYPE html><html lang="de"><body>` +
		`<p>Deutscher Absatz.</p>` +
		`<div lang="en"><h2>English heading</h2><p>English paragraph.</p></div>` +
		`<div lang="de"><p>Immer noch deutsch.</p></div>` +
		`<p lang="fr">Un paragraphe.</p>` +
		`</body></html>`

	root := renderForStructTree(t, html)
	if root == nil {
		t.Fatal("structure root is nil — PDF/UA tagging did not activate")
	}

	var divLangs []string
	langByRole := map[string][]string{}
	walkStruct(root, func(se *document.StructureElement) {
		langByRole[se.Role] = append(langByRole[se.Role], se.Lang)
		if se.Role == "Div" {
			divLangs = append(divLangs, se.Lang)
		}
	})

	found := func(role, lang string) bool {
		return slices.Contains(langByRole[role], lang)
	}

	if !found("Div", "en") {
		t.Errorf("div lang=en did not produce /Lang en on its Div, got Div langs %q", divLangs)
	}
	if !found("P", "fr") {
		t.Errorf("p lang=fr did not produce /Lang fr, got P langs %q", langByRole["P"])
	}
	// Same language as inherited: no stamp.
	for _, l := range divLangs {
		if l == "de" {
			t.Error(`div lang=de re-declares the inherited language and must not be stamped`)
		}
	}
	// Children inside the English div inherit through the tree, no stamp.
	if found("H2", "en") {
		t.Error("h2 inside div lang=en must inherit /Lang through the tree, not repeat it")
	}
	if found("P", "en") {
		t.Error("p inside div lang=en must inherit /Lang through the tree, not repeat it")
	}
}

// TestLangOnTableStructElem: a language switch on the <table> element
// lands on the Table structure element (the cells inherit it).
func TestLangOnTableStructElem(t *testing.T) {
	html := `<!DOCTYPE html><html lang="de"><body>` +
		`<table lang="en-GB"><tr><td>cell</td></tr></table>` +
		`</body></html>`

	root := renderForStructTree(t, html)
	if root == nil {
		t.Fatal("structure root is nil — PDF/UA tagging did not activate")
	}
	var tableLang string
	walkStruct(root, func(se *document.StructureElement) {
		if se.Role == "Table" {
			tableLang = se.Lang
		}
	})
	if tableLang != "en-GB" {
		t.Errorf("table lang=en-GB: got /Lang %q on Table", tableLang)
	}
}
