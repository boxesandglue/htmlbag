package htmlbag

import (
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

var (
	pdfObject     = regexp.MustCompile(`(?s)(\d+) 0 obj\s*(<<.*?>>)\s*endobj`)
	pdfActualText = regexp.MustCompile(`/ActualText\s*(<[0-9a-fA-F]*>|\((?:\\.|[^\\)])*\))`)
	pdfK          = regexp.MustCompile(`/K\s*\[([^\]]*)\]`)
	pdfRef        = regexp.MustCompile(`(\d+) 0 R`)
)

// decodePDFString decodes a hex string with a UTF-16 byte order mark or a
// literal string.
func decodePDFString(s string) string {
	if strings.HasPrefix(s, "(") {
		return strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	b, err := hex.DecodeString(strings.Trim(s, "<>"))
	if err != nil || len(b) < 2 || b[0] != 0xfe || b[1] != 0xff {
		return string(b)
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}

// kActualTexts returns, for the structure element in pdf whose /K holds a
// child with the ActualText first, the ActualTexts of all its children in
// the order of /K, "-" for a child without one.
func kActualTexts(t *testing.T, pdf, first string) []string {
	t.Helper()
	dicts := map[string]string{}
	for _, m := range pdfObject.FindAllStringSubmatch(pdf, -1) {
		dicts[m[1]] = m[2]
	}
	actualText := func(num string) string {
		if m := pdfActualText.FindStringSubmatch(dicts[num]); m != nil {
			return decodePDFString(m[1])
		}
		return "-"
	}
	for _, dict := range dicts {
		k := pdfK.FindStringSubmatch(dict)
		if k == nil {
			continue
		}
		var texts []string
		for _, ref := range pdfRef.FindAllStringSubmatch(k[1], -1) {
			texts = append(texts, actualText(ref[1]))
		}
		for _, s := range texts {
			if s == first {
				return texts
			}
		}
	}
	t.Fatalf("no element has a child with ActualText %q", first)
	return nil
}

// A block of right-to-left text with an image among its text has no
// ActualText, which would hide the Figure, and its content stream is in
// visual order. Its text runs are Spans with ActualText in reading order,
// and the P writes them and the Figure in reading order, not in the order
// of the content stream.
func TestRightToLeftTextRunsAroundFigure(t *testing.T) {
	dir := figureImages(t)
	const html = `<!DOCTYPE html><html><body>
<p dir="rtl" lang="he">שלום <img src="DIR/box.svg" alt="Box" width="10"> עולם <a href="https://example.com">קישור</a> סוף</p>
</body></html>`
	for _, format := range []document.Format{document.FormatPDFUA, document.FormatPDFUA2} {
		root, pdf := renderTagged(t, dir, html, format)
		checkMarkedContent(t, pdf)
		if got := roles(root); len(got) != 1 || !strings.EqualFold(got[0], "P") {
			t.Fatalf("%v: document children %v, want one P", format, got)
		}
		p := root.Children()[0]
		if p.ActualText != "" {
			t.Errorf("%v: P /ActualText %q hides its Figure", format, p.ActualText)
		}
		if !p.KeepOrder {
			t.Errorf("%v: P sorts its children by the content stream", format)
		}
		var got []string
		for _, c := range p.Children() {
			got = append(got, strings.ToLower(c.Role)+":"+c.ActualText+c.Alt)
		}
		want := []string{"span:שלום ", "figure:Box", "span: עולם קישור סוף"}
		if !sameStrings(got, want...) {
			t.Errorf("%v: P children %q, want %q", format, got, want)
		}
		// The link ends in the second run and is part of it.
		if run := p.Children()[2]; len(run.Children()) != 1 || !strings.EqualFold(run.Children()[0].Role, "Link") {
			t.Errorf("%v: second run has children %v, want the Link", format, roles(run))
		}
		// The PDF holds the children in reading order too.
		if got := kActualTexts(t, pdf, "שלום "); !sameStrings(got, "שלום ", "-", " עולם קישור סוף") {
			t.Errorf("%v: /K of the P %q", format, got)
		}
	}
}

// A block of left-to-right text with an image keeps its text in the P: its
// content stream is in reading order already.
func TestLeftToRightTextAroundFigureUnchanged(t *testing.T) {
	dir := figureImages(t)
	const html = `<!DOCTYPE html><html><body>
<p>Hello <img src="DIR/box.svg" alt="Box" width="10"> world</p>
</body></html>`
	root, pdf := renderTagged(t, dir, html, document.FormatPDFUA2)
	checkMarkedContent(t, pdf)
	p := root.Children()[0]
	if p.KeepOrder || len(p.Children()) != 1 || !strings.EqualFold(p.Children()[0].Role, "Figure") {
		t.Errorf("P children %v (keep order %v), want the Figure only", roles(p), p.KeepOrder)
	}
}
