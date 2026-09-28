package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func splitRowsOf(n node.Node, out *[]*node.HList) {
	for ; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.HList:
			if t.Attributes["origin"] == "table row" {
				*out = append(*out, t)
			}
		case *node.VList:
			splitRowsOf(t.List, out)
		}
	}
}

// break-inside: auto on a row lets it break across pages; a row without it,
// or with avoid, stays whole.
func TestBreakInsideAutoLetsARowSplit(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<table><tr style="break-inside:auto"><td>a</td></tr><tr><td>b</td></tr><tr style="break-inside:avoid"><td>c</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("16cm"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []*node.HList
	splitRowsOf(vl, &rows)
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	for i, want := range []bool{true, false, false} {
		_, got := rows[i].Attributes["_split"].(frontend.RowSplitter)
		if got != want {
			t.Errorf("row %d splits: %v, want %v", i+1, got, want)
		}
	}
}
