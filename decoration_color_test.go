package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// resolveOnto runs one declaration block through the style resolver on top of
// the styles it inherits, the way a child element sees its parent's state.
func resolveOnto(t *testing.T, ih *FormattingStyles, decls StyleMap) *FormattingStyles {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := StylesToStyles(ih, decls, fe, ih.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	return ih
}

func baseStyles() *FormattingStyles {
	return &FormattingStyles{DefaultFontSize: bag.MustSP("10pt"), Fontsize: bag.MustSP("10pt")}
}

func rgb(t *testing.T, c *color.Color) string {
	t.Helper()
	if c == nil {
		return "<unset>"
	}
	return c.String()
}

// An element that declares a decoration takes the colour it is set in. Without
// this the line is stroked in the default black: the run colour is set as the
// non-stroking colour, and nothing sets the stroking one.
func TestDecorationTakesTheDeclaringElementsColour(t *testing.T) {
	ih := resolveOnto(t, baseStyles(), resolveCSSText("color: rgb(0,0,255); text-decoration-line: underline"))
	if ih.TextDecorationColor == nil || rgb(t, ih.TextDecorationColor) != rgb(t, ih.color) {
		t.Errorf("decoration colour = %s, want the element's own colour %s",
			rgb(t, ih.TextDecorationColor), rgb(t, ih.color))
	}
}

// The resolution cannot depend on which declaration the map yields first, which
// is why it happens after the property loop rather than inside it.
func TestDecorationColourIsIndependentOfDeclarationOrder(t *testing.T) {
	// Same block, different insertion order. Map iteration is randomised per
	// run, so repeating it exercises both orders.
	for i := 0; i < 32; i++ {
		ih := resolveOnto(t, baseStyles(), resolveCSSText("text-decoration-line: underline; color: rgb(0,0,255)"))
		if rgb(t, ih.TextDecorationColor) != rgb(t, ih.color) {
			t.Fatalf("run %d: decoration colour = %s, want %s", i,
				rgb(t, ih.TextDecorationColor), rgb(t, ih.color))
		}
	}
}

// Originating-element semantics: a descendant that changes colour but inherits
// the decoration must not re-tint the line.
func TestDescendantColourDoesNotRetintInheritedDecoration(t *testing.T) {
	para := resolveOnto(t, baseStyles(), resolveCSSText("color: rgb(0,0,255); text-decoration-line: underline"))
	// Without this the test passes vacuously: if nothing ever captures a
	// decoration colour, parent and child agree at <unset>.
	if para.TextDecorationColor == nil {
		t.Fatal("the paragraph captured no decoration colour, so this proves nothing")
	}
	want := rgb(t, para.TextDecorationColor)

	span := resolveOnto(t, para, resolveCSSText("color: rgb(255,0,0)"))
	if got := rgb(t, span.TextDecorationColor); got != want {
		t.Errorf("a red span inside an underlined paragraph moved the line to %s, want the paragraph's %s", got, want)
	}
}

// A descendant that declares its own decoration originates one, and takes its
// own colour.
func TestDescendantDeclaringItsOwnDecorationTakesItsOwnColour(t *testing.T) {
	para := resolveOnto(t, baseStyles(), resolveCSSText("color: rgb(0,0,255); text-decoration-line: underline"))
	span := resolveOnto(t, para, resolveCSSText("color: rgb(255,0,0); text-decoration-line: underline"))
	if rgb(t, span.TextDecorationColor) != rgb(t, span.color) {
		t.Errorf("decoration colour = %s, want the span's own colour %s",
			rgb(t, span.TextDecorationColor), rgb(t, span.color))
	}
}

// An explicit text-decoration-color still wins over currentcolor.
func TestExplicitDecorationColourWins(t *testing.T) {
	ih := resolveOnto(t, baseStyles(), resolveCSSText("color: rgb(0,0,255); text-decoration-line: underline; text-decoration-color: rgb(0,255,0)"))
	green := resolveOnto(t, baseStyles(), resolveCSSText("color: rgb(0,255,0)")).color
	if got, want := rgb(t, ih.TextDecorationColor), rgb(t, green); got != want {
		t.Errorf("explicit decoration colour = %s, want %s (the text colour is %s)", got, want, rgb(t, ih.color))
	}
}

// The real path: authors write the `text-decoration` shorthand, and the CSS
// parser expands it before the renderer sees it. This pins that the longhand the capture
// keys on is what actually arrives.
func TestDecorationColourThroughTheShorthand(t *testing.T) {
	styles := resolveCSSText("text-decoration: underline; color: rgb(0,0,255)")
	if _, ok := styles["text-decoration-line"]; !ok {
		t.Fatalf("the shorthand did not expand to text-decoration-line: %v", styles)
	}
	ih := resolveOnto(t, baseStyles(), styles)
	if ih.TextDecorationColor == nil || rgb(t, ih.TextDecorationColor) != rgb(t, ih.color) {
		t.Errorf("decoration colour = %s, want the element's own colour %s",
			rgb(t, ih.TextDecorationColor), rgb(t, ih.color))
	}
}
