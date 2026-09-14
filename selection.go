package htmlbag

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/boxesandglue/boxesandglue/frontend"
	"golang.org/x/net/html"
)

var (
	// These patterns use [ \t\n\r\f] instead of \s to exclude NBSP (U+00A0).
	// CSS treats NBSP as non-collapsible whitespace.
	isSpace          = regexp.MustCompile(`^[ \t\n\r\f]*$`)
	reLeadcloseWhtsp = regexp.MustCompile(`^[ \t\n\r\f]+|[ \t\n\r\f]+$`)
	reInsideWS       = regexp.MustCompile(`\n|[ \t\n\r\f]{2,}`)
)

// isCollapsibleSpace returns true for whitespace characters that CSS considers
// collapsible. NBSP (U+00A0) is explicitly excluded.
func isCollapsibleSpace(r rune) bool {
	return r != '\u00A0' && unicode.IsSpace(r)
}

// Mode is the progression direction of the current HTML element.
type Mode int

func (m Mode) String() string {
	if m == ModeHorizontal {
		return "→"
	}
	return "↓"
}

const (
	// ModeHorizontal represents inline progression direction.
	ModeHorizontal Mode = iota
	// ModeVertical represents block progression direction.
	ModeVertical
)

// whiteSpaceStack tracks the CSS white-space value in force, so a text node
// knows whether its whitespace collapses. Only `pre` used to be recognised;
// `pre-line` — collapse the spaces, keep the newlines — now has a mode of its
// own, which is what lets a hard break survive in ordinary prose.
var whiteSpaceStack = []frontend.WhiteSpace{frontend.WhiteSpaceNormal}

var (
	// A newline plus any horizontal space hugging it, which pre-line collapses
	// down to the newline alone.
	reNewlineRun = regexp.MustCompile(`[ \t\r\f]*\n[ \t\r\f]*`)
	// Runs of horizontal whitespace, which pre-line collapses to one space.
	reHorizWS = regexp.MustCompile(`[ \t\r\f]{2,}`)
)

// collapsesSpaces reports whether runs of whitespace collapse to one space.
func collapsesSpaces(ws frontend.WhiteSpace) bool {
	return ws != frontend.WhiteSpacePre && ws != frontend.WhiteSpacePreWrap
}

// keepsNewlines reports whether a newline survives as a forced break.
func keepsNewlines(ws frontend.WhiteSpace) bool {
	return ws != frontend.WhiteSpaceNormal && ws != frontend.WhiteSpaceNowrap
}

// HTMLItem is a struct which represents a HTML element or a text node.
type HTMLItem struct {
	Typ        html.NodeType
	Data       string
	Dir        Mode
	Attributes map[string]string
	Styles     StyleMap
	Children   []*HTMLItem
}

func (itm *HTMLItem) String() string {
	switch itm.Typ {
	case html.TextNode:
		return fmt.Sprintf("%q", itm.Data)
	case html.ElementNode:
		return fmt.Sprintf("<%s>", itm.Data)
	default:
		return fmt.Sprintf("%s", itm.Data)
	}
}

// isCustomVoidElement returns true for custom element names that should be
// treated as void (self-closing) elements. The HTML5 parser does not recognize
// custom tags as void, so <barcode ... /> gets parsed as an opening tag that
// swallows subsequent siblings as children.
func isCustomVoidElement(name string) bool {
	return name == "barcode"
}

// GetHTMLItemFromHTMLNode fills the firstItem with the contents of thisNode. Comments and
// DocumentNodes are ignored. The receiver supplies the cascade result recorded
// by ApplyCSS, which must have run on the node's document first.
func (c *CSS) GetHTMLItemFromHTMLNode(thisNode *html.Node, direction Mode, firstItem *HTMLItem) error {
	newDir := direction
	for {
		if thisNode == nil {
			break
		}
		switch thisNode.Type {
		case html.CommentNode, html.DoctypeNode:
			// ignore
		case html.TextNode:
			itm := &HTMLItem{}
			ws := whiteSpaceStack[len(whiteSpaceStack)-1]
			collapse, keepNL := collapsesSpaces(ws), keepsNewlines(ws)
			txt := thisNode.Data
			// When turning from vertical to horizontal (a text is always
			// horizontal material), trim the left space. TODO: honor preserve
			// whitespace setting
			if direction == ModeVertical {
				txt = strings.TrimLeftFunc(txt, isCollapsibleSpace)
			}
			if collapse {
				if isSpace.MatchString(txt) {
					// Whitespace-only: it collapses to a single space, except
					// under pre-line, where a newline in it still breaks.
					if keepNL && strings.ContainsRune(txt, '\n') {
						txt = "\n"
					} else {
						txt = " "
					}
				}
			}
			if !isSpace.MatchString(txt) {
				if direction == ModeVertical {
					newDir = ModeHorizontal
				}
			}
			if txt != "" && collapse {
				if keepNL {
					// pre-line: fold the spaces, keep the breaks.
					txt = reNewlineRun.ReplaceAllString(txt, "\n")
					txt = reHorizWS.ReplaceAllString(txt, " ")
				} else {
					txt = reLeadcloseWhtsp.ReplaceAllString(txt, " ")
					txt = reInsideWS.ReplaceAllString(txt, " ")
				}
			}
			itm.Data = txt
			itm.Typ = html.TextNode
			firstItem.Children = append(firstItem.Children, itm)
		case html.ElementNode:
			ws := whiteSpaceStack[len(whiteSpaceStack)-1]
			eltname := thisNode.Data
			switch eltname {
			case "body", "address", "article", "aside", "blockquote", "canvas", "col", "colgroup", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "noscript", "ol", "p", "pre", "section", "table", "tfoot", "thead", "tbody", "tr", "td", "th", "ul", "video":
				newDir = ModeVertical
			case "b", "big", "i", "small", "tt", "abbr", "acronym", "cite", "code", "dfn", "em", "kbd", "strong", "samp", "var", "a", "barcode", "bdo", "img", "map", "object", "q", "script", "span", "sub", "sup", "button", "input", "label", "select", "textarea", "svg", "math":
				newDir = ModeHorizontal
			default:
				// keep dir
			}

			itm := &HTMLItem{
				Typ:        html.ElementNode,
				Data:       thisNode.Data,
				Dir:        newDir,
				Attributes: map[string]string{},
			}
			firstItem.Children = append(firstItem.Children, itm)
			itm.Styles = c.ComputedStyles(thisNode)
			for _, attr := range thisNode.Attr {
				itm.Attributes[attr.Key] = attr.Val
			}
			switch itm.Styles.Get("white-space") {
			case "normal":
				ws = frontend.WhiteSpaceNormal
			case "nowrap":
				ws = frontend.WhiteSpaceNowrap
			case "pre":
				ws = frontend.WhiteSpacePre
			case "pre-wrap":
				ws = frontend.WhiteSpacePreWrap
			case "pre-line":
				ws = frontend.WhiteSpacePreLine
			}
			// CSS `display` can override the tag-based block/inline
			// classification above. Only the two basic keywords are
			// honoured; `display: none` is consumed downstream via
			// FormattingStyles.Hide, and exotic values (flex, grid,
			// inline-block, ...) keep the tag default.
			switch itm.Styles.Get("display") {
			case "block":
				newDir = ModeVertical
				itm.Dir = ModeVertical
			case "inline":
				newDir = ModeHorizontal
				itm.Dir = ModeHorizontal
			}
			// Inline <svg> and <math> are opaque leaves for the HTML
			// pipeline: their children (rect/path/g for SVG, mi/mn/mo
			// for MathML, …) are not HTML elements and must not be
			// walked as such. Serialise the subtree back to XML and
			// stash it on the item; collectHorizontalNodes' svg / math
			// cases will parse it via svgreader / mathml. The HTML5
			// parser already integrated each subtree into the
			// html.Node tree as foreign content with correct nesting,
			// so html.Render produces valid SVG / MathML XML.
			if (eltname == "svg" || eltname == "math") && thisNode.FirstChild != nil {
				var buf bytes.Buffer
				if eltname == "math" {
					// MathML round-trips through encoding/xml downstream
					// (see mathml.Parse). Work on a copy so the namespace
					// fixup below does not disturb the tree the styling
					// pass still walks.
					cleaned := cloneNode(thisNode)
					// Normalise the root namespace to MathML. When the <math>
					// originates from a host document in a foreign default
					// namespace (e.g. xts feeds layout content whose default
					// namespace is the xts one), the element carries a literal
					// xmlns="…host…" that would otherwise win over the MathML
					// namespace in the serialised source — making the associated
					// file fail "rooted at <math> in the MathML namespace"
					// (PDF/UA-2 §17). Drop any xmlns/xmlns:* on the root and
					// force the MathML namespace so html.Render emits the
					// canonical xmlns.
					cleaned.Namespace = "math"
					kept := cleaned.Attr[:0]
					for _, a := range cleaned.Attr {
						if a.Key == "xmlns" || a.Namespace == "xmlns" || strings.HasPrefix(a.Key, "xmlns:") {
							continue
						}
						kept = append(kept, a)
					}
					cleaned.Attr = kept
					if err := html.Render(&buf, cleaned); err == nil {
						itm.Attributes["_mathmlSource"] = buf.String()
					}
				} else if err := html.Render(&buf, thisNode); err == nil {
					itm.Attributes["_svgSource"] = buf.String()
				}
			} else if thisNode.FirstChild != nil {
				if isCustomVoidElement(eltname) {
					// Custom void elements like <barcode> are not
					// recognized as self-closing by the HTML5 parser,
					// so subsequent siblings get incorrectly nested as
					// children. Promote them back to the parent level.
					whiteSpaceStack = append(whiteSpaceStack, ws)
					c.GetHTMLItemFromHTMLNode(thisNode.FirstChild, direction, firstItem)
					whiteSpaceStack = whiteSpaceStack[:len(whiteSpaceStack)-1]
				} else {
					whiteSpaceStack = append(whiteSpaceStack, ws)
					c.GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, itm)
					whiteSpaceStack = whiteSpaceStack[:len(whiteSpaceStack)-1]
				}
			}
		case html.DocumentNode:
			// just passthrough
			if err := c.GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, firstItem); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Output: unknown node type %T", thisNode.Type)
		}
		thisNode = thisNode.NextSibling
		direction = newDir
	}
	return nil
}

// cloneNode returns a deep copy of n. Any subtree that is serialised and fed
// back into an XML parser (currently: inline MathML on its way into
// mathml.Parse) goes through a copy, so the attribute filtering below never
// touches the tree the styling pass still walks.
func cloneNode(n *html.Node) *html.Node {
	clone := &html.Node{
		Type:      n.Type,
		DataAtom:  n.DataAtom,
		Data:      n.Data,
		Namespace: n.Namespace,
		Attr:      append([]html.Attribute(nil), n.Attr...),
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		cc := cloneNode(c)
		cc.Parent = clone
		if clone.LastChild == nil {
			clone.FirstChild = cc
		} else {
			clone.LastChild.NextSibling = cc
			cc.PrevSibling = clone.LastChild
		}
		clone.LastChild = cc
	}
	return clone
}

// HTMLNodeToText converts an HTML node to a *frontend.Text element.
// cb is needed so Output can collect anchors and inline-anchor markers
// onto the same builder state used at shipout. anchorPages is the
// previous-pass id → page map used to resolve CSS target-counter().
// Pass nil for anchorPages on a clean first pass.
func HTMLNodeToText(cb *CSSBuilder, n *html.Node, ss StylesStack, df *frontend.Document, anchorPages map[string]int) (*frontend.Text, error) {
	h := &HTMLItem{Dir: ModeVertical}
	cb.css.GetHTMLItemFromHTMLNode(n, ModeVertical, h)
	return Output(cb, h, ss, df, anchorPages)
}
