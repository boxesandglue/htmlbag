package htmlbag

import (
	"testing"

	"github.com/boxesandglue/csshtml"
)

// TestEvaluateTargetCounter_URLForm covers the path where the anchor id
// is spelled out as url(#id) in the CSS content value.
func TestEvaluateTargetCounter_URLForm(t *testing.T) {
	tokens := csshtml.ParseContentValue(`"see page " target-counter(url(#chap1), page)`)
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2: %#v", len(tokens), tokens)
	}
	anchorPages := map[string]int{"chap1": 3}
	got := evaluateContentWithStack(tokens, StylesStack{}, anchorPages, nil, nil)
	if want := "see page 3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetCounter_AttrForm covers attr(href) lookup, which is
// the load-bearing case for `.toc a::after` TOC styling.
func TestEvaluateTargetCounter_AttrForm(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-counter(attr(href), page)`)
	attrs := map[string]string{"href": "#chap2"}
	attrLookup := func(name string) string { return attrs[name] }
	anchorPages := map[string]int{"chap2": 7}
	got := evaluateContentWithStack(tokens, StylesStack{}, anchorPages, nil, attrLookup)
	if want := "7"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetCounter_UnresolvedRendersQuestionMark verifies the
// Pass-1 contract: with a nil anchorPages map, target-counter() renders
// "?" rather than panicking or emitting an empty string. This lets the
// first pass complete and write the anchor map for the second pass.
func TestEvaluateTargetCounter_UnresolvedRendersQuestionMark(t *testing.T) {
	tokens := csshtml.ParseContentValue(`"p. " target-counter(url(#missing), page)`)
	got := evaluateContentWithStack(tokens, StylesStack{}, nil, nil, nil)
	if want := "p. ?"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetCounter_HrefWithoutHash trims the leading "#" from
// the attribute value, matching how anchors are stored in cb.Anchors
// (without the fragment marker).
func TestEvaluateTargetCounter_HrefWithoutHash(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-counter(attr(href), page)`)
	attrs := map[string]string{"href": "#alpha"}
	attrLookup := func(name string) string { return attrs[name] }
	got := evaluateContentWithStack(
		tokens,
		StylesStack{},
		map[string]int{"alpha": 5},
		nil,
		attrLookup,
	)
	if got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

// TestEvaluateTargetCounter_NonPageCounterRendersQuestionMark documents
// the v1 limit: target-counter() with a counter other than "page"
// renders "?" because we don't snapshot named counters at anchor
// positions.
func TestEvaluateTargetCounter_NonPageCounterRendersQuestionMark(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-counter(url(#x), section)`)
	got := evaluateContentWithStack(tokens, StylesStack{}, map[string]int{"x": 3}, nil, nil)
	if want := "?"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetText_ResolvedFromMap is the v2 target-text path:
// the anchorTexts map carries the captured text, evaluator emits it.
func TestEvaluateTargetText_ResolvedFromMap(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-text(url(#chap1))`)
	got := evaluateContentWithStack(
		tokens,
		StylesStack{},
		nil,
		map[string]string{"chap1": "Introduction"},
		nil,
	)
	if want := "Introduction"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetText_AttrForm covers attr(href) lookup for
// target-text — the same path used by `.toc a::before { content:
// target-text(attr(href)) }` to pull heading titles into a TOC.
func TestEvaluateTargetText_AttrForm(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-text(attr(href))`)
	attrs := map[string]string{"href": "#chap2"}
	attrLookup := func(name string) string { return attrs[name] }
	got := evaluateContentWithStack(
		tokens,
		StylesStack{},
		nil,
		map[string]string{"chap2": "Line breaking"},
		attrLookup,
	)
	if want := "Line breaking"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEvaluateTargetText_UnresolvedRendersQuestionMark keeps the
// Pass-1 contract — nil anchorTexts → "?", not empty or panic.
func TestEvaluateTargetText_UnresolvedRendersQuestionMark(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-text(url(#missing))`)
	got := evaluateContentWithStack(tokens, StylesStack{}, nil, nil, nil)
	if got != "?" {
		t.Errorf("got %q, want ?", got)
	}
}

// TestEvaluateTargetText_NonContentTypeRendersQuestionMark documents
// the v1 limit: target-text(..., before) / after / first-letter would
// need pseudo-element snapshots that we don't take.
func TestEvaluateTargetText_NonContentTypeRendersQuestionMark(t *testing.T) {
	tokens := csshtml.ParseContentValue(`target-text(url(#x), before)`)
	got := evaluateContentWithStack(
		tokens,
		StylesStack{},
		nil,
		map[string]string{"x": "Title"},
		nil,
	)
	if got != "?" {
		t.Errorf("got %q, want ?", got)
	}
}
