package htmlbag

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

var (
	isSpace          = regexp.MustCompile(`^\s*$`)
	reLeadcloseWhtsp = regexp.MustCompile(`^[\s\p{Zs}]+|[\s\p{Zs}]+$`)
	reInsideWS       = regexp.MustCompile(`\n|[\s\p{Zs}]{2,}`) //to match 2 or more whitespace symbols inside a string or NL
)

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

// GetHTMLItemFromHTMLNode fills the firstItem with the contents of thisNode. Comments and
// DocumentNodes are ignored.
func GetHTMLItemFromHTMLNode(thisNode *html.Node, direction Mode, firstItem *HTMLItem) error {
	newDir := direction
	for {
		if thisNode == nil {
			break
		}
		switch thisNode.Type {
		case html.CommentNode:
			// ignore
		case html.TextNode:
			itm := &HTMLItem{}
			preserveWhitespace := preserveWhitespace[len(preserveWhitespace)-1]
			txt := thisNode.Data
			// When turning from vertical to horizontal (a text is always
			// horizontal material), trim the left space. TODO: honor preserve
			// whitespace setting
			if direction == ModeVertical {
				txt = strings.TrimLeftFunc(txt, unicode.IsSpace)
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
			case "body", "address", "article", "aside", "blockquote", "canvas", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "noscript", "ol", "p", "pre", "section", "table", "tfoot", "thead", "tbody", "tr", "td", "th", "ul", "video":
				newDir = ModeVertical
			case "b", "big", "i", "small", "tt", "abbr", "acronym", "cite", "code", "dfn", "em", "kbd", "strong", "samp", "var", "a", "bdo", "img", "map", "object", "q", "script", "span", "sub", "sup", "button", "input", "label", "select", "textarea":
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
				// for _, attr := range attributes {
				// 	itm.Attributes[attr.Key] = attr.Val
				// }

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
			if thisNode.FirstChild != nil {
				preserveWhitespace = append(preserveWhitespace, ws)
				GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, itm)
				preserveWhitespace = preserveWhitespace[:len(preserveWhitespace)-1]
			}
		case html.DocumentNode:
			// just passthrough
			GetHTMLItemFromHTMLNode(thisNode.FirstChild, newDir, firstItem)
		default:
			return fmt.Errorf("Output: unknown node type %T", thisNode.Type)
		}
		thisNode = thisNode.NextSibling
		direction = newDir
	}
	return nil
}

// HTMLNodeToText converts an HTML node to a *frontend.Text element.
func HTMLNodeToText(n *html.Node, ss StylesStack, df *frontend.Document) (*frontend.Text, error) {
	h := &HTMLItem{Dir: ModeVertical}
	GetHTMLItemFromHTMLNode(n, ModeVertical, h)
	return Output(h, ss, df)
}
