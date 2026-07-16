package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestPageBreakInsideRoundTripsToVListAttribute parses a stylesheet with
// .avoid-me { page-break-inside: avoid }, runs a matching <div> through
// the same pipeline a real document would (ParseCSSString → HTMLToText →
// CreateVlist), and asserts that the materialized VList carries
// Attributes["pageBreakInside"] = "avoid". This is the load-bearing
// contract: the paginator (avoidBreakInside) reads the attribute, so if
// the value is lost anywhere between CSS parsing and VList building,
// the directive becomes silent. The leaf branch (block with only inline
// children) is exercised here because that path used to drop the
// sentinel without forwarding it.
func TestPageBreakInsideRoundTripsToVListAttribute(t *testing.T) {
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
	if err := cb.ParseCSSString(`.avoid-me { page-break-inside: avoid }`); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	const html = `<html><body><div class="avoid-me">single inline line</div></body></html>`
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}

	// Walk the Text tree until we find the div carrying class="avoid-me".
	var divText *frontend.Text
	var walk func(*frontend.Text)
	walk = func(t *frontend.Text) {
		if t == nil {
			return
		}
		if tag, _ := t.Settings[frontend.SettingDebug].(string); tag == "div" {
			divText = t
		}
		for _, itm := range t.Items {
			if child, ok := itm.(*frontend.Text); ok {
				walk(child)
			}
		}
	}
	walk(te)
	if divText == nil {
		t.Fatal("could not locate div Text in HTMLToText result")
	}
	if v, ok := divText.Settings[settingPageBreakInside]; !ok || v != "avoid" {
		t.Fatalf("div Text.Settings[settingPageBreakInside] = %v (ok=%v); want \"avoid\"", v, ok)
	}

	vl, err := cb.CreateVlist(divText, bag.MustSP("400pt"))
	if err != nil {
		t.Fatalf("CreateVlist: %v", err)
	}
	if vl == nil {
		t.Fatal("CreateVlist returned nil")
	}
	if vl.Attributes == nil {
		t.Fatal("CreateVlist returned VList with nil Attributes")
	}
	if got := vl.Attributes["pageBreakInside"]; got != "avoid" {
		t.Errorf("vl.Attributes[\"pageBreakInside\"] = %v; want \"avoid\"", got)
	}
	// And the sentinel must survive the build on the source Text's
	// Settings: buildVlistInternal strips it before FormatParagraph runs
	// (it would hit the strict unknown-setting default there) and restores
	// it afterwards, so a reflow rebuild at another page width still sees
	// the directive.
	if v, ok := divText.Settings[settingPageBreakInside]; !ok || v != "avoid" {
		t.Errorf("settingPageBreakInside not restored on divText.Settings after CreateVlist; got %v (ok=%v)", v, ok)
	}
}

// TestAvoidBreakInsidePredicate covers the avoidBreakInside helper for
// both node kinds: VList (block) and HList (table row). The paginator
// uses this predicate to decide whether to drop the empty-page guard,
// so its node-kind coverage is load-bearing.
func TestAvoidBreakInsidePredicate(t *testing.T) {
	withAttrs := func(n node.Node, attrs node.H) node.Node {
		switch t := n.(type) {
		case *node.VList:
			t.Attributes = attrs
		case *node.HList:
			t.Attributes = attrs
		}
		return n
	}
	cases := []struct {
		name string
		n    node.Node
		want bool
	}{
		{name: "VList nil Attributes", n: &node.VList{}, want: false},
		{name: "VList without pageBreakInside", n: withAttrs(&node.VList{}, node.H{}), want: false},
		{name: "VList pageBreakInside=auto", n: withAttrs(&node.VList{}, node.H{"pageBreakInside": "auto"}), want: false},
		{name: "VList pageBreakInside=avoid", n: withAttrs(&node.VList{}, node.H{"pageBreakInside": "avoid"}), want: true},
		{name: "HList pageBreakInside=avoid (table row)", n: withAttrs(&node.HList{}, node.H{"pageBreakInside": "avoid"}), want: true},
		{name: "Glyph (not break-eligible)", n: &node.Glyph{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := avoidBreakInside(tc.n); got != tc.want {
				t.Errorf("avoidBreakInside(%T) = %v; want %v", tc.n, got, tc.want)
			}
		})
	}
}
