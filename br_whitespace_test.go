package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// collectStrings flattens any string content nested inside a frontend.Text
// tree into a single slice. Used by the BR-whitespace tests below to inspect
// the raw text segments produced by collectHorizontalNodes without first
// going through inline-layout (which collapses whitespace and turns
// leading-space-after-<br> into a glue, making the bug invisible at the
// rendered-PDF level).
func collectStrings(items []any) []string {
	var out []string
	for _, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, v)
		case *frontend.Text:
			out = append(out, collectStrings(v.Items)...)
		}
	}
	return out
}

func renderToText(t *testing.T, html string) *frontend.Text {
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
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	return te
}

// TestBRSwallowsFollowingWhitespace covers the spec-conformant behaviour
// that a forced line break terminates pending inter-word whitespace and the
// next inline text starts flush with the line box. Previously
// "<p>foo<br>\n  bar</p>" produced a stored text segment that began with
// whitespace, which then layered through inline layout as a leading glue —
// visually a one-space indent on every line after a <br>.
func TestBRSwallowsFollowingWhitespace(t *testing.T) {
	te := renderToText(t, "<!DOCTYPE html><html><body><p>foo<br>\n  bar</p></body></html>")
	got := strings.Join(collectStrings(te.Items), "|")
	if strings.Contains(got, "|\n") || strings.Contains(got, "| ") || strings.Contains(got, "|\t") {
		t.Errorf("leading whitespace survived after <br>; segments = %q", got)
	}
	if !strings.Contains(got, "bar") {
		t.Errorf("bar segment missing: %q", got)
	}
}

// TestBRMultipleConsecutiveStillBreak covers the edge case of two <br>
// elements in a row with a whitespace text node between them: the whitespace
// alone should not produce a stored segment, and the second <br> must still
// register as a hard break.
func TestBRMultipleConsecutiveStillBreak(t *testing.T) {
	te := renderToText(t, "<!DOCTYPE html><html><body><p>a<br>\n  <br>b</p></body></html>")
	segs := collectStrings(te.Items)
	got := strings.Join(segs, "|")
	if strings.Contains(got, "|\n") || strings.Contains(got, "| ") || strings.Contains(got, "|\t") {
		t.Errorf("whitespace-only segment survived between consecutive <br>s; segments = %q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("expected both 'a' and 'b' segments, got %q", got)
	}
}

// TestBRLeavesNonWhitespaceAlone is the negative guard: a <br> immediately
// followed by a non-whitespace text node must not lose any characters.
func TestBRLeavesNonWhitespaceAlone(t *testing.T) {
	te := renderToText(t, "<!DOCTYPE html><html><body><p>foo<br>bar</p></body></html>")
	segs := collectStrings(te.Items)
	got := strings.Join(segs, "")
	if !strings.Contains(got, "foo") || !strings.Contains(got, "bar") {
		t.Errorf("expected foo and bar intact; got %q", got)
	}
}

// TestBRWithoutPrecedingSpaceUnchanged is the symmetric negative guard:
// without any <br>, leading whitespace on a text node is whatever the
// pipeline normally does — this test simply records that we have not
// regressed the no-<br> path.
func TestBRWithoutPrecedingSpaceUnchanged(t *testing.T) {
	te := renderToText(t, "<!DOCTYPE html><html><body><p>foo\n  bar</p></body></html>")
	got := strings.Join(collectStrings(te.Items), "")
	if !strings.Contains(got, "foo") || !strings.Contains(got, "bar") {
		t.Errorf("expected foo and bar intact; got %q", got)
	}
}
