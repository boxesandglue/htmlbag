package htmlbag

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Fragment.Node is the caller's own node a flow child comes from, through
// ParseHTMLFromNode: the element of a block and the first node of loose text.
// A child htmlbag sets no block for is in no fragment.
func TestFragmentNodeIsTheSourceNode(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<html><head></head><body>` +
		`<p id="a">A</p>` +
		`<p></p>` +
		`<p style="display: none">Hidden</p>` +
		`Loose <b>text</b>` +
		`<div><p>B</p></div>` +
		`</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var body *html.Node
	var find func(n *html.Node)
	find = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "body" {
				body = c
			}
			find(c)
		}
	}
	find(doc)
	var kids []*html.Node
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		kids = append(kids, c)
	}
	// kids: p#a, empty p, hidden p, "Loose ", b, div
	cb, _ := newFlowBuilder(t, "")
	te, err := cb.ParseHTMLFromNode(doc)
	if err != nil {
		t.Fatal(err)
	}
	tr := &testRegions{sizes: []Region{wide("1000pt")}}
	if err := cb.FlowText(te, tr); err != nil {
		t.Fatal(err)
	}
	want := []*html.Node{kids[0], kids[3], kids[5]}
	frs := tr.filled[0].Fragments
	if len(frs) != len(want) {
		t.Fatalf("got %d fragments, want %d", len(frs), len(want))
	}
	for i, fr := range frs {
		if fr.Node != want[i] {
			t.Errorf("fragment %d (index %d) has node %v, want %v", i, fr.Index, fr.Node, want[i])
		}
	}
}
