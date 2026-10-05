package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"golang.org/x/net/html"
)

// inlineNodeLines formats body, in which InlineNode's element replaces the
// first <x-here>, and returns the lines and, for each, whether it holds a copy
// of the node, which is a StartStop with a callback.
func inlineNodeLines(t *testing.T, css, body string) []bool {
	t.Helper()
	cb, _ := newLineModelBuilder(t, css, nil)
	doc, err := html.Parse(strings.NewReader("<html><body>" + body + "</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	ss := node.NewStartStop()
	ss.ShipoutCallback = func(node.Node) string { called = true; return "" }
	var replace func(n *html.Node) bool
	replace = func(n *html.Node) bool {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "x-here" {
				n.InsertBefore(cb.InlineNode(ss), c)
				n.RemoveChild(c)
				return true
			}
			if replace(c) {
				return true
			}
		}
		return false
	}
	if !replace(doc) {
		t.Fatal("no <x-here>")
	}
	te, err := cb.ParseHTMLFromNode(doc)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("60pt"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []bool
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch v := e.(type) {
			case *node.VList:
				walk(v.List)
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin != "line" {
					walk(v.List)
					continue
				}
				has := false
				for g := v.List; g != nil; g = g.Next() {
					if s, ok := g.(*node.StartStop); ok && s != ss && s.ShipoutCallback != nil {
						called = false
						s.ShipoutCallback(s)
						has = called
					}
				}
				lines = append(lines, has)
			}
		}
	}
	walk(vl.List)
	return lines
}

// A node from InlineNode goes into the line its element is in, as a copy
// that keeps its ShipoutCallback, whatever the styles say.
func TestInlineNodeGoesIntoItsLine(t *testing.T) {
	for _, c := range []struct {
		name, css, body string
		want            []bool
	}{
		{"in a paragraph", `p { font-size: 10pt }`, `<p>one two three <x-here></x-here>four five six</p>`, []bool{false, true}},
		{"first in a paragraph", `p { font-size: 10pt }`, `<p><x-here></x-here>one two three four five six</p>`, []bool{true, false}},
		{"in a span", `p { font-size: 10pt }`, `<p>one <b>two <x-here></x-here>three</b> four five six</p>`, []bool{true, false}},
		{"in the body", `body { font-size: 10pt }`, `one two three four five <x-here></x-here>six`, []bool{false, true}},
		{"hidden by a style", `p { font-size: 10pt } * { display: none } html, body, p { display: block }`, `<p>one <x-here></x-here>two</p>`, []bool{true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := inlineNodeLines(t, c.css, c.body)
			if len(got) != len(c.want) {
				t.Fatalf("lines with the node %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("lines with the node %v, want %v", got, c.want)
				}
			}
		})
	}
}
