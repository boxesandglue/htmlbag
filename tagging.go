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
	"tr":         "TR",
	"th":         "TH",
	"td":         "TD",
	"ul":         "L",
	"ol":         "L",
	"li":         "LI",
	"blockquote": "BlockQuote",
	"code":       "Code",
	"pre":        "Code",
	"section":    "Sect",
	"article":    "Art",
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
