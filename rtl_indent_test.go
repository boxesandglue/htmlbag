package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// lineEdges reads, for every line, the linebreaker's two edge glues: the
// width of the leftskip when it is the first node and the width of the
// line-end glue when it is the last node. A missing edge is reported as -1,
// which is how a test tells "the reorder moved it" from "it is zero". The
// tests set justified paragraphs, where the edge glues carry no alignment
// stretch and their width is the linebreaker's inset alone.
type lineEdge struct{ leftskip, lineend bag.ScaledPoint }

func lineEdges(v *node.VList) []lineEdge {
	var out []lineEdge
	edge := func(n node.Node, origin string) bag.ScaledPoint {
		if g, ok := n.(*node.Glue); ok {
			if o, _ := g.Attributes["origin"].(string); o == origin {
				return g.Width
			}
		}
		return -1
	}
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.VList:
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					continue
				}
				walk(c.List)
			case *node.HList:
				if origin, _ := c.Attributes["origin"].(string); origin == "line" && c.List != nil {
					out = append(out, lineEdge{
						leftskip: edge(c.List, "leftskip"),
						lineend:  edge(node.Tail(c.List), "lineend"),
					})
				}
				walk(c.List)
			}
		}
	}
	walk(v.List)
	return out
}

// The bidi reorder of an RTL paragraph mirrors the content of a line, not
// the linebreaker's edge glues: a right float's inset has to stay on the
// right, or the lines run underneath the float.
func TestRTLFloatRightKeepsTheInsetOnTheRight(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div style="direction:rtl;text-align:justify"><div style="float:right;width:60pt;height:40pt"></div><p>`+floatProse+`</p></div>`)

	edges := lineEdges(vl)
	if len(edges) < 4 {
		t.Fatalf("got %d lines, want a paragraph longer than the float band", len(edges))
	}
	var narrowed, full int
	for i, e := range edges {
		if e.leftskip < 0 || e.lineend < 0 {
			t.Fatalf("line %d: edge glue moved by the reorder (leftskip %s, lineend %s)", i, e.leftskip, e.lineend)
		}
		if e.leftskip != 0 {
			t.Errorf("line %d: a right float must not move where the line starts, leftskip is %s", i, e.leftskip)
		}
		if e.lineend > 0 {
			narrowed++
		} else {
			full++
		}
	}
	if narrowed == 0 {
		t.Error("no line was narrowed from the right")
	}
	if full == 0 {
		t.Error("no line ran full width below the float")
	}
	if edges[0].lineend == 0 {
		t.Error("the first line beside the float is not narrowed")
	}
}

// text-indent indents the line-start edge: the left in LTR, the right in
// RTL. The frontend resolves it once the paragraph direction is known.
func TestTextIndentFollowsTheParagraphDirection(t *testing.T) {
	indent := bag.MustSP("30pt")
	cases := []struct {
		name      string
		direction string
		leftskip  bag.ScaledPoint
		lineend   bag.ScaledPoint
	}{
		{"ltr", "ltr", indent, 0},
		{"rtl", "rtl", 0, indent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vl := buildHTML(t, floatBuilder(t), `<div style="direction:`+tc.direction+`;text-align:justify"><p style="text-indent:30pt">`+floatProse+`</p></div>`)
			edges := lineEdges(vl)
			if len(edges) < 2 {
				t.Fatalf("got %d lines, want at least 2", len(edges))
			}
			if edges[0].leftskip != tc.leftskip || edges[0].lineend != tc.lineend {
				t.Errorf("first line: leftskip %s, lineend %s; want %s and %s", edges[0].leftskip, edges[0].lineend, tc.leftskip, tc.lineend)
			}
			if edges[1].leftskip != 0 || edges[1].lineend != 0 {
				t.Errorf("second line: leftskip %s, lineend %s; want no indent", edges[1].leftskip, edges[1].lineend)
			}
		})
	}
}
