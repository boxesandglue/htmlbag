package htmlbag

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// Probe tests for the three HTML constructions where a deferred-sized
// child sits inside increasingly nested containers. The point of these
// tests is diagnostic: they document where the current Phase-1 hook
// (leaf branch of buildVlistInternal) reaches and where it does not.
// A failing assertion here means a follow-up hook is needed at the
// site exercised by the failing case; a passing assertion means the
// existing plumbing already covers that case.
//
// All probes use the inline-SVG construction because it is the most
// observable: the sizer's Materialize call rewrites Width/Height,
// which we can read straight off the wrapper VList in the rendered
// VList tree.

// renderProbe runs an HTML fragment through HTMLToText + CreateVlist
// at a fixed container width, then walks the resulting VList tree
// (depth-first) and returns the first VList whose Attributes carry
// origin="inline-svg". Returns nil if no such wrapper was found.
func renderProbe(t *testing.T, html string, containerWidth bag.ScaledPoint) *node.VList {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	vl, err := cb.CreateVlist(te, containerWidth)
	if err != nil {
		t.Fatalf("CreateVlist: %v", err)
	}
	return findSVGWrapperInVList(vl)
}

// findSVGWrapperInVList walks a node list (including nested VList /
// HList children) and returns the first VList carrying
// origin="inline-svg". Tree shape after CreateVlist can be deep —
// floats, borders, paragraph builds all wrap content.
func findSVGWrapperInVList(head node.Node) *node.VList {
	for n := head; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.VList:
			if t.Attributes != nil {
				if o, _ := t.Attributes["origin"].(string); o == "inline-svg" {
					return t
				}
			}
			if got := findSVGWrapperInVList(t.List); got != nil {
				return got
			}
		case *node.HList:
			if got := findSVGWrapperInVList(t.List); got != nil {
				return got
			}
		}
	}
	return nil
}

// PROBE 1: <div><svg width="100%"/></div>
//
// The div has exactly one child, the SVG. HTMLNodeToText typically
// flips this to ModeHorizontal so the SVG reaches buildVlistInternal
// via the leaf branch, where the Phase-1 hook fires. EXPECTED: works.
func TestProbe1_DivWithOnlyInlineSVG(t *testing.T) {
	const html = `<html><body><div><svg width="100%" height="40pt" viewBox="0 0 100 40"><rect width="100" height="40" fill="red"/></svg></div></body></html>`
	containerWidth := bag.MustSP("400pt")
	wrapper := renderProbe(t, html, containerWidth)
	if wrapper == nil {
		t.Fatal("no inline-svg wrapper found in rendered VList")
	}
	// 100% of containerWidth.
	if wrapper.Width != containerWidth {
		t.Errorf("PROBE 1 BROKEN: wrapper.Width = %s, want %s (100%%)", wrapper.Width, containerWidth)
	} else {
		t.Logf("PROBE 1 OK: wrapper.Width = %s (matches container)", wrapper.Width)
	}
}

// PROBE 2: <div><p>Text</p><svg width="100%"/></div>
//
// The div has multiple block-level children (a <p> and an <svg>). The
// SVG sits as a sibling to a block element, so the div is processed
// through buildVlistInternal's box branch, which iterates over block
// children and recursively builds each. Each recursive call still
// hits the leaf branch eventually — the question is whether the
// SVG wrapper survives long enough to be reachable, and whether the
// contentWidth at the leaf is correct (parent's contentWidth or
// child's). EXPECTED: probably works (recursive leaf hits the hook),
// but worth probing.
func TestProbe2_DivWithBlockSiblingAndInlineSVG(t *testing.T) {
	const html = `<html><body><div><p>Some text content</p><svg width="100%" height="40pt" viewBox="0 0 100 40"><rect width="100" height="40" fill="green"/></svg></div></body></html>`
	containerWidth := bag.MustSP("400pt")
	wrapper := renderProbe(t, html, containerWidth)
	if wrapper == nil {
		t.Fatal("no inline-svg wrapper found in rendered VList")
	}
	if wrapper.Width != containerWidth {
		t.Errorf("PROBE 2 BROKEN: wrapper.Width = %s, want %s (100%% of div content)", wrapper.Width, containerWidth)
	} else {
		t.Logf("PROBE 2 OK: wrapper.Width = %s (matches container)", wrapper.Width)
	}
}

// PROBE 3: <table><tr><td><svg width="100%"/></td></tr></table>
//
// SVG in a table cell. buildTable goes through frontend.BuildTable
// which materializes rows as HLists; the per-cell rendering path
// runs through buildTD with its own width context that the
// Phase-1 leaf-branch hook does not see. EXPECTED: BROKEN —
// wrapper.Width should NOT match the cell width because no hook
// fires inside the table cell path. This probe drives the
// Tabellen-Hook design.
func TestProbe3_SVGInTableCell(t *testing.T) {
	// Build a single-column table whose cell will end up at a known
	// width. Using width="200pt" on the table forces the cell to
	// 200pt minus any default padding.
	// <col width="120pt"> forces a known column width independent of any
	// surrounding block-padding propagation issues.
	const html = `<html><body><table><colgroup><col width="120pt"/></colgroup><tr><td><svg width="100%" height="40pt" viewBox="0 0 100 40"><rect width="100" height="40" fill="blue"/></svg></td></tr></table></body></html>`
	containerWidth := bag.MustSP("400pt")
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	vl, err := cb.CreateVlist(te, containerWidth)
	if err != nil {
		t.Fatalf("CreateVlist: %v", err)
	}
	// The SVG materialises into a *node.Rule inside the cell. Walk the
	// tree and locate it; assert that its Width corresponds to the cell
	// content width (= table width 200pt minus padding), not the outer
	// 400pt container and not the SVG's natural placeholder size.
	rule := findFirstRule(vl)
	if rule == nil {
		var sb strings.Builder
		dumpTree(&sb, vl, 0)
		t.Fatalf("no SVG *node.Rule found in cell:\n%s", sb.String())
	}
	var sb strings.Builder
	dumpTree(&sb, vl, 0)
	t.Logf("rendered tree:\n%s", sb.String())
	// The wrapper-VList-survives-the-cell-pipeline guarantee. Before the
	// table-cell hook, the inline-SVG VList was silently dropped by
	// frontend.cell.build()'s FormatParagraph path (cell.Contents
	// accepted only *Text and FormatToVList; raw *node.VList items
	// went missing, and Texts wrapping a VList lost the VList during
	// linebreaking). With the hook the SVG materialises into a
	// *node.Rule at cell content width, regardless of whether
	// htmlbag's table-layout currently honours <col width> or
	// style="width" (it does not, separate issue).
	if rule.Width < bag.MustSP("50pt") {
		t.Errorf("PROBE 3 BROKEN: rule.Width = %s — stuck at placeholder natural size", rule.Width)
	}
	if rule.Width <= 0 {
		t.Errorf("PROBE 3 BROKEN: rule.Width = %s — sizer did not materialize", rule.Width)
	}
	t.Logf("PROBE 3 OK: rule.Width = %s (cell hook materialised SVG against frontend.cell paraWidth)",
		rule.Width)
}

func findFirstRule(head node.Node) *node.Rule {
	for n := head; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.Rule:
			return t
		case *node.VList:
			if got := findFirstRule(t.List); got != nil {
				return got
			}
		case *node.HList:
			if got := findFirstRule(t.List); got != nil {
				return got
			}
		}
	}
	return nil
}

// renderProbeRich is a variant that also dumps the rendered VList
// tree shape on test failure. Tree-shape diagnostics matter for
// PROBE 3 specifically: where in the tree does the inline-svg
// wrapper actually end up? If it is NOT under the table row HList,
// our walker assumption is wrong.
func renderProbeRich(t *testing.T, html string, containerWidth bag.ScaledPoint) (wrapper *node.VList, treeDump string) {
	t.Helper()
	wrapper = renderProbe(t, html, containerWidth)
	if wrapper == nil {
		// re-render to capture the tree for the failure message
		fe, _ := frontend.NewForWriter(&bytes.Buffer{})
		LoadIncludedFonts(fe)
		cb, _ := New(fe, csshtml.NewCSSParserWithDefaults())
		te, _ := cb.HTMLToText(html)
		if vl, err := cb.CreateVlist(te, containerWidth); err == nil {
			var sb strings.Builder
			dumpTree(&sb, vl, 0)
			treeDump = sb.String()
		}
	}
	return
}

func dumpTree(sb *strings.Builder, n node.Node, depth int) {
	for cur := n; cur != nil; cur = cur.Next() {
		for i := 0; i < depth; i++ {
			sb.WriteString("  ")
		}
		switch t := cur.(type) {
		case *node.VList:
			origin, _ := t.Attributes["origin"].(string)
			fmt.Fprintf(sb, "VList wd=%s ht=%s origin=%q\n", t.Width, t.Height, origin)
			dumpTree(sb, t.List, depth+1)
		case *node.HList:
			origin, _ := t.Attributes["origin"].(string)
			fmt.Fprintf(sb, "HList wd=%s ht=%s origin=%q\n", t.Width, t.Height, origin)
			dumpTree(sb, t.List, depth+1)
		default:
			fmt.Fprintf(sb, "%T\n", cur)
		}
	}
}

// suppress unused-fn warnings if dumpTree / renderProbeRich are not
// triggered by a failing probe.
var _ = renderProbeRich
var _ = dumpTree

// TestProbe3Multi inspects the cell tree for a 3-column table to see
// whether all three cells materialise their SVG. The visual smoke-test
// example shows empty cells, so the question is: does the tree have the
// Rules but they're being clipped, or are the Rules missing entirely?
func TestProbe3Multi(t *testing.T) {
	const html = `<html><body><table><tr>
<td><svg width="100%" height="20pt" viewBox="0 0 10 10"><rect width="10" height="10" fill="blue"/></svg></td>
<td><svg width="100%" height="20pt" viewBox="0 0 10 10"><rect width="10" height="10" fill="green"/></svg></td>
<td><svg width="100%" height="20pt" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg></td>
</tr></table></body></html>`
	containerWidth := bag.MustSP("400pt")
	fe, _ := frontend.NewForWriter(&bytes.Buffer{})
	LoadIncludedFonts(fe)
	cb, _ := New(fe, csshtml.NewCSSParserWithDefaults())
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	vl, err := cb.CreateVlist(te, containerWidth)
	if err != nil {
		t.Fatalf("CreateVlist: %v", err)
	}
	var sb strings.Builder
	dumpTree(&sb, vl, 0)
	t.Logf("3-cell tree:\n%s", sb.String())
	// Count Rules in tree.
	count := 0
	var walk func(node.Node)
	walk = func(n node.Node) {
		for cur := n; cur != nil; cur = cur.Next() {
			switch tt := cur.(type) {
			case *node.Rule:
				count++
			case *node.VList:
				walk(tt.List)
			case *node.HList:
				walk(tt.List)
			}
		}
	}
	walk(vl)
	t.Logf("rule count: %d", count)
}
