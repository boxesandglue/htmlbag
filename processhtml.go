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
	var errcond error
	doc.Find(":root > head link").Each(func(i int, sel *goquery.Selection) {
		if stylesheetfile, attExists := sel.Attr("href"); attExists {
			block, err := c.tokenizeCSSFile(stylesheetfile)
			if err != nil {
				errcond = err
			}
			parsedStyles := consumeBlock(block, false)
			parsedStyles.blocks = flattenNestedBlocks(parsedStyles.blocks)
			c.stylesheet = append(c.stylesheet, parsedStyles)
		}
	})
	if errcond != nil {
		return nil, errcond
	}
	doc.Find(":root > head style").Each(func(i int, sel *goquery.Selection) {
		if err := c.AddCSSText(sel.Text()); err != nil {
			errcond = err
		}
	})
	if errcond != nil {
		return nil, errcond
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
	var errcond error
	doc.Find(":root > head link").Each(func(i int, sel *goquery.Selection) {
		if stylesheetfile, attExists := sel.Attr("href"); attExists {
			block, err := c.tokenizeCSSFile(stylesheetfile)
			if err != nil {
				errcond = err
			}
			parsedStyles := consumeBlock(block, false)
			parsedStyles.blocks = flattenNestedBlocks(parsedStyles.blocks)
			if err = c.processAtRules(parsedStyles); err != nil {
				errcond = err
			}
			c.stylesheet = append(c.stylesheet, parsedStyles)
		}
	})
	if errcond != nil {
		return nil, errcond
	}
	doc.Find(":root > head style").Each(func(i int, sel *goquery.Selection) {
		if err := c.AddCSSText(sel.Text()); err != nil {
			errcond = err
		}
	})
	if errcond != nil {
		return nil, errcond
	}
	_, err = c.ApplyCSS(doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// AddCSSText parses CSS text and appends the rules to the previously read
// rules. If the fragment contains relative links to other files (fonts or other
// stylesheets for example), the dir stack must be set in advance.
func (c *CSS) AddCSSText(fragment string) error {
	toks, err := c.tokenizeAndApplyImport(fragment)
	if err != nil {
		return err
	}
	block := consumeBlock(toks, false)
	// Flatten CSS nesting (e.g. .card { & > h2 { ... } } → .card > h2 { ... })
	block.blocks = flattenNestedBlocks(block.blocks)
	if err = c.processAtRules(block); err != nil {
		return err
	}
	// Reject broken selectors here, so the error points at the stylesheet
	// and not at the first content that ApplyCSS runs on.
	if err = validateSelectors(block); err != nil {
		return err
	}
	c.stylesheet = append(c.stylesheet, block)
	return nil
}
