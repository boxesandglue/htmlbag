package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestInlinePaddingSymmetry guards the contract that Mknodes inserts BOTH a
// padding-left and a padding-right Kern around an inline run carrying CSS
// padding. Regression history: through 2026-05-11 only the padding-right
// branch existed in nodebuilding.go, so any inline `<code>` or `<span>` with
// `padding: 0 4pt` rendered visibly asymmetric — 4pt on the right, 0 on the
// left. Symptom: a wide gap before the comma in `(H1, H2)` rendered through
// the markdown showcase. Fix lives in Mknodes (inline-Kern insertion) plus
// FormatParagraph (which now deletes SettingPaddingLeft after consuming it
// as IndentLeft so the inline path doesn't double-apply at the block level).
// 2026-05-13: switched from Glue to Kern — inline padding is rigid space
// without a Knuth-Plass breakpoint, so a trailing ")" can no longer wrap to
// the next line across the </code> font boundary.
func TestInlinePaddingSymmetry(t *testing.T) {
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

	serif := fe.FindFontFamily("serif")
	if serif == nil {
		t.Fatal("default 'serif' font family missing — htmlbag.New should register it")
	}
	innerSettings := frontend.TypesettingSettings{
		frontend.SettingPaddingLeft:  bag.MustSP("4pt"),
		frontend.SettingPaddingRight: bag.MustSP("4pt"),
		frontend.SettingFontFamily:   serif,
		frontend.SettingSize:         bag.MustSP("10pt"),
	}
	inner := &frontend.Text{
		Settings: innerSettings,
		Items:    []any{"x"},
	}
	outer := &frontend.Text{
		Settings: frontend.TypesettingSettings{
			frontend.SettingFontFamily: serif,
			frontend.SettingSize:       bag.MustSP("10pt"),
		},
		Items: []any{inner},
	}

	head, _, err := fe.Mknodes(outer)
	if err != nil {
		t.Fatalf("Mknodes: %v", err)
	}
	if head == nil {
		t.Fatal("Mknodes returned nil head")
	}

	var sawLeft, sawRight bool
	var leftWidth, rightWidth bag.ScaledPoint
	for n := head; n != nil; n = n.Next() {
		k, ok := n.(*node.Kern)
		if !ok || k.Attributes == nil {
			continue
		}
		switch k.Attributes["origin"] {
		case "padding left":
			sawLeft = true
			leftWidth = k.Kern
		case "padding right":
			sawRight = true
			rightWidth = k.Kern
		}
	}
	want := bag.MustSP("4pt")
	if !sawLeft {
		t.Error("no padding-left Kern inserted around inline Text")
	} else if leftWidth != want {
		t.Errorf("padding-left Kern width = %s, want %s", leftWidth, want)
	}
	if !sawRight {
		t.Error("no padding-right Kern inserted around inline Text")
	} else if rightWidth != want {
		t.Errorf("padding-right Kern width = %s, want %s", rightWidth, want)
	}
}

// TestBlockPaddingLeftNotDoubled guards that FormatParagraph consumes
// SettingPaddingLeft and removes it from te.Settings before delegating to
// Mknodes — otherwise a `<ul style="padding-left: 16pt">` would get the
// 16pt applied both as IndentLeft (block-level) AND as a leading inline
// glue (Mknodes), doubling the indent on every list. The contract is:
// padding at the paragraph (block) level becomes IndentLeft and the
// inline-glue path never sees it.
func TestBlockPaddingLeftNotDoubled(t *testing.T) {
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

	serif := fe.FindFontFamily("serif")
	if serif == nil {
		t.Fatal("default 'serif' font family missing — htmlbag.New should register it")
	}
	te := &frontend.Text{
		Settings: frontend.TypesettingSettings{
			frontend.SettingPaddingLeft: bag.MustSP("16pt"),
			frontend.SettingFontFamily:  serif,
			frontend.SettingSize:        bag.MustSP("10pt"),
		},
		Items: []any{"hello"},
	}
	if _, _, err := fe.FormatParagraph(te, bag.MustSP("200pt"), frontend.Family(serif)); err != nil {
		t.Fatalf("FormatParagraph: %v", err)
	}
	if _, ok := te.Settings[frontend.SettingPaddingLeft]; ok {
		t.Error("FormatParagraph did not delete SettingPaddingLeft from te.Settings; " +
			"Mknodes would now double-apply the indent as inline glue")
	}
}
