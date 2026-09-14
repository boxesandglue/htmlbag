package htmlbag

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	scanner "github.com/speedata/css"
)

var (
	level int
	out   io.Writer

	dimen              = regexp.MustCompile(`^^[+\-]?(?:(?:0+|[1-9]\d*)(?:\.\d*)?|\.\d+)(px|mm|cm|in|pt|pc|ch|em|ex|lh|rem|0)$`)
	zeroDimen          = regexp.MustCompile(`^0+(px|mm|cm|in|pt|pc|ch|em|ex|lh|rem)?`)
	style              = regexp.MustCompile(`^none|hidden|dotted|dashed|solid|double|groove|ridge|inset|outset$`)
	toprightbottomleft = [...]string{"top", "right", "bottom", "left"}
)

func normalizespace(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

// cssQuoteString wraps s in double quotes using CSS string-literal
// escaping rules (only `"` and `\` need escaping; UTF-8 byte sequences
// pass through verbatim). Critically *not* Go's %q, which escapes
// non-ASCII codepoints as ` ` — a Go-string literal that CSS
// parsers misread (CSS escapes are `\2009 `, no `u` prefix).
func cssQuoteString(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + 2)
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	sb.WriteByte('"')
	return sb.String()
}

func stringValue(toks tokenstream) string {
	ret := []string{}
	negative := false
	prevNegative := false
	for _, tok := range toks {
		prevNegative = negative
		negative = false
		switch tok.Type {
		case scanner.Ident:
			ret = append(ret, tok.Value)
		case scanner.String:
			ret = append(ret, cssQuoteString(tok.Value))
		case scanner.Number, scanner.Dimension:
			if prevNegative {
				ret = append(ret, "-"+tok.Value)
			} else {
				ret = append(ret, tok.Value)
			}
		case scanner.Percentage:
			ret = append(ret, tok.Value+"%")
		case scanner.Hash:
			ret = append(ret, "#"+tok.Value)
		case scanner.Function:
			ret = append(ret, tok.Value+"(")
		case scanner.S:
			// ret = append(ret, " ")
		case scanner.Delim:
			switch tok.Value {
			case ";":
				// ignore
			case ",", ")":
				ret = append(ret, tok.Value)
			case "-":
				negative = true
			default:
				fmt.Println("unhandled delimiter", tok)
			}
		case scanner.URI:
			ret = append(ret, "url("+tok.Value+")")
		case scanner.Local:
			ret = append(ret, "local("+tok.Value+")")
		case scanner.Format:
			ret = append(ret, "format("+tok.Value+")")
		case scanner.Tech:
			ret = append(ret, "tech("+tok.Value+")")
		default:
			fmt.Println("tree: unhandled token", tok)
		}
	}
	return strings.Join(ret, " ")
}

// resolveStyleAttribute recurses through the HTML tree and folds every inline
// style="..." attribute into the cascade. It runs after the stylesheet rules,
// so an inline declaration wins over any selector regardless of specificity.
func (c *CSS) resolveStyleAttribute(i int, sel *goquery.Selection) {
	if a, ok := sel.Attr("style"); ok {
		decls := declarationsFromText(a)
		for _, node := range sel.Nodes {
			for _, d := range decls {
				c.setDeclaration(node, d.property, d.value)
			}
		}
	}
	sel.Children().Each(c.resolveStyleAttribute)
}

func isDimension(str string) (bool, string) {
	switch str {
	case "thick":
		return true, "2pt"
	case "medium":
		return true, "1pt"
	case "thin":
		return true, "0.5pt"
	}
	return dimen.MatchString(str), str
}
func isBorderStyle(str string) (bool, string) {
	return style.MatchString(str), str
}

// validateSelectors parses every selector of the block's rule blocks and
// returns an error naming the first selector that cascadia cannot parse.
// This reports broken selectors where the stylesheet is read, long before
// ApplyCSS runs on some unrelated content.
func validateSelectors(block sBlock) error {
	for _, b := range block.blocks {
		selector := selectorString(b.componentValues)
		if selector == "" {
			continue
		}
		if _, err := cascadia.ParseGroupWithPseudoElements(selector); err != nil {
			return fmt.Errorf("cannot parse CSS selector %q: %w", selector, err)
		}
	}
	return nil
}

// ApplyCSS resolves the CSS rules against the DOM and records the cascade
// result for every matched element on the CSS object, where ComputedStyles
// reads it back. Pseudo-element declarations keep a "name::" prefix on the
// property, e.g. "before::content". The DOM itself is left untouched; the
// values never leave their parsed token form.
//
// One CSS object holds the result of one ApplyCSS run: the next call replaces
// it. Read the styles of a document (or walk it) before applying CSS to
// another one.
func (c *CSS) ApplyCSS(doc *goquery.Document) (*goquery.Document, error) {
	type selRule struct {
		selector cascadia.Sel
		rule     []qrule
	}

	rules := map[int][]selRule{}

	for _, stylesheet := range c.stylesheet {
		for _, block := range stylesheet.blocks {
			selector := selectorString(block.componentValues)
			if selector == "" {
				continue
			}
			selectors, err := cascadia.ParseGroupWithPseudoElements(selector)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CSS selector %q: %w", selector, err)
			}
			for _, sel := range selectors {
				selSpecificity := sel.Specificity()
				s := selSpecificity[0]*100 + selSpecificity[1]*10 + selSpecificity[2]
				rules[s] = append(rules[s], selRule{selector: sel, rule: block.rules})
			}
		}
	}
	// sort map keys
	keys := make([]int, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	// now sorted by specificity
	sort.Ints(keys)
	c.computed = make(map[*html.Node][]declaration)
	root := doc.Get(0)
	for _, k := range keys {
		for _, r := range rules[k] {
			for _, singlerule := range r.rule {
				for _, node := range cascadia.QueryAll(root, r.selector) {
					var prefix string
					if pe := r.selector.PseudoElement(); pe != "" {
						prefix = pe + "::"
					}
					c.setDeclaration(node, prefix+stringValue(singlerule.key), singlerule.value)
				}
			}
		}
	}

	doc.Each(c.resolveStyleAttribute)
	return doc, nil
}

// setDeclaration records one cascaded declaration for a node. A property that
// is already present is dropped and re-appended, so the last writer both wins
// and ends up last in the order that shorthand expansion walks.
func (c *CSS) setDeclaration(n *html.Node, property string, value tokenstream) {
	decls := c.computed[n]
	for i, d := range decls {
		if d.property == property {
			decls = append(decls[:i], decls[i+1:]...)
			break
		}
	}
	c.computed[n] = append(decls, declaration{property: property, value: value})
}

// ComputedStyles returns the computed CSS declarations of a node, with every
// shorthand expanded into its longhands. Valid until the next ApplyCSS call;
// an element no rule matched yields an empty map.
func (c *CSS) ComputedStyles(n *html.Node) StyleMap {
	return resolveDeclarations(c.computed[n])
}

// PapersizeWidthHeight converts the spec to the width and height. The parameter
// can be a known paper size (such as A4 or letter) or a one or two parameter
// string such as 20cm 20cm. The return values are in one of the units 'mm' or
// 'in'.  'mm' if the  parameter spec is one of a5, a4, a3, b5, b4, jis-b5 or
// jis-b4 and 'in' if the parameter spec is one of letter, legal or ledger.
func PapersizeWidthHeight(spec string) (string, string) {
	spec = strings.ToLower(spec)
	var width, height string
	portrait := true
	for i, e := range strings.Fields(spec) {
		switch e {
		case "portrait":
			// good, nothing to do
		case "landscape":
			portrait = false
		case "a0":
			width = "841mm"
			height = "1189mm"
		case "a1":
			width = "594mm"
			height = "841mm"
		case "a2":
			width = "420mm"
			height = "594mm"
		case "a3":
			width = "297mm"
			height = "420mm"
		case "a4":
			width = "210mm"
			height = "297mm"
		case "a5":
			width = "148mm"
			height = "210mm"
		case "a6":
			width = "105mm"
			height = "148mm"
		case "a7":
			width = "74mm"
			height = "105mm"
		case "a8":
			width = "52mm"
			height = "74mm"
		case "b0":
			width = "1000mm"
			height = "1414mm"
		case "b1":
			width = "707mm"
			height = "1000mm"
		case "b2":
			width = "500mm"
			height = "707mm"
		case "b3":
			width = "353mm"
			height = "500mm"
		case "b4":
			width = "250mm"
			height = "353mm"
		case "b5":
			width = "176mm"
			height = "250mm"
		case "b6":
			width = "125mm"
			height = "176mm"
		case "b7":
			width = "88mm"
			height = "125mm"
		case "b8":
			width = "62mm"
			height = "88mm"
		case "jis-b5":
			width = "182mm"
			height = "257mm"
		case "jis-b4":
			width = "257mm"
			height = "364mm"
		case "letter":
			width = "8.5in"
			height = "11in"
		case "legal":
			width = "8.5in"
			height = "14in"
		case "ledger":
			width = "11in"
			height = "17in"
		default:
			if i == 0 {
				width = e
				height = e
			} else {
				height = e
			}
		}
	}

	if portrait {
		return width, height
	}
	return height, width
}
