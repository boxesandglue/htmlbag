package htmlbag

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// structurePaths renders html and returns each element that holds text as its
// path of roles, such as "Document/Div/P text".
func structurePaths(t *testing.T, html string) []string {
	t.Helper()
	root := renderForStructTree(t, "<!DOCTYPE html><html><body>"+html+"</body></html>")
	var paths []string
	var walk func(se *document.StructureElement, path string)
	walk = func(se *document.StructureElement, path string) {
		path += "/" + se.Role
		if text := strings.TrimSpace(se.ActualText); text != "" {
			paths = append(paths, path+" "+text)
		}
		for _, child := range se.Children() {
			walk(child, path)
		}
	}
	walk(root, "")
	return paths
}

// TestTextHasAnElement: text must reach assistive technology, whatever holds
// it. Without an element it would be an artifact, which validators accept.
func TestTextHasAnElement(t *testing.T) {
	testdata := []struct {
		name string
		html string
		want []string
	}{
		{"paragraph", `<p>one</p>`, []string{"/Document/P one"}},
		{"text in a div", `<div>one</div>`, []string{"/Document/Div/P one"}},
		{"text in a box", `<div style="padding:4pt">one</div>`, []string{"/Document/Div/P one"}},
		{"text in a section", `<section>one</section>`, []string{"/Document/Sect/P one"}},
		{"text in an article", `<article>one</article>`, []string{"/Document/Art/P one"}},
		{"text in a blockquote", `<blockquote>one</blockquote>`, []string{"/Document/BlockQuote/P one"}},
		{"nested", `<div><div>one</div></div>`, []string{"/Document/Div/Div/P one"}},
		{"run in the body", `one`, []string{"/Document/P one"}},
		{"run before a block", `<div>one<p>two</p></div>`, []string{"/Document/Div/P one", "/Document/Div/P two"}},
		{"run after a block", `<div><p>one</p>two</div>`, []string{"/Document/Div/P one", "/Document/Div/P two"}},
		{"element without a role", `<main>one</main>`, []string{"/Document/P one"}},
		{"definition list", `<dl><dt>one</dt><dd>two</dd></dl>`, []string{"/Document/P one", "/Document/P two"}},
		{"run in a list item", `<ul><li>one<p>two</p></li></ul>`, []string{"/Document/L/LI/LBody/P one", "/Document/L/LI/LBody/P two"}},
	}
	for _, td := range testdata {
		t.Run(td.name, func(t *testing.T) {
			got := structurePaths(t, td.html)
			if strings.Join(got, "\n") != strings.Join(td.want, "\n") {
				t.Errorf("elements:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(td.want, "\n"))
			}
		})
	}
}

// TestEmptyBlockHasNoParagraph: nothing to read, so a background stays an
// artifact.
func TestEmptyBlockHasNoParagraph(t *testing.T) {
	root := renderForStructTree(t, `<!DOCTYPE html><html><body><div style="height:10pt;background-color:#fd0"></div><p>one</p></body></html>`)
	paragraphs := 0
	walkStruct(root, func(se *document.StructureElement) {
		if se.Role == "P" {
			paragraphs++
		}
	})
	if paragraphs != 1 {
		t.Errorf("%d paragraphs, want 1", paragraphs)
	}
}

// TestCellContentIsTaggedOnce: a table measures its cells several times, and a
// cell owns its content.
func TestCellContentIsTaggedOnce(t *testing.T) {
	testdata := []string{
		`<ul><li>one</li></ul>`,
		`<div style="padding:4pt">one</div>`,
		`<div>one<p>two</p></div>`,
	}
	for _, cell := range testdata {
		root := renderForStructTree(t, `<!DOCTYPE html><html><body><table><tr><td>`+cell+`</td></tr></table></body></html>`)
		walkStruct(root, func(se *document.StructureElement) {
			switch se.Role {
			case "L", "LI", "LBody", "Div", "P":
				t.Errorf("%s: a %s in the structure tree", cell, se.Role)
			}
		})
	}
}

func TestMarginBoxIsNoParagraph(t *testing.T) {
	root := renderForStructTree(t, `<!DOCTYPE html><html><head><style>@page { @top-center { content: "head" } }</style></head><body><p>one</p></body></html>`)
	walkStruct(root, func(se *document.StructureElement) {
		if strings.Contains(se.ActualText, "head") {
			t.Errorf("the margin box is part of the structure: %s", se.Role)
		}
	})
}

func TestRolesMissingInPDF20MapToPDF17(t *testing.T) {
	m := html5RoleMap()
	for _, role := range []string{"article", "blockquote", "code"} {
		if got := m[role].TargetNS; got != document.NamespacePDF17SSN {
			t.Errorf("%s maps to namespace %s", role, got)
		}
	}
	for _, role := range []string{"div", "section", "p"} {
		if got := m[role].TargetNS; got != document.NamespacePDF20SSN {
			t.Errorf("%s maps to namespace %s", role, got)
		}
	}
}

// taggedContent renders html as PDF/UA-2 with plain content streams.
func taggedContent(t *testing.T, html string) string {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	fe.Doc.Format = document.FormatPDFUA2
	fe.Doc.DefaultLanguageTag = "en"
	if l, err := frontend.GetLanguage("en"); err == nil {
		fe.Doc.DefaultLanguage = l
	}
	fe.Doc.CompressLevel = 0
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText("<!DOCTYPE html><html><body>" + html + "</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

var markedContent = regexp.MustCompile(`(?s)(/Artifact BMC|<</MCID \d+>> BDC)(.*?)EMC`)

// TestTextIsNoArtifact looks at the content stream. The element of a block has
// its ActualText in any case, so the structure tree alone does not show text
// that ended up as an artifact.
func TestTextIsNoArtifact(t *testing.T) {
	words := strings.Repeat("word ", 2500)
	testdata := []struct {
		name string
		html string
	}{
		{"one line, the only block", `<p style="margin:0">one</p>`},
		{"div, the only block", `<div>one</div>`},
		{"run in the body", `one`},
		{"run beside a block", `<div>one<p>two</p></div>`},
		{"paragraph over pages", `<p>` + words + `</p>`},
		{"box over pages", `<div style="border:1pt solid red">` + words + `</div>`},
	}
	for _, td := range testdata {
		t.Run(td.name, func(t *testing.T) {
			tagged := 0
			for _, m := range markedContent.FindAllStringSubmatch(taggedContent(t, td.html), -1) {
				text := strings.Contains(m[2], "TJ")
				if text && m[1] == "/Artifact BMC" {
					t.Fatalf("text in an artifact:\n%s", m[0])
				}
				if text {
					tagged++
				}
			}
			if tagged == 0 {
				t.Error("no tagged text")
			}
		})
	}
}

// TestFragmentsDoNotNest: a block with a border holds its structure element on
// the outer box only.
func TestFragmentsDoNotNest(t *testing.T) {
	out := taggedContent(t, `<pre style="border:1pt solid red">`+strings.Repeat("line\n", 200)+`</pre>`)
	for _, m := range markedContent.FindAllStringSubmatch(out, -1) {
		if strings.Contains(m[2], "BDC") {
			t.Fatalf("nested marked content:\n%.200s", m[0])
		}
	}
}
