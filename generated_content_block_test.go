package htmlbag

import (
	"strings"
	"testing"
)

// styledStrings runs the style pass and returns the flattened text
// content of the resulting Text tree in document order.
func styledStrings(t *testing.T, html string) string {
	t.Helper()
	_, te := runStylePassWithBuilder(t, html)
	return strings.Join(collectStrings(te.Items), "")
}

// TestBlockGeneratedContentBefore is the reproduction from issue #4:
// ::before on block-level elements must render its generated content
// as the element's first inline content.
func TestBlockGeneratedContentBefore(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><style>
h1::before  { content: "XX. "; }
p::before   { content: "P-"; }
div::before { content: "D-"; }
</style></head><body>
<h1>Alpha</h1>
<p>para</p>
<div>block</div>
</body></html>`
	all := styledStrings(t, html)
	for _, want := range []string{"XX. Alpha", "P-para", "D-block"} {
		if !strings.Contains(all, want) {
			t.Errorf("block ::before content missing: want %q in %q", want, all)
		}
	}
}

// TestBlockBeforeCounter covers the heading-numbering pattern the issue
// calls out as the main impact: counter() in a block ::before must see
// the element's own counter-increment.
func TestBlockBeforeCounter(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><style>
body { counter-reset: chapter; }
h1 { counter-increment: chapter; }
h1::before { content: counter(chapter) ". "; }
</style></head><body>
<h1>One</h1>
<h1>Two</h1>
</body></html>`
	all := styledStrings(t, html)
	for _, want := range []string{"1. One", "2. Two"} {
		if !strings.Contains(all, want) {
			t.Errorf("counter in block ::before wrong: want %q in %q", want, all)
		}
	}
}

// TestBlockGeneratedContentAfter: ::after joins the trailing inline run
// when one is open, and forms an anonymous final run when the last
// child is block-level.
func TestBlockGeneratedContentAfter(t *testing.T) {
	all := styledStrings(t, `<!DOCTYPE html>
<html><head><style>div::after { content: "-end"; }</style></head>
<body><div>text</div></body></html>`)
	if !strings.Contains(all, "text-end") {
		t.Errorf("::after on trailing inline run missing: %q", all)
	}

	all = styledStrings(t, `<!DOCTYPE html>
<html><head><style>div.x::after { content: "END"; }</style></head>
<body><div class="x"><p>inner</p></div></body></html>`)
	inner, end := strings.Index(all, "inner"), strings.Index(all, "END")
	if inner == -1 || end == -1 || end < inner {
		t.Errorf("::after must follow block children: %q", all)
	}
}

// TestBlockBeforeWithBlockChild: when the first flow child is
// block-level, the ::before content becomes an anonymous run that
// precedes it instead of merging into a later inline run.
func TestBlockBeforeWithBlockChild(t *testing.T) {
	all := styledStrings(t, `<!DOCTYPE html>
<html><head><style>div.x::before { content: "B:"; }</style></head>
<body><div class="x"><p>child</p></div></body></html>`)
	b, c := strings.Index(all, "B:"), strings.Index(all, "child")
	if b == -1 || c == -1 || b > c {
		t.Errorf("::before must precede block children: %q", all)
	}
}

// TestEmptyBlockBothPseudos: an element without children still renders
// its generated content; ::before and ::after share one run so both
// land on a single line.
func TestEmptyBlockBothPseudos(t *testing.T) {
	all := styledStrings(t, `<!DOCTYPE html>
<html><head><style>
div.note::before { content: "["; }
div.note::after  { content: "]"; }
</style></head>
<body><p>a</p><div class="note"></div><p>b</p></body></html>`)
	if !strings.Contains(all, "[]") {
		t.Errorf("empty block with ::before/::after must render both adjacently: %q", all)
	}
}

// TestLiBeforeNotDoubled: <li> consumes ::before through the marker
// path (SettingPrepend node list, not a string item); the block-level
// generated-content path must not render it a second time.
func TestLiBeforeNotDoubled(t *testing.T) {
	all := styledStrings(t, `<!DOCTYPE html>
<html><head><style>li::before { content: "NUM "; }</style></head>
<body><ul><li>item</li></ul></body></html>`)
	if strings.Contains(all, "NUM") {
		t.Errorf("li ::before must stay on the marker path, found string content: %q", all)
	}
}

// TestInlineBeforeStillWorks guards the emitGeneratedContent refactor:
// the inline path keeps rendering pseudo content as before.
func TestInlineBeforeStillWorks(t *testing.T) {
	all := styledStrings(t, `<!DOCTYPE html>
<html><head><style>span::before { content: "S-"; }</style></head>
<body><p><span>x</span></p></body></html>`)
	if !strings.Contains(all, "S-x") {
		t.Errorf("inline ::before regressed: %q", all)
	}
}
