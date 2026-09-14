package htmlbag

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestFlattenNesting_AmpersandBeginning tests & at the start of a nested selector.
func TestFlattenNesting_AmpersandBeginning(t *testing.T) {
	css := `.card {
		color: blue;
		& > h2 {
			color: red;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	assertSelector(t, blocks[0], ".card")
	assertRule(t, blocks[0], "color", "blue")
	assertSelector(t, blocks[1], ".card > h2")
	assertRule(t, blocks[1], "color", "red")
}

// TestFlattenNesting_AmpersandEnd tests & at the end of a nested selector.
func TestFlattenNesting_AmpersandEnd(t *testing.T) {
	css := `h2 {
		color: blue;
		.card & {
			color: red;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	assertSelector(t, blocks[0], "h2")
	assertSelector(t, blocks[1], ".card h2")
}

// TestFlattenNesting_AmpersandMiddle tests & in the middle of a nested selector.
func TestFlattenNesting_AmpersandMiddle(t *testing.T) {
	css := `.a {
		.b & .c {
			color: green;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	assertSelector(t, blocks[0], ".b .a .c")
}

// TestFlattenNesting_NoAmpersand tests implicit descendant combinator when & is absent.
func TestFlattenNesting_NoAmpersand(t *testing.T) {
	css := `.card {
		h2 {
			font-size: 2em;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	assertSelector(t, blocks[0], ".card h2")
	assertRule(t, blocks[0], "font-size", "2em")
}

// TestFlattenNesting_AmpersandSuffix tests &.class (no space, class appended to parent).
func TestFlattenNesting_AmpersandSuffix(t *testing.T) {
	css := `.card {
		&.active {
			background: yellow;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	assertSelector(t, blocks[0], ".card.active")
}

// TestFlattenNesting_DeepNesting tests multiple levels of nesting.
func TestFlattenNesting_DeepNesting(t *testing.T) {
	css := `.a {
		.b {
			.c {
				color: red;
			}
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	assertSelector(t, blocks[0], ".a .b .c")
}

// TestFlattenNesting_MixedRulesAndNesting tests a block with both rules and nested blocks.
func TestFlattenNesting_MixedRulesAndNesting(t *testing.T) {
	css := `.card {
		padding: 1em;
		border: 1pt solid black;
		& > .title {
			font-weight: bold;
		}
		& > .body {
			font-size: 0.9em;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	assertSelector(t, blocks[0], ".card")
	assertRule(t, blocks[0], "padding", "1em")
	assertSelector(t, blocks[1], ".card > .title")
	assertRule(t, blocks[1], "font-weight", "bold")
	assertSelector(t, blocks[2], ".card > .body")
	assertRule(t, blocks[2], "font-size", "0.9em")
}

// TestFlattenNesting_SiblingCombinator tests + combinator with nesting.
func TestFlattenNesting_SiblingCombinator(t *testing.T) {
	css := `.a {
		& + .b {
			margin-top: 0;
		}
	}`
	blocks := parseAndFlatten(t, css)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	assertSelector(t, blocks[0], ".a + .b")
}

// TestFlattenNesting_Integration tests that nested CSS actually applies to HTML elements.
func TestFlattenNesting_Integration(t *testing.T) {
	htmlStr := `<html><head></head><body>
		<div class="card">
			<h2>Title</h2>
			<p>Body</p>
		</div>
	</body></html>`

	css := `.card {
		color: blue;
		& > h2 {
			color: red;
		}
		& > p {
			font-size: 10pt;
		}
	}`

	c := NewCSSParser()
	if err := c.AddCSSText(css); err != nil {
		t.Fatal(err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.ApplyCSS(doc)
	if err != nil {
		t.Fatal(err)
	}

	// Check that .card got color: blue
	card := doc.Find(".card")
	if val, exists := card.Attr("!color"); !exists || val != "blue" {
		t.Errorf(".card !color = %q (exists=%v), want 'blue'", val, exists)
	}

	// Check that h2 got color: red
	h2 := doc.Find(".card > h2")
	if val, exists := h2.Attr("!color"); !exists || val != "red" {
		t.Errorf(".card > h2 !color = %q (exists=%v), want 'red'", val, exists)
	}

	// Check that p got font-size: 10pt
	p := doc.Find(".card > p")
	if val, exists := p.Attr("!font-size"); !exists || val != "10pt" {
		t.Errorf(".card > p !font-size = %q (exists=%v), want '10pt'", val, exists)
	}
}

// --- helpers ---

func parseAndFlatten(t *testing.T, css string) []*sBlock {
	t.Helper()
	toks := tokenizeCSSString(css)
	block := consumeBlock(toks, false)
	return flattenNestedBlocks(block.blocks)
}

func assertSelector(t *testing.T, block *sBlock, want string) {
	t.Helper()
	got := selectorString(block.componentValues)
	// Normalize whitespace for comparison
	got = strings.Join(strings.Fields(got), " ")
	want = strings.Join(strings.Fields(want), " ")
	if got != want {
		t.Errorf("selector = %q, want %q", got, want)
	}
}

func assertRule(t *testing.T, block *sBlock, key, wantVal string) {
	t.Helper()
	for _, r := range block.rules {
		k := strings.TrimSpace(r.key.String())
		if k == key {
			v := strings.TrimSpace(stringValue(r.value))
			if v != wantVal {
				t.Errorf("rule %s = %q, want %q", key, v, wantVal)
			}
			return
		}
	}
	t.Errorf("rule %s not found", key)
}
