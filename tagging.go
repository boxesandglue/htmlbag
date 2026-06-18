package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// mathMLNamespace is the MathML namespace URI required on the root <math>
// element of a standalone MathML document (PDF/UA-2 associated file,
// MathML Core).
const mathMLNamespace = "http://www.w3.org/1998/Math/MathML"

// ensureMathMLNamespace re-attaches the MathML namespace declaration to the
// root <math> element when it is missing. The HTML5 parser tracks the
// namespace implicitly, but html.Render drops the xmlns attribute when
// serialising foreign content; as a standalone associated file the MathML
// must be self-describing XML. If src already declares any xmlns it is left
// untouched.
func ensureMathMLNamespace(src string) string {
	if strings.Contains(src, "xmlns") {
		return src
	}
	return strings.Replace(src, "<math", `<math xmlns="`+mathMLNamespace+`"`, 1)
}

// pdfTagInfo records the PDF SSN role name and its HTML5 equivalent (if any)
// for a structural role. The SSN name is the role identifier in the PDF 1.7
// Standard Structure Namespace (and PDF 2.0 SSN, which inherits it). The
// HTML5 name is the lowercase tag name used when emitting under the HTML5
// namespace for PDF/UA-2.
type pdfTagInfo struct {
	ssn   string
	html5 string // empty if no HTML5 equivalent
}

// htmlToPDFTag maps HTML element names to structural role info. Use
// pdfRoleForTag to resolve the format-appropriate (role, namespace) pair.
//
// pre maps to P because PDF/UA-1 treats Code as an inline structure element
// — peer of Span/Quote/Link, never a top-level block. Goldmark renders
// Markdown code blocks as <pre><code>…</code></pre>; mapping <pre> to P
// keeps the inner <code> as inline Code inside a proper block container,
// which is what PAC expects. Without this PAC reports "Possibly
// inappropriate use of a Code structure element" for every fenced code
// block in the document.
var htmlToPDFTag = map[string]pdfTagInfo{
	"h1":         {"H1", "h1"},
	"h2":         {"H2", "h2"},
	"h3":         {"H3", "h3"},
	"h4":         {"H4", "h4"},
	"h5":         {"H5", "h5"},
	"h6":         {"H6", "h6"},
	"p":          {"P", "p"},
	"div":        {"Div", "div"},
	"span":       {"Span", "span"},
	"a":          {"Link", "a"},
	"img":        {"Figure", "img"},
	"figure":     {"Figure", "figure"},
	"table":      {"Table", "table"},
	"thead":      {"THead", "thead"},
	"tbody":      {"TBody", "tbody"},
	"tfoot":      {"TFoot", "tfoot"},
	"tr":         {"TR", "tr"},
	"th":         {"TH", "th"},
	"td":         {"TD", "td"},
	"ul":         {"L", "ul"},
	"ol":         {"L", "ol"},
	"li":         {"LI", "li"},
	"blockquote": {"BlockQuote", "blockquote"},
	"code":       {"Code", "code"},
	"pre":        {"P", "p"},
	"section":    {"Sect", "section"},
	"article":    {"Art", "article"},
}

// canonicalToHTML5 maps canonical PDF SSN role names to their HTML5
// equivalents. Entries with empty html5 use the PDF 2.0 SSN namespace
// under UA-2. This map is the single source of truth for the format-
// aware role/namespace resolution in roleAndNS; htmlToPDFTag is its
// HTML-tag-keyed view (built at init time).
var canonicalToHTML5 = map[string]string{
	// PDF 2.0 SSN with no HTML5 equivalent
	"Document": "", // root element role
	"Part":     "",
	"Note":     "",
	"L":        "", // ambiguous: ul or ol; kept structural
	"LBody":    "",
	"Lbl":      "",
	// HTML5-mapped roles
	"Sect":       "section",
	"Art":        "article",
	"P":          "p",
	"H1":         "h1",
	"H2":         "h2",
	"H3":         "h3",
	"H4":         "h4",
	"H5":         "h5",
	"H6":         "h6",
	"Div":        "div",
	"Span":       "span",
	"Link":       "a",
	"Figure":     "figure",
	"Caption":    "caption",
	"Code":       "code",
	"BlockQuote": "blockquote",
	"Table":      "table",
	"THead":      "thead",
	"TBody":      "tbody",
	"TFoot":      "tfoot",
	"TR":         "tr",
	"TH":         "th",
	"TD":         "td",
	"LI":         "li",
}

// pdf17OnlyRoles lists canonical role names whose target under
// PDF/UA-2 RoleMapNS must be the PDF 1.7 Standard Structure Namespace
// rather than PDF 2.0 SSN. PDF 2.0 §14.7.2 does not retain every PDF 1.7
// inline structure type in the PDF 2.0 SSN, and veraPDF's UA-2 profile
// rejects RoleMapNS targets that don't resolve to a recognised standard
// type. The PDF 1.7 SSN is one of the three allowed namespaces under
// ISO 14289-2 §8.2.4, so mapping to it is conformant.
var pdf17OnlyRoles = map[string]bool{
	"Code": true,
}

// html5RoleMap returns the RoleMapNS contents for the HTML5 namespace
// under PDF/UA-2: each HTML5 element name we may emit as a structure
// role is mapped to its canonical equivalent in a standard namespace.
// Most roles target PDF 2.0 SSN; roles in pdf17OnlyRoles target PDF 1.7
// SSN. Built from canonicalToHTML5 so the two stay in lockstep.
func html5RoleMap() map[string]document.NamespaceRoleEntry {
	m := make(map[string]document.NamespaceRoleEntry, len(canonicalToHTML5))
	for canonical, html5 := range canonicalToHTML5 {
		if html5 == "" {
			continue
		}
		targetNS := document.NamespacePDF20SSN
		if pdf17OnlyRoles[canonical] {
			targetNS = document.NamespacePDF17SSN
		}
		m[html5] = document.NamespaceRoleEntry{
			TargetRole: canonical,
			TargetNS:   targetNS,
		}
	}
	return m
}

// roleAndNS translates a canonical PDF SSN role name to the (role, namespace)
// pair appropriate for the given document format.
//
//   - UA-1 (or any non-UA-2 format): returns (canonical, "") — empty NS means
//     no /NS attribute is written; the element falls into the default PDF 1.7
//     SSN, which is what UA-1 expects.
//   - UA-2: returns the HTML5-equivalent lowercase role with the HTML5
//     namespace if known, otherwise falls back to the canonical role with the
//     PDF 2.0 SSN namespace.
func roleAndNS(canonical string, format document.Format) (role, ns string) {
	if !format.IsPDFUA2() {
		return canonical, ""
	}
	if html5, ok := canonicalToHTML5[canonical]; ok && html5 != "" {
		return html5, document.NamespaceHTML5
	}
	return canonical, document.NamespacePDF20SSN
}

// newSE constructs a StructureElement with role and namespace resolved from
// the canonical PDF SSN name under the given format.
func newSE(canonical string, format document.Format) *document.StructureElement {
	role, ns := roleAndNS(canonical, format)
	return &document.StructureElement{Role: role, NS: ns}
}

// canonicalRoleForTag returns the canonical PDF SSN role name for an HTML
// tag, or "" if the tag has no structural mapping. Use this for internal
// logic that needs stable identifiers ("P", "LI", "Figure") regardless of
// the document format. Pass the result to newSE or roleAndNS to obtain the
// emission-ready (role, namespace) pair.
func canonicalRoleForTag(htmlTag string) string {
	if info, ok := htmlToPDFTag[htmlTag]; ok {
		return info.ssn
	}
	return ""
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
