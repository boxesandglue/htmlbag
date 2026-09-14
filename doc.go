// Package htmlbag turns HTML and CSS into PDF pages.
//
// The package covers the whole chain: it parses CSS from files or strings,
// matches the rules against an HTML document to compute the style of every
// selected node, and typesets the result with boxes and glue.
//
// The CSS side is driven by [CSS], created with [NewCSSParser] or
// [NewCSSParserWithDefaults], and consumed through [CSS.ProcessHTMLFile] or
// [CSS.ProcessHTMLChunk]. Both apply the cascade; [CSS.ComputedStyles] reads
// the result for a node as a [StyleMap], with shorthands expanded and values
// still in their parsed form. The typesetting side is driven by [CSSBuilder],
// created with [New].
package htmlbag
