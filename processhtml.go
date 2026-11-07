package htmlbag

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ProcessHTMLFile opens an HTML file, reads linked stylesheets, applies the CSS
// rules and returns the DOM structure.
func (c *CSS) ProcessHTMLFile(filename string) (*goquery.Document, error) {
	dir, fn := filepath.Split(filename)
	c.PushDir(dir)

	filename, err := c.findFile(fn)
	if err != nil {
		return nil, err
	}

	r, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	if err = c.readStylesheets(doc); err != nil {
		return nil, err
	}
	if err = c.addPendingCSS(); err != nil {
		return nil, err
	}
	_, err = c.ApplyCSS(doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// ProcessHTMLChunk reads the HTML text. If there are linked style sheets (<link
// href=...) these are also read. After reading, the CSS is applied to the HTML
// DOM which is returned.
func (c *CSS) ProcessHTMLChunk(htmltext string) (*goquery.Document, error) {
	var err error
	r := strings.NewReader(htmltext)
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	if err = c.readStylesheets(doc); err != nil {
		return nil, err
	}
	if err = c.addPendingCSS(); err != nil {
		return nil, err
	}
	_, err = c.ApplyCSS(doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// readStylesheets reads the stylesheets of doc in document order, as a browser
// does: the style elements and the link elements with rel="stylesheet", in
// the head and in the body. The HTML parser moves only those before the body
// into the head. A style or link element inside an SVG image belongs to the
// image.
func (c *CSS) readStylesheets(doc *goquery.Document) error {
	var err error
	doc.Find("style, link").EachWithBreak(func(i int, sel *goquery.Selection) bool {
		n := sel.Nodes[0]
		if n.Namespace != "" {
			return true
		}
		switch n.Data {
		case "style":
			err = c.AddCSSText(sel.Text())
		case "link":
			if href, ok := sel.Attr("href"); ok && isStylesheetLink(sel.AttrOr("rel", "")) {
				err = c.addLinkedCSS(href)
			}
		}
		return err == nil
	})
	return err
}

// isStylesheetLink reports whether the rel attribute of a link element loads a
// stylesheet. rel is a list of keywords in any case. An alternate stylesheet
// is not applied unless the reader picks it, which paper does not allow.
func isStylesheetLink(rel string) bool {
	stylesheet := false
	for _, kw := range strings.Fields(rel) {
		switch strings.ToLower(kw) {
		case "stylesheet":
			stylesheet = true
		case "alternate":
			return false
		}
	}
	return stylesheet
}

// addLinkedCSS reads the stylesheet file href and appends its rules, as
// AddCSSText does for the text of a style element.
func (c *CSS) addLinkedCSS(href string) error {
	toks, err := c.tokenizeCSSFile(href)
	if err != nil {
		return err
	}
	block := consumeBlock(toks, false)
	block.blocks = flattenNestedBlocks(block.blocks)
	if err = c.processAtRules(block); err != nil {
		return err
	}
	if err = validateSelectors(block); err != nil {
		return err
	}
	c.warnInvalidValues(block, false)
	c.stylesheet = append(c.stylesheet, block)
	return nil
}

// AddCSSText parses CSS text and appends the rules to the previously read
// rules. If the fragment contains relative links to other files (fonts or other
// stylesheets for example), the dir stack must be set in advance.
func (c *CSS) AddCSSText(fragment string) error {
	block, err := c.addCSSText(fragment)
	if err != nil {
		return err
	}
	c.warnInvalidValues(block, false)
	return nil
}

// addCSSText is AddCSSText without the warnings, for the default stylesheet:
// it declares the display of table parts, which an author's stylesheet cannot
// change.
func (c *CSS) addCSSText(fragment string) (sBlock, error) {
	toks, err := c.tokenizeAndApplyImport(fragment)
	if err != nil {
		return sBlock{}, err
	}
	block := consumeBlock(toks, false)
	// Flatten CSS nesting (e.g. .card { & > h2 { ... } } → .card > h2 { ... })
	block.blocks = flattenNestedBlocks(block.blocks)
	if err = c.processAtRules(block); err != nil {
		return sBlock{}, err
	}
	// Reject broken selectors here, so the error points at the stylesheet
	// and not at the first content that ApplyCSS runs on.
	if err = validateSelectors(block); err != nil {
		return sBlock{}, err
	}
	c.stylesheet = append(c.stylesheet, block)
	return block, nil
}
