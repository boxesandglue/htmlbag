package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

// stylesToBorderWidths runs one CSS declaration block through the style
// resolver and returns the four resolved border widths.
func stylesToBorderWidths(t *testing.T, decls map[string]string) [4]bag.ScaledPoint {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	ih := &FormattingStyles{DefaultFontSize: bag.MustSP("10pt"), Fontsize: bag.MustSP("10pt")}
	if err := StylesToStyles(ih, decls, fe, ih.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	return [4]bag.ScaledPoint{ih.BorderTopWidth, ih.BorderRightWidth, ih.BorderBottomWidth, ih.BorderLeftWidth}
}

// CSS 2.1 §8.5.3: a border-style of none or hidden forces the used border
// width to zero, whatever width the shorthand implied.
func TestBorderNoneZeroesWidth(t *testing.T) {
	// csshtml expands the shorthand before htmlbag sees it, so the test
	// feeds the resolved longhands the same way the cascade does.
	resolve := func(shorthand string) map[string]string {
		styles, _ := csshtml.ResolveAttributes([]html.Attribute{
			{Key: "!border", Val: shorthand},
		})
		return styles
	}

	for _, shorthand := range []string{"none", "0", "hidden"} {
		t.Run(shorthand, func(t *testing.T) {
			got := stylesToBorderWidths(t, resolve(shorthand))
			for i, w := range got {
				if w != 0 {
					t.Errorf("border: %s left side %d at width %s, want 0", shorthand, i, w)
				}
			}
		})
	}

	t.Run("solidKeepsWidth", func(t *testing.T) {
		got := stylesToBorderWidths(t, resolve("0.4pt solid black"))
		want := bag.MustSP("0.4pt")
		for i, w := range got {
			if w != want {
				t.Errorf("side %d is %s, want %s", i, w, want)
			}
		}
	})

	// A longhand after the shorthand still wins: only the sides left at
	// style none lose their width.
	t.Run("longhandOverridesShorthand", func(t *testing.T) {
		styles := resolve("none")
		styles["border-top-style"] = "solid"
		styles["border-top-width"] = "2pt"
		got := stylesToBorderWidths(t, styles)
		if want := bag.MustSP("2pt"); got[0] != want {
			t.Errorf("top border is %s, want %s", got[0], want)
		}
		for i := 1; i < 4; i++ {
			if got[i] != 0 {
				t.Errorf("side %d is %s, want 0", i, got[i])
			}
		}
	})
}
