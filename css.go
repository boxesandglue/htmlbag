package htmlbag

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	scanner "github.com/speedata/css"
	"golang.org/x/net/html"
)

// tokenstream is a list of CSS tokens
type tokenstream []*scanner.Token

type qrule struct {
	key   tokenstream
	value tokenstream
}

// sBlock is a block with a selector
type sBlock struct {
	name            string      // only set if this is an at-rule
	componentValues tokenstream // the "selector"
	childAtRules    []*sBlock   // the block's at-rules, if any
	blocks          []*sBlock   // the at-rule's blocks, if any
	rules           []qrule     // the key-value pairs
}

// ContentTokenType distinguishes different kinds of CSS content tokens.
type ContentTokenType int

const (
	// ContentString is a literal string from a CSS content value.
	ContentString ContentTokenType = iota
	// ContentCounter is a counter() function call.
	ContentCounter
	// ContentCounters is a counters(name, separator) function call —
	// joins every ancestor counter of the same name with the separator,
	// giving hierarchical numbering like "2.1.1".
	ContentCounters
	// ContentLeader is a leader() function call (CSS GCPM).
	ContentLeader
	// ContentURL is a url() reference to an image or other resource.
	ContentURL
	// ContentTargetCounter is a target-counter(target, counter) function
	// call from CSS GCPM cross-references. Resolves the counter (typically
	// "page") at the referenced anchor's position.
	ContentTargetCounter
	// ContentTargetCounters is a target-counters(target, counter, separator)
	// function call. Joins the counter stack at the referenced anchor with
	// the separator.
	ContentTargetCounters
	// ContentTargetText is a target-text(target [, content-type]) function
	// call. Pulls textual content from the referenced anchor (e.g. the
	// heading title for a TOC entry).
	ContentTargetText
	// ContentAttr is an attr(name) function call at the top level of a
	// content value. Reads the named HTML attribute of the current element
	// and inserts its value as a string. Note that attr() also appears as
	// a *sub*-argument of the target-* functions (e.g. target-counter(
	// attr(href), page)); that nested use is recognised inside the
	// target-* branches and does not produce a ContentAttr token.
	ContentAttr
	// ContentElement is an element(name) function call (CSS GCPM running
	// elements). Valid only in @page margin boxes: it places the running
	// element that was removed from the normal flow via
	// `position: running(name)`. Value holds the running element name.
	// The optional keyword argument (first | start | last | first-except)
	// is not modelled; the first occurrence in the document is used.
	ContentElement
)

// ContentToken represents a single parsed piece of a CSS content property value.
type ContentToken struct {
	Type      ContentTokenType
	Value     string // string literal, counter name, or target-text content-type
	Separator string // counters() / target-counters() separator
	// Style is the counter style of counter(), counters() and the target-*
	// counters: "lower-roman", "upper-alpha", ... Empty means decimal.
	Style string
	// TargetID is the literal anchor id (with leading "#" stripped) for
	// target-* tokens, when the reference is url(#id) or "#id".
	TargetID string
	// TargetAttr is the attribute name for target-* tokens whose first
	// argument is attr(name) (typically attr(href)). The evaluator must
	// resolve this against the current element to obtain the actual id.
	TargetAttr string
}

// ParseContentValue tokenises a raw CSS content-property value string
// and returns the structured ContentToken slice. Convenience wrapper for
// callers that only have the value as text; the renderer reads the tokens
// the cascade already produced and calls parseContentTokens directly.
func ParseContentValue(value string) []ContentToken {
	return parseContentTokens(tokenizeCSSString(value))
}

// parseContentTokens converts a CSS tokenstream (the value side of a
// content property) into structured ContentToken values.
func parseContentTokens(ts tokenstream) []ContentToken {
	var tokens []ContentToken
	for i := 0; i < len(ts); i++ {
		tok := ts[i]
		switch tok.Type {
		case scanner.String:
			tokens = append(tokens, ContentToken{Type: ContentString, Value: tok.Value})
		case scanner.URI:
			tokens = append(tokens, ContentToken{Type: ContentURL, Value: tok.Value})
		case scanner.Function:
			if tok.Value == "counter" {
				// counter(name, style?): the name, then an optional
				// counter style, both idents.
				i++
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.Ident {
					name := ts[i].Value
					i++
					style, newI := parseCounterStyleArg(ts, i)
					i = newI
					tokens = append(tokens, ContentToken{Type: ContentCounter, Value: name, Style: style})
				}
				// skip until closing )
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "counters" {
				// counters(name, "sep", style?) — name first, then a string
				// separator, then an optional counter style
				i++
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				var name, sep, style string
				if i < len(ts) && ts[i].Type == scanner.Ident {
					name = ts[i].Value
					i++
				}
				// consume optional whitespace and comma
				for i < len(ts) && (ts[i].Type == scanner.S || (ts[i].Type == scanner.Delim && ts[i].Value == ",")) {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.String {
					sep = ts[i].Value
					i++
					style, i = parseCounterStyleArg(ts, i)
				}
				if name != "" {
					tokens = append(tokens, ContentToken{Type: ContentCounters, Value: name, Separator: sep, Style: style})
				}
				// skip until closing )
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "leader" {
				// next non-whitespace token should be the pattern string
				i++
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.String {
					tokens = append(tokens, ContentToken{Type: ContentLeader, Value: ts[i].Value})
				}
				// skip until closing )
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "target-counter" {
				// target-counter(target, counter)
				i++
				id, attr, newI := parseTargetReference(ts, i)
				i = newI
				// consume optional whitespace and comma
				for i < len(ts) && (ts[i].Type == scanner.S || (ts[i].Type == scanner.Delim && ts[i].Value == ",")) {
					i++
				}
				var counterName, style string
				if i < len(ts) && ts[i].Type == scanner.Ident {
					counterName = ts[i].Value
					i++
					style, i = parseCounterStyleArg(ts, i)
				}
				if counterName != "" && (id != "" || attr != "") {
					tokens = append(tokens, ContentToken{
						Type:       ContentTargetCounter,
						Value:      counterName,
						Style:      style,
						TargetID:   id,
						TargetAttr: attr,
					})
				}
				// skip until closing )
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "target-counters" {
				// target-counters(target, counter, separator)
				i++
				id, attr, newI := parseTargetReference(ts, i)
				i = newI
				for i < len(ts) && (ts[i].Type == scanner.S || (ts[i].Type == scanner.Delim && ts[i].Value == ",")) {
					i++
				}
				var counterName, sep, style string
				if i < len(ts) && ts[i].Type == scanner.Ident {
					counterName = ts[i].Value
					i++
				}
				for i < len(ts) && (ts[i].Type == scanner.S || (ts[i].Type == scanner.Delim && ts[i].Value == ",")) {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.String {
					sep = ts[i].Value
					i++
					style, i = parseCounterStyleArg(ts, i)
				}
				if counterName != "" && (id != "" || attr != "") {
					tokens = append(tokens, ContentToken{
						Type:       ContentTargetCounters,
						Value:      counterName,
						Separator:  sep,
						Style:      style,
						TargetID:   id,
						TargetAttr: attr,
					})
				}
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "target-text" {
				// target-text(target [, content-type])
				i++
				id, attr, newI := parseTargetReference(ts, i)
				i = newI
				for i < len(ts) && (ts[i].Type == scanner.S || (ts[i].Type == scanner.Delim && ts[i].Value == ",")) {
					i++
				}
				contentType := "content" // CSS GCPM default
				if i < len(ts) && ts[i].Type == scanner.Ident {
					contentType = ts[i].Value
				}
				if id != "" || attr != "" {
					tokens = append(tokens, ContentToken{
						Type:       ContentTargetText,
						Value:      contentType,
						TargetID:   id,
						TargetAttr: attr,
					})
				}
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "element" {
				// element(name [, first|start|last|first-except]):
				// CSS GCPM running element placement. Only the name is
				// consumed; the optional occurrence keyword is skipped.
				i++
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.Ident {
					tokens = append(tokens, ContentToken{Type: ContentElement, Value: ts[i].Value})
				}
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			} else if tok.Value == "attr" {
				// attr(name) — top-level CSS Values 4 attribute reference.
				// The fallback / type forms (`attr(href url)`, `attr(x px,
				// 0)`) are not modelled; only the bare ident is consumed
				// and the rest of the parenthesised tail is skipped.
				i++
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				var attrName string
				if i < len(ts) && ts[i].Type == scanner.Ident {
					attrName = ts[i].Value
					i++
				}
				if attrName != "" {
					tokens = append(tokens, ContentToken{Type: ContentAttr, Value: attrName})
				}
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
			}
		}
	}
	return tokens
}

// parseTargetReference parses the first argument of a target-* function:
// url(#id), "#id", or attr(name). Returns the explicit id (with leading "#"
// stripped) or the attribute name, plus the new token index positioned
// just past whatever was consumed. Returns empty strings if no recognised
// reference form was found.
// parseCounterStyleArg reads the optional trailing counter style argument of
// counter() and friends, ", lower-roman" say, starting at ts[i] which is the
// token after the previous argument. It returns the style name (empty when
// there is none) and the index of the first token it did not consume.
func parseCounterStyleArg(ts []*scanner.Token, i int) (string, int) {
	j := i
	for j < len(ts) && ts[j].Type == scanner.S {
		j++
	}
	if j >= len(ts) || ts[j].Type != scanner.Delim || ts[j].Value != "," {
		return "", i
	}
	j++
	for j < len(ts) && ts[j].Type == scanner.S {
		j++
	}
	if j < len(ts) && ts[j].Type == scanner.Ident {
		return ts[j].Value, j + 1
	}
	return "", i
}

func parseTargetReference(ts tokenstream, i int) (id, attr string, newIdx int) {
	for i < len(ts) && ts[i].Type == scanner.S {
		i++
	}
	if i >= len(ts) {
		return "", "", i
	}
	switch ts[i].Type {
	case scanner.URI:
		id = strings.TrimPrefix(ts[i].Value, "#")
		i++
	case scanner.String:
		id = strings.TrimPrefix(ts[i].Value, "#")
		i++
	case scanner.Function:
		if ts[i].Value == "attr" {
			i++
			for i < len(ts) && ts[i].Type == scanner.S {
				i++
			}
			if i < len(ts) && ts[i].Type == scanner.Ident {
				attr = ts[i].Value
				i++
			}
			for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
				i++
			}
			if i < len(ts) {
				i++
			}
		}
	case scanner.Ident:
		// Round-trip recovery: "attr" as a bare Ident followed by "(" is
		// the same as the Function token.
		if ts[i].Value == "attr" {
			j := i + 1
			for j < len(ts) && ts[j].Type == scanner.S {
				j++
			}
			if j < len(ts) && ts[j].Type == scanner.Delim && ts[j].Value == "(" {
				i = j + 1
				for i < len(ts) && ts[i].Type == scanner.S {
					i++
				}
				if i < len(ts) && ts[i].Type == scanner.Ident {
					attr = ts[i].Value
					i++
				}
				for i < len(ts) && !(ts[i].Type == scanner.Delim && ts[i].Value == ")") {
					i++
				}
				if i < len(ts) {
					i++
				}
			}
		}
	}
	return id, attr, i
}

// Page defines a page.
type Page struct {
	PageArea        map[string]StyleMap       // computed styles per page area
	PageAreaContent map[string][]ContentToken // parsed content tokens per area
	// Attributes holds the page's own declarations in cascade order, ready
	// for resolveDeclarations. Kept unresolved so a later stylesheet can add
	// to the list before the shorthands are expanded.
	Attributes    []declaration
	Papersize     string
	MarginLeft    string
	MarginRight   string
	MarginTop     string
	MarginBottom  string
	pageareaRules map[string][]qrule
}

// NamedPage returns the @page rule for the given page name, with the
// generic @page rule folded in as the base (CSS Paged Media 3 §3.2, the
// same cascade getPageType applies to :first/:left/:right). An empty name
// or an unknown name yields the generic @page rule alone. The boolean is
// false when neither rule exists.
//
// This is the entry point for callers that select pages themselves, such
// as xts: a master page named "default" picks up `@page default { }`.
func (c *CSS) NamedPage(name string) (*Page, bool) {
	base, hasBase := c.Pages[""]
	if name != "" {
		if named, ok := c.Pages[name]; ok {
			if !hasBase {
				return &named, true
			}
			merged := mergePageWithBase(named, base)
			return &merged, true
		}
	}
	if hasBase {
		return &base, true
	}
	return nil, false
}

// CSS is the main structure that contains cascading style sheet information.
// Multiple stylesheets can be added to the CSS structure and then applied to
// HTML.
type CSS struct {
	Pages      map[string]Page
	FileFinder func(string) (string, error)
	FontFaces  []FontFace
	// Colors holds the named colors from @-bag-color rules, in source order.
	Colors     []ColorDefinition
	dirstack   []string
	stylesheet []sBlock
	// computed holds the cascade result per element, filled by ApplyCSS and
	// read by the renderer. The declarations stay in cascade order because
	// shorthand expansion depends on it.
	computed map[*html.Node][]declaration
}

// PushDir adds a directory to the dir stack. When a file is opened, all new
// Open calls are relative to this directory. ProcessHTMLFile uses the dir stack
// internally when it reads a cascade of CSS files.
func (c *CSS) PushDir(dir string) {
	if filepath.IsAbs(dir) {
		c.dirstack = append(c.dirstack, dir)
		return
	}
	var newEntry string
	if len(c.dirstack) > 0 {
		lastEntry := c.dirstack[len(c.dirstack)-1]
		newEntry = filepath.Join(lastEntry, dir)
	} else {
		newEntry = dir
	}
	c.dirstack = append(c.dirstack, newEntry)
}

// PopDir removes the last entry from the dir stack.
func (c *CSS) PopDir() {
	c.dirstack = c.dirstack[:len(c.dirstack)-1]
}

// FindFile resolves a CSS-referenced path to an absolute location using the
// same rules as internal stylesheet loading: the CSS.FileFinder callback if
// set, otherwise the top of the dirstack (populated via PushDir). Callers
// outside the package that resolve url() paths (e.g. htmlbag page
// backgrounds) should use this rather than reading FileFinder directly, so
// the dirstack-based glu/markdown route resolves too.
func (c *CSS) FindFile(filename string) (string, error) {
	return c.findFile(filename)
}

// findFile returns the absolute path of the file. If the function in
// CSS.FileFinder is set, it is used to find the file. If it is unset, findFile
// returns the filename if is an absolute path or it prefixes the filename with
// the top entry of the dirstack.
func (c *CSS) findFile(filename string) (string, error) {
	if c.FileFinder != nil {
		if loc, err := c.FileFinder(filename); loc != "" && err == nil {
			return loc, nil
		}
	}
	if len(c.dirstack) == 0 {
		return filename, nil
	}
	lastEntry := c.dirstack[len(c.dirstack)-1]
	if filepath.IsAbs(filename) {
		return filename, nil
	}
	return filepath.Join(lastEntry, filename), nil
}

// resolveURITokens rewrites relative url() token values to absolute paths
// while the declaring stylesheet's directory is still on the dirstack
// (CSS Values and Units §4.5: relative URLs resolve against the stylesheet).
// Fragment references (#anchor, used by target-counter), data: URIs and
// scheme URLs (http://, https://) are left alone.
func (c *CSS) resolveURITokens(ts tokenstream) {
	for _, tok := range ts {
		if tok.Type != scanner.URI {
			continue
		}
		if strings.HasPrefix(tok.Value, "#") || strings.HasPrefix(tok.Value, "data:") || strings.Contains(tok.Value, "://") {
			continue
		}
		if resolved, err := c.findFile(tok.Value); err == nil && resolved != "" {
			tok.Value = resolved
		}
	}
}

// CSSdefaults contains browser-like styling of some elements.
var CSSdefaults = `
html            { font-size: 10pt; tab-size: 4; font-family: serif; }
li              { display: list-item; padding-left: 0; }
head            { display: none }
table           { display: table }
tr              { display: table-row }
thead           { display: table-header-group }
tbody           { display: table-row-group }
tfoot           { display: table-footer-group }
td, th          { display: table-cell }
caption         { display: table-caption }
th              { font-weight: bold; text-align: center }
caption         { text-align: center }
body            { margin: 0pt; line-height: 1.2; hyphens: auto; font-weight: normal; text-align: start; -bag-leading-model: half; }
p               { font-size: 1em; margin: 1.5em 0 }
h1              { font-size: 2em; margin:  .67em 0 }
h2              { font-size: 1.5em; margin: .75em 0 }
h3              { font-size: 1.17em; margin: .83em 0 }
h4,
blockquote, ul,
fieldset, form,
ol, dl, dir,
h5              { font-size: 1em; margin: 1.5em 0; text-align: start; }
h6              { font-size: .75em; margin: 1.67em 0 }
h1, h2, h3, h4,
h5, h6, b,
strong          { font-weight: bold }
blockquote      { margin-left: 40px; margin-right: 40px }
i, cite, em,
var, address    { font-style: italic }
pre, tt, code,
kbd, samp       { font-family: monospace; -bag-font-expansion: 0%;}
pre             { white-space: pre; margin: 1em 0px; }
button, textarea,
input, select   { display: inline-block }
big             { font-size: 1.17em }
small, sub, sup { font-size: .83em }
sub             { vertical-align: sub }
sup             { vertical-align: super }
table           { border-spacing: 2pt; }
thead, tbody,
tfoot           { vertical-align: middle }
td, th, tr      { vertical-align: inherit }
s, strike, del  { text-decoration: line-through }
hr              { border: 1px inset }
ol, ul, dir, dd { padding-left: 20pt }
ol              { list-style-type: decimal }
ul              { list-style-type: disc }
ol ul, ul ol,
ul ul, ol ol    { margin-top: 0; margin-bottom: 0 }
u, ins          { text-decoration: underline }
center          { text-align: center }
`

// :link           { text-decoration: underline }

// Return the position of the matching closing brace "}"
func findClosingBrace(toks tokenstream) int {
	level := 1
	for i, t := range toks {
		if t.Type == scanner.Delim {
			switch t.Value {
			case "{":
				level++
			case "}":
				level--
				if level == 0 {
					return i + 1
				}
			}
		}
	}
	return len(toks)
}

// fixupComponentValues changes DELIM[.] + IDENT[foo] to IDENT[.foo]
func fixupComponentValues(toks tokenstream) tokenstream {
	toks = trimSpace(toks)
	var combineNext bool
	for i := 0; i < len(toks)-1; i++ {
		combineNext = false
		if toks[i].Type == scanner.Delim && toks[i].Value == "." && toks[i+1].Type == scanner.Ident {
			toks[i+1].Value = "." + toks[i+1].Value
			combineNext = true
		} else if toks[i].Type == scanner.Delim && toks[i].Value == ":" && toks[i+1].Type == scanner.Ident {
			toks[i+1].Value = ":" + toks[i+1].Value
			combineNext = true
		} else if toks[i].Type == scanner.Delim && toks[i].Value == ":" && toks[i+1].Type == scanner.Function {
			toks[i+1].Value = ":" + toks[i+1].Value
			combineNext = true
		} else if toks[i].Type == scanner.Hash {
			toks[i].Value = "#" + toks[i].Value
		}

		if combineNext {
			toks = append(toks[:i], toks[i+1:]...)
			i++
		}
	}
	return toks
}

// stripImportant removes a trailing CSS `!important` marker (DELIM "!"
// followed by IDENT "important", optionally separated by whitespace) from
// a value token stream. The cascade does not yet honour the priority boost
// from the cascade, but the value itself must still be applied — without
// this strip, the bare DELIM "!" reaches stringValue and produces an
// "unhandled delimiter" warning, while "important" leaks into the
// stringified value (e.g. "center important") and breaks the property.
func stripImportant(toks tokenstream) tokenstream {
	out := make(tokenstream, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Type == scanner.Delim && t.Value == "!" {
			j := i + 1
			for j < len(toks) && toks[j].Type == scanner.S {
				j++
			}
			if j < len(toks) && toks[j].Type == scanner.Ident && strings.EqualFold(toks[j].Value, "important") {
				i = j
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func trimSpace(toks tokenstream) tokenstream {
	i := 0
	for {
		if i == len(toks) {
			break
		}
		if t := toks[i]; t.Type == scanner.S {
			i++
		} else {
			break
		}
	}
	toks = toks[i:]
	return toks
}

// consumeBlock get the contents of a block. The name (in case of an at-rule)
// and the selector will be added later on
func consumeBlock(toks tokenstream, inblock bool) sBlock {
	// This is the whole block between the opening { and closing }
	if len(toks) <= 1 {
		return sBlock{}
	}
	b := sBlock{}
	i := 0
	// we might start with whitespace, skip it
	for {
		if i == len(toks) {
			break
		}
		if t := toks[i]; t.Type == scanner.S {
			i++
		} else {
			break
		}
	}
	start := i
	colon := 0

outer:
	for {
		if i == len(toks) {
			break
		}
		// There are only two cases: a key-value rule or something with
		// curly braces
		if t := toks[i]; t.Type == scanner.Delim {
			switch t.Value {
			case ":":
				if inblock {
					colon = i
				}
			case ";":
				if colon == 0 || colon < start {
					// No colon found before semicolon — this is not a
					// valid key:value rule (e.g. a stray "import url(...);"
					// without the leading @). Skip it.
					start = i + 1
					if start < len(toks) && toks[start].Type == scanner.S {
						start++
					}
					if start == len(toks) {
						break outer
					}
					i++
					continue
				}
				key := trimSpace(toks[start:colon])
				value := stripImportant(trimSpace(toks[colon+1 : i]))
				q := qrule{key: key, value: value}
				b.rules = append(b.rules, q)
				colon = 0
				start = i + 1
				if start < len(toks) && toks[start].Type == scanner.S {
					start++
				}
				if start == len(toks) {
					break outer
				}
			case "{":
				var nb sBlock
				// l is the length of the sub block
				l := findClosingBrace(toks[i+1:])
				if l == 1 {
					break
				}
				subblock := toks[i+1 : i+l]
				// subblock is without the enclosing curly braces
				starttok := toks[start]
				startsWithATKeyword := starttok.Type == scanner.AtKeyword && (starttok.Value == "media" || starttok.Value == "supports")
				nb = consumeBlock(subblock, !startsWithATKeyword)
				if toks[start].Type == scanner.AtKeyword {
					nb.name = toks[start].Value
					b.childAtRules = append(b.childAtRules, &nb)
					nb.componentValues = fixupComponentValues(toks[start+1 : i])
				} else {
					b.blocks = append(b.blocks, &nb)
					nb.componentValues = fixupComponentValues(toks[start:i])
				}

				i = i + l
				start = i + 1
				colon = 0
				// skip over whitespace
				if start < len(toks) && toks[start].Type == scanner.S {
					start++
					i++
				}
			case ",", ")", ".":
				// ignore
			default:
				// w("unknown delimiter", t.Value)
			}
		}
		i++
		if i == len(toks) {
			break
		}
	}
	if colon > 0 {
		b.rules = append(b.rules, qrule{key: toks[start:colon], value: stripImportant(toks[colon+1:])})
	}
	return b
}

// FontSource has information from the src attribute.
type FontSource struct {
	Local  string
	URI    string
	Format string
	Tech   string
}

// FontFace contains information from a @font-face rule.
type FontFace struct {
	Weight int
	// WeightMax is the upper end of a CSS Fonts 4 weight range
	// (`font-weight: 200 900`, typically a variable font). For a single
	// weight value WeightMax equals Weight.
	WeightMax         int
	Style             string
	Family            string
	Source            []FontSource
	Features          []string
	VariationSettings map[string]float64 // axis tag -> value (e.g., "wght" -> 700)
	SizeAdjust        float64
}

// parseFontWeightValue parses one font-weight value: a number or one of
// the CSS weight keywords. Reports false for anything else.
func parseFontWeightValue(value string) (int, bool) {
	if i, err := strconv.Atoi(value); err == nil {
		return i, true
	}
	switch strings.ToLower(value) {
	case "thin", "hairline":
		return 100, true
	case "extra light", "ultra light":
		return 200, true
	case "light":
		return 300, true
	case "normal":
		return 400, true
	case "medium":
		return 500, true
	case "semi bold", "demi bold":
		return 600, true
	case "bold":
		return 700, true
	case "extra bold", "ultra bold":
		return 800, true
	case "black", "heavy":
		return 900, true
	}
	return 0, false
}

func (c *CSS) doFontFace(ff []qrule) error {
	f := FontFace{
		Weight:    400,
		WeightMax: 400,
	}
	// var fontweight frontend.FontWeight = 400
	// var fontstyle frontend.FontStyle = frontend.FontStyleNormal
	// var fontfamily string
	for _, rule := range ff {
		key := strings.TrimSpace(rule.key.String())
		value := strings.TrimSpace(stringValue(rule.value))
		switch key {
		case "font-family":
			f.Family = strings.Trim(value, `"`)
		case "font-style":
			f.Style = value
		case "font-weight":
			// A single value ("500", "bold", also two-word keywords like
			// "extra light") or a CSS Fonts 4 range for variable fonts
			// ("200 900"). Try the whole value first so the spaced
			// keywords keep working, then the two-value range form.
			if w, ok := parseFontWeightValue(value); ok {
				f.Weight = w
				f.WeightMax = w
			} else if fields := strings.Fields(value); len(fields) == 2 {
				w1, ok1 := parseFontWeightValue(fields[0])
				w2, ok2 := parseFontWeightValue(fields[1])
				if ok1 && ok2 {
					f.Weight = w1
					f.WeightMax = w2
				}
			}
		case "src":
			src := FontSource{}
			for _, v := range rule.value {
				switch v.Type {
				case scanner.Local:
					src.Local = v.Value
				case scanner.URI:
					if resolved, err := c.findFile(v.Value); err == nil {
						src.URI = resolved
					} else {
						src.URI = v.Value
					}
				case scanner.Format:
					src.Format = v.Value
				case scanner.Tech:
					src.Tech = v.Value
				case scanner.Delim:
					if v.Value == "," {
						f.Source = append(f.Source, src)
					}
				case scanner.S:
					// ignore
				default:
					return fmt.Errorf("css src(): unhandled token %T", v)
				}
			}
			f.Source = append(f.Source, src)
		case "font-feature-settings":
			settingOn := true
			r := regexp.MustCompile(`(on|off|\d+)\s*$`)
			if r.MatchString(value) {
				idx := r.FindAllStringIndex(value, -1)
				if idx != nil {
					sw := value[idx[0][0]:idx[0][1]]
					if sw == "on" {
						// keep on
					} else if sw == "off" || sw == "0" {
						settingOn = false
					} else if sw >= "1" && sw <= "9" {
						// keep on
					}
				}
				value = value[:idx[0][0]]
			}
			var prefix string
			for _, v := range strings.Split(value, ",") {
				if settingOn {
					prefix = "+"
				} else {
					prefix = "-"
				}
				f.Features = append(f.Features, prefix+strings.TrimSpace(v))
			}
		case "font-variation-settings":
			// Parse CSS syntax: "wght" 700, "wdth" 100
			if f.VariationSettings == nil {
				f.VariationSettings = make(map[string]float64)
			}
			for _, pair := range strings.Split(value, ",") {
				pair = strings.TrimSpace(pair)
				parts := strings.Fields(pair)
				if len(parts) >= 2 {
					// Remove quotes from axis tag
					tag := strings.Trim(parts[0], `"'`)
					if val, err := strconv.ParseFloat(parts[1], 64); err == nil {
						f.VariationSettings[tag] = val
					}
				}
			}
		case "size-adjust":
			v := strings.TrimSuffix(value, "%")
			flt, err := strconv.ParseFloat(v, 64)
			if err != nil {
				panic(err)
			}
			f.SizeAdjust = 1 - (flt / 100)
		default:
			fmt.Println("unhandled font setting", key)
		}
	}
	c.FontFaces = append(c.FontFaces, f)
	return nil
}

func (c *CSS) doPage(block *sBlock) {
	selector := strings.Trim(block.componentValues.String(), " ")
	pg := c.Pages[selector]
	if pg.pageareaRules == nil {
		pg.pageareaRules = make(map[string][]qrule)
	}
	for _, v := range block.rules {
		switch v.key.String() {
		case "size":
			pg.Papersize = v.value.String()
		case "margin":
			fv := fourValues(v.value)
			pg.MarginTop = fv["top"].String()
			pg.MarginBottom = fv["bottom"].String()
			pg.MarginLeft = fv["left"].String()
			pg.MarginRight = fv["right"].String()
		// The margin longhands are valid page-context properties (CSS Paged
		// Media 3 §7.2). Without these cases they fell into the generic
		// attribute list, where nothing consumes them for the page geometry —
		// a silent no-op. Declaration order decides, so a longhand after the
		// shorthand overrides that side (normal cascade within one rule).
		case "margin-top":
			pg.MarginTop = strings.TrimSpace(v.value.String())
		case "margin-bottom":
			pg.MarginBottom = strings.TrimSpace(v.value.String())
		case "margin-left":
			pg.MarginLeft = strings.TrimSpace(v.value.String())
		case "margin-right":
			pg.MarginRight = strings.TrimSpace(v.value.String())
		default:
			// Resolve url() while the declaring stylesheet's directory is
			// still on the dirstack; downstream consumers run after PopDir
			// and would resolve against the document instead (issue #3).
			c.resolveURITokens(v.value)
			pg.Attributes = append(pg.Attributes, declaration{property: v.key.String(), value: v.value})
		}
	}
	for _, rule := range block.childAtRules {
		for _, r := range rule.rules {
			c.resolveURITokens(r.value)
		}
		pg.pageareaRules[rule.name] = rule.rules
	}
	if pg.PageArea == nil {
		pg.PageArea = make(map[string]StyleMap)
	}
	for k, v := range pg.pageareaRules {
		decls := make([]declaration, 0, len(v))
		for _, r := range v {
			decls = append(decls, declaration{property: r.key.String(), value: r.value})
		}
		pg.PageArea[strings.TrimPrefix(k, "@")] = resolveDeclarations(decls)
	}

	if pg.PageAreaContent == nil {
		pg.PageAreaContent = make(map[string][]ContentToken)
	}
	for areaName, rules := range pg.pageareaRules {
		for _, r := range rules {
			if r.key.String() == "content" {
				pg.PageAreaContent[strings.TrimPrefix(areaName, "@")] = parseContentTokens(r.value)
				break
			}
		}
	}

	c.Pages[selector] = pg
}

func (c *CSS) processAtRules(stylesheet sBlock) error {
	if c.Pages == nil {
		c.Pages = make(map[string]Page)
	}
	for _, atrule := range stylesheet.childAtRules {
		switch atrule.name {
		case "font-face":
			if err := c.doFontFace(atrule.rules); err != nil {
				return err
			}
		case "page":
			c.doPage(atrule)
		case "-bag-color":
			if err := c.doColor(atrule); err != nil {
				return err
			}
		default:
			fmt.Println("unknown at rule", atrule)
		}
	}
	return nil
}

// NewCSSParser returns a new CSS object
func NewCSSParser() *CSS {
	return &CSS{}
}

// NewCSSParserWithDefaults returns a new CSS object with the default stylesheet
// included. This is a convenience function which adds the CSSdefaults to the
// returned CSS struct.
func NewCSSParserWithDefaults() *CSS {
	c := &CSS{}
	c.AddCSSText(CSSdefaults)
	return c
}
