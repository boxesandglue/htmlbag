package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// runStylePassWithBuilder is runStylePass plus access to the CSSBuilder,
// for tests that need to inspect builder state (anchors).
func runStylePassWithBuilder(t *testing.T, html string) (*CSSBuilder, *frontend.Text) {
	t.Helper()
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
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	return cb, te
}

// TestInlineDisplayNone: display:none on an inline element removes the
// element and its subtree, like it always did on block elements.
func TestInlineDisplayNone(t *testing.T) {
	cb, te := runStylePassWithBuilder(t, `<p>keep <span style="display: none" id="hidden-anchor">hidden</span> end</p>`)
	all := strings.Join(collectStrings(te.Items), " ")
	if strings.Contains(all, "hidden") {
		t.Errorf("display:none inline content still present: %q", all)
	}
	if !strings.Contains(all, "keep") {
		t.Errorf("sibling content lost: %q", all)
	}
	for _, a := range cb.Anchors {
		if a.ID == "hidden-anchor" {
			t.Error("hidden element registered as anchor")
		}
	}
}

// findEmptyBorderedBlock walks a Text tree looking for a block that has no
// content items but a positive top border width — the shape an <hr>
// produces.
func findEmptyBorderedBlock(te *frontend.Text) bool {
	if len(te.Items) == 0 {
		if wd, ok := te.Settings[frontend.SettingBorderTopWidth].(bag.ScaledPoint); ok && wd > 0 {
			return true
		}
	}
	for _, itm := range te.Items {
		if sub, ok := itm.(*frontend.Text); ok && findEmptyBorderedBlock(sub) {
			return true
		}
	}
	return false
}

// TestEmptyBlockWithBorderSurvives: an element without any content whose
// rendering consists entirely of its border (<hr>) must not be dropped
// from the Text tree.
func TestEmptyBlockWithBorderSurvives(t *testing.T) {
	_, te := runStylePassWithBuilder(t, `<p>a</p><hr style="border-top: 1pt solid black"><p>b</p>`)
	if !findEmptyBorderedBlock(te) {
		t.Error("empty block with border was dropped from the Text tree")
	}

	// but a plain empty block with no decoration stays dropped
	_, te = runStylePassWithBuilder(t, `<p>a</p><div></div><p>b</p>`)
	if findEmptyBorderedBlock(te) {
		t.Error("undecorated empty block unexpectedly kept with border")
	}
}
