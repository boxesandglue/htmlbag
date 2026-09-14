// Package htmlbag turns HTML and CSS into PDF pages.
//
// The package covers the whole chain: it parses CSS from files or strings,
// matches the rules against an HTML document to build a DOM with computed
// style attributes at the selected nodes, and typesets the result with
// boxes and glue.
//
// The CSS side is driven by [CSS], created with [NewCSSParser] or
// [NewCSSParserWithDefaults], and consumed through [CSS.ProcessHTMLFile] or
// [CSS.ProcessHTMLChunk]. The typesetting side is driven by [CSSBuilder],
// created with [New].
package htmlbag
