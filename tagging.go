package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// htmlToPDFRole maps HTML element names to PDF structure element roles.
var htmlToPDFRole = map[string]string{
	"h1":         "H1",
	"h2":         "H2",
	"h3":         "H3",
	"h4":         "H4",
	"h5":         "H5",
	"h6":         "H6",
	"p":          "P",
	"div":        "Div",
	"span":       "Span",
	"a":          "Link",
	"img":        "Figure",
	"figure":     "Figure",
	"table":      "Table",
	"thead":      "THead",
	"tbody":      "TBody",
	"tfoot":      "TFoot",
	"tr":         "TR",
	"th":         "TH",
	"td":         "TD",
	"ul":         "L",
	"ol":         "L",
	"li":         "LI",
	"blockquote": "BlockQuote",
	"code":       "Code",
	// PDF/UA-1 (ISO 14289-1, based on PDF 1.7) treats Code as an inline
	// structure element — peer of Span/Quote/Link, never a top-level block.
	// Goldmark renders Markdown code blocks as <pre><code>…</code></pre>;
	// mapping <pre> to P keeps the inner <code> as inline Code inside a
	// proper block container, which is what PAC expects. Without this
	// PAC reports "Possibly inappropriate use of a Code structure element"
	// for every fenced code block in the document.
	"pre":     "P",
	"section": "Sect",
	"article": "Art",
}

// pdfRoleForTag returns the PDF structure element role for an HTML tag.
// If no mapping exists, it returns an empty string.
func pdfRoleForTag(htmlTag string) string {
	return htmlToPDFRole[htmlTag]
}

// tagVList sets the "tag" attribute on a VList so that the backend emits
// the correct BDC/EMC marked content operators during shipout.
func tagVList(vl *node.VList, se *document.StructureElement) {
	if vl.Attributes == nil {
		vl.Attributes = node.H{}
	}
	vl.Attributes["tag"] = se
}

// tagVListAsXObjectFigure marks the VList as a Figure whose body is a single
// imported PDF Form XObject. Instead of emitting a BDC/EMC pair on the page
// around the /Do, the backend will route the structure attachment via a
// /StructParent on the XObject and an OBJR entry in se.K — PDF 1.7 §14.7.4.4
// + PDF/UA-1 §7.1 Note 1. This is the only path that stops Adobe Acrobat's
// tag inspector from expanding the imported XObject's path operators into a
// "Pfad / Path" list under the Figure tag.
func tagVListAsXObjectFigure(vl *node.VList, se *document.StructureElement) {
	if vl.Attributes == nil {
		vl.Attributes = node.H{}
	}
	vl.Attributes["tag"] = se
	vl.Attributes["xobject-figure"] = true
}
