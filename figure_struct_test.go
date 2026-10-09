package htmlbag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// roles lists the roles of the children of se.
func roles(se *document.StructureElement) []string {
	var out []string
	for _, c := range se.Children() {
		out = append(out, c.Role)
	}
	return out
}

func sameRoles(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// A <figure> with a figcaption groups a Figure with the image's Alt and a
// Caption in a Sect, in reading order. Before, the figure was one Figure
// with the Alt and no content, set at the end of the document, and the
// caption had no structure element.
func TestFigureWithCaptionStructure(t *testing.T) {
	svg := filepath.Join(t.TempDir(), "pic.svg")
	if err := os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	root := renderForStructTree(t, `<!DOCTYPE html><html><body><p>Before.</p><figure><img src="`+svg+`" alt="A box."><figcaption>The caption.</figcaption></figure><p>After.</p></body></html>`)
	if got := roles(root); !sameRoles(got, "P", "Sect", "P") {
		t.Fatalf("document children %v, want P Sect P", got)
	}
	sect := root.Children()[1]
	if got := roles(sect); !sameRoles(got, "Figure", "Caption") {
		t.Fatalf("figure children %v, want Figure Caption", got)
	}
	if alt := sect.Children()[0].Alt; alt != "A box." {
		t.Errorf("Figure Alt %q, want the image's", alt)
	}
}

// The Caption of a table is the first child of the Table element: a table
// caption is set as a block above the table, and its element stood beside
// the Table, which PDF/UA does not allow.
func TestTableCaptionStructure(t *testing.T) {
	root := renderForStructTree(t, `<!DOCTYPE html><html><body><table><caption>The caption</caption><tr><td>Cell</td></tr></table></body></html>`)
	if got := roles(root); !sameRoles(got, "Table") {
		t.Fatalf("document children %v, want Table", got)
	}
	if got := roles(root.Children()[0]); len(got) == 0 || got[0] != "Caption" {
		t.Errorf("table children %v, want Caption first", got)
	}
}
