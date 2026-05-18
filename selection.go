package htmlbag

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
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

var preserveWhitespace = []bool{false}

// HTMLItem is a struct which represents a HTML element or a text node.
type HTMLItem struct {
	Typ        html.NodeType
	Data       string
	Dir        Mode
	Attributes map[string]string
	Styles     map[string]string
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
// DocumentNodes are ignored.
func GetHTMLItemFromHTMLNode(thisNode *html.Node, direction Mode, firstItem *HTMLItem) error {
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
			preserveWhitespace := preserveWhitespace[len(preserveWhitespace)-1]
			txt := thisNode.Data
			// When turning from vertical to horizontal (a text is always
			// horizontal material), trim the left space. TODO: honor preserve
			// whitespace setting
			if direction == ModeVertical {
				txt = strings.TrimLeftFunc(txt, isCollapsibleSpace)
			}
			if !preserveWhitespace {
				if isSpace.MatchString(txt) {
					txt = " "
				}
			}
			if !isSpace.MatchString(txt) {
				if direction == ModeVertical {
					newDir = ModeHorizontal
				}
			}
			if txt != "" {
				if !preserveWhitespace {
					txt = reLeadcloseWhtsp.ReplaceAllString(txt, " ")
					txt = reInsideWS.ReplaceAllString(txt, " ")
				}
			}
			itm.Data = txt
			itm.Typ = html.TextNode
			firstItem.Children = append(firstItem.Children, itm)
		case html.ElementNode:
			ws := preserveWhitespace[len(preserveWhitespace)-1]
			eltname := thisNode.Data
			switch eltname {
			case "body", "address", "article", "aside", "blockquote", "canvas", "col", "colgroup", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "noscript", "ol", "p", "pre", "section", "table", "tfoot", "thead", "tbody", "tr", "td", "th", "ul", "video":
				newDir = ModeVertical
			case "b", "big", "i", "small", "tt", "abbr", "acronym", "cite", "code", "dfn", "em", "kbd", "strong", "samp", "var", "a", "barcode", "bdo", "img", "map", "object", "q", "script", "span", "sub", "sup", "button", "input", "label", "select", "textarea", "svg":
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
			attributes := thisNode.Attr
			if len(attributes) > 0 {
				itm.Styles, attributes = csshtml.ResolveAttributes(attributes)
				for _, attr := range attributes {
					itm.Attributes[attr.Key] = attr.Val
				}

				for key, value := range itm.Styles {
					if key == "white-space" {
						if value == "pre" {
							ws = true
						} else {
							ws = false
						}
					}
				}
			}
			// CSS `display` can override the tag-based block/inline
			// classification above. Only the two basic keywords are
			// honoured; `display: none` is consumed downstream via
			// FormattingStyles.Hide, and exotic values (flex, grid,
			// inline-block, ...) keep the tag default.
			if disp, ok := itm.Styles["display"]; ok {
				switch disp {
				case "block":
					newDir = ModeVertical
					itm.Dir = ModeVertical
				case "inline":
					newDir = ModeHorizontal
					itm.Dir = ModeHorizontal
				}
			}
			// Inline <svg> is an opaque leaf for the HTML pipeline: its
			// children (rect / path / text / g / …) are not HTML
			// elements and must not be walked as such. Serialise the
			// subtree back to XML and stash it on the item; the
			// collectHorizontalNodes svg-case will parse it via
			// svgreader. The HTML5 parser already integrated the
			// <svg> subtree into the html.Node tree with the correct
			// nesting, so html.Render produces valid SVG XML.
			if eltname == "svg" && thisNode.FirstChild != nil {
				var buf bytes.Buffer
				if err := html.Render(&buf, thisNode); err == nil {
					itm.Attributes["_svgSource"] = buf.String()
				}
			} else if thisNode.FirstChild != nil {
				if isCustomVoidElement(eltname) {
					// Custom void elements like <barcode> are not
					// recognized as self-closing by the HTML5 parser,
					// so subsequent siblings get incorrectly nested as
					// children. Promote them back to the parent level.
					preserveWhitespace = append(preserveWhitespace, ws)
					GetHTMLItemFromHTMLNode(thisNode.FirstChild, direction, firstItem)
					preserveWhitespace = preserveWhitespace[:len(preserveWhitespace)-1]
				} else {
					preserveWhitespace = append(preserveWhitespace, ws)
					GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, itm)
					preserveWhitespace = preserveWhitespace[:len(preserveWhitespace)-1]
				}
			}
		case html.DocumentNode:
			// just passthrough
			if err := GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, firstItem); err != nil {
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

// HTMLNodeToText converts an HTML node to a *frontend.Text element.
// cb is needed so Output can collect anchors and inline-anchor markers
// onto the same builder state used at shipout. anchorPages is the
// previous-pass id → page map used to resolve CSS target-counter().
// Pass nil for anchorPages on a clean first pass.
func HTMLNodeToText(cb *CSSBuilder, n *html.Node, ss StylesStack, df *frontend.Document, anchorPages map[string]int) (*frontend.Text, error) {
	h := &HTMLItem{Dir: ModeVertical}
	GetHTMLItemFromHTMLNode(n, ModeVertical, h)
	return Output(cb, h, ss, df, anchorPages)
}
