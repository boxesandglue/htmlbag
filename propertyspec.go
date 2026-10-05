package htmlbag

// PropertyGroup names a section of the CSS property reference. The groups
// appear in the order of [PropertyGroups].
type PropertyGroup string

// The property groups. The first five hold the properties that apply to
// elements, the last three the descriptors of the at-rules.
const (
	GroupText      PropertyGroup = "Text"
	GroupBox       PropertyGroup = "Box model"
	GroupBorder    PropertyGroup = "Borders"
	GroupLayout    PropertyGroup = "Layout"
	GroupGenerated PropertyGroup = "Lists and generated content"
	GroupCustom    PropertyGroup = "Custom properties"
	GroupPage      PropertyGroup = "@page"
	GroupFontFace  PropertyGroup = "@font-face"
	GroupColor     PropertyGroup = "@-bag-color"
)

// PropertyGroups lists the groups in the order a reference should print them.
var PropertyGroups = []PropertyGroup{
	GroupText, GroupBox, GroupBorder, GroupLayout, GroupGenerated, GroupCustom,
	GroupPage, GroupFontFace, GroupColor,
}

// PropertySpec documents one CSS property (or at-rule descriptor) the package
// understands. Values and Note are Markdown, backticks mark literal keywords.
type PropertySpec struct {
	// Name is the property as written in a stylesheet. A group of longhands
	// that share one description is listed under one Name with the siblings in
	// Aliases.
	Name string
	// Aliases are further spellings that behave exactly like Name.
	Aliases []string
	// Values describes the accepted values.
	Values string
	// Example is one declaration, without the surrounding selector.
	Example string
	// Note names limits or deviations from the CSS specification. Empty when
	// the property behaves as CSS defines it.
	Note string
	// Group is the section of the reference.
	Group PropertyGroup
}

// Names returns the Name followed by the Aliases.
func (p PropertySpec) Names() []string {
	return append([]string{p.Name}, p.Aliases...)
}

// Properties is the list of CSS properties and at-rule descriptors the package
// reads. It is the single source for the user facing reference. The test
// TestPropertySpecCoverage checks it against the switch statements that consume
// the properties, so a new case in one of the switches needs an entry here.
var Properties = []PropertySpec{
	// Text
	{Name: "font-family", Values: "Font family name or a comma separated list. A list falls back per glyph, the first family that has the glyph renders it", Example: `font-family: "Minion Pro", serif;`, Group: GroupText},
	{Name: "font-size", Values: "Length, `em`, `%`, the keywords `xx-small` to `xxx-large`, `smaller` and `larger`", Example: "font-size: 12pt;", Group: GroupText},
	{Name: "font-weight", Values: "`normal`, `bold`, `bolder`, `lighter`, the names `thin` to `black`, or a number from 100 to 900", Example: "font-weight: 600;", Group: GroupText},
	{Name: "font-style", Values: "`normal`, `italic`", Example: "font-style: italic;", Note: "`oblique` is not recognized", Group: GroupText},
	{Name: "font-synthesis-style", Values: "`auto`, `none`", Example: "font-synthesis-style: auto;", Note: "`auto` slants a family's upright for an italic it lacks; the default is `none`, where the upright is used as it is", Group: GroupText},
	{Name: "font-synthesis", Values: "`none`, or any of `weight`, `style`, `small-caps`, `position`", Example: "font-synthesis: style;", Note: "Only `style` and `small-caps` are read, as `font-synthesis-style` and `font-synthesis-small-caps`", Group: GroupText},
	{Name: "font-synthesis-small-caps", Values: "`auto`, `none`", Example: "font-synthesis-small-caps: none;", Note: "`auto` (the default) sets small caps for a face without `smcp` as capitals at 70% of the size; `none` leaves the text as it is. Synthesized small caps are capitals in the PDF, so text extraction and search see `SMALL CAPS`, not `Small Caps`; a face with `smcp` keeps the original text", Group: GroupText},
	{Name: "font-variant-caps", Values: "`normal`, `small-caps`", Example: "font-variant-caps: small-caps;", Note: "Inherited. The face's `smcp` feature when it has one, else synthesized (see `font-synthesis-small-caps`). Other values are read as `normal`", Group: GroupText},
	{Name: "font-variant", Values: "`normal`, or values including `small-caps`", Example: "font-variant: small-caps;", Note: "Only `small-caps` is read, as `font-variant-caps`", Group: GroupText},
	{Name: "font", Values: "Shorthand: optional `font-style`, `small-caps` and `font-weight`, then the size with an optional `/line-height`, then the family", Example: "font: italic bold 10pt/12pt serif;", Note: "Style, caps, weight and line height reset to `normal` when omitted, as in CSS. Size and family are required", Group: GroupText},
	{Name: "font-feature-settings", Values: "Comma separated OpenType feature tags, each optionally followed by `on`, `off` or a number. `normal` removes all features", Example: `font-feature-settings: "smcp", "onum";`, Group: GroupText},
	{Name: "font-variation-settings", Values: "Comma separated pairs of an axis tag and a number, for variable fonts", Example: `font-variation-settings: "wght" 650;`, Group: GroupText},
	{Name: "color", Values: "Color value or a defined color name", Example: "color: #333;", Group: GroupText},
	{Name: "text-align", Values: "`left`, `right`, `center`, `justify`, `start`, `end`", Example: "text-align: justify;", Group: GroupText},
	{Name: "text-box", Values: "`normal`, or a `text-box-trim` and a `text-box-edge` value", Example: "text-box: trim-both text;", Note: "Shorthand; without a trim value it is `trim-both`", Group: GroupText},
	{Name: "text-box-edge", Values: "`auto`, `text`", Example: "text-box-edge: text;", Note: "The edges a block's lines are trimmed to: the block's font's text-over and text-under edges, its content area. Other edges warn and use `text`", Group: GroupText},
	{Name: "text-box-trim", Values: "`none`, `trim-start`, `trim-end`, `trim-both`", Example: "text-box-trim: trim-both;", Note: "Not inherited. Takes the space above the block's font's text-over edge off its first line and below its text-under edge off its last line, such as half-leading, so the block's box ends at its text. Lines set tighter than the content area make the block grow to its text. Split across pages, only the first fragment's start and the last fragment's end are trimmed", Group: GroupText},
	{Name: "text-indent", Values: "Length, indents the first line", Example: "text-indent: 1em;", Group: GroupText},
	{Name: "text-decoration", Values: "Shorthand for line, style and color", Example: "text-decoration: underline dotted red;", Group: GroupText},
	{Name: "text-decoration-line", Values: "`none`, `underline`, `overline`, `line-through`", Example: "text-decoration-line: underline;", Group: GroupText},
	{Name: "text-decoration-style", Values: "`solid`, `double`, `dotted`, `dashed`, `wavy`", Example: "text-decoration-style: wavy;", Group: GroupText},
	{Name: "text-decoration-color", Values: "Color value", Example: "text-decoration-color: red;", Group: GroupText},
	{Name: "line-height", Values: "`normal`, a number (factor of the font size) or a length", Example: "line-height: 1.4;", Group: GroupText},
	{Name: "letter-spacing", Values: "`normal` or a length", Example: "letter-spacing: 0.05em;", Group: GroupText},
	{Name: "white-space", Values: "`normal`, `nowrap`, `pre`, `pre-wrap`, `pre-line`", Example: "white-space: pre;", Group: GroupText},
	{Name: "hyphens", Values: "`auto`, `manual`, `none`", Example: "hyphens: none;", Note: "`manual` and `none` both switch the language patterns off, soft hyphens (U+00AD) stay break opportunities", Group: GroupText},
	{Name: "hanging-punctuation", Values: "`none`, `allow-end`", Example: "hanging-punctuation: allow-end;", Group: GroupText},
	{Name: "vertical-align", Values: "`baseline`, `sub`, `super`, `top`, `text-top`, `middle`, `bottom`, `text-bottom`, a length or a percentage of the line height", Example: "vertical-align: super;", Note: "Under a line model registered with `CSSBuilder.RegisterLineModel`, the shift of `sub`, `super`, a length or a percentage is passed to the model, which may grow the line with it", Group: GroupText},
	{Name: "direction", Values: "`ltr`, `rtl`", Example: "direction: rtl;", Group: GroupText},
	{Name: "unicode-bidi", Values: "`normal`, `embed`, `isolate`, `bidi-override`, `isolate-override`, `plaintext`", Example: "unicode-bidi: isolate;", Group: GroupText},
	{Name: "tab-size", Values: "Number of spaces or a length", Example: "tab-size: 4;", Group: GroupText},
	{Name: "initial-letter", Values: "Number of lines for a drop cap, or `normal`", Example: "initial-letter: 3;", Group: GroupText},
	{Name: "user-select", Values: "Any value", Example: "user-select: none;", Note: "Accepted and ignored, there is no selection in PDF", Group: GroupText},

	// Box model
	{Name: "margin", Values: "One to four lengths, shorthand for the four sides", Example: "margin: 10pt;", Group: GroupBox},
	{Name: "margin-top", Aliases: []string{"margin-right", "margin-bottom", "margin-left"}, Values: "Length", Example: "margin-top: 12pt;", Group: GroupBox},
	{Name: "padding", Values: "One to four lengths, shorthand for the four sides", Example: "padding: 5pt 10pt;", Group: GroupBox},
	{Name: "padding-top", Aliases: []string{"padding-right", "padding-bottom", "padding-left"}, Values: "Length", Example: "padding-left: 10pt;", Group: GroupBox},
	{Name: "padding-inline-start", Values: "Length", Example: "padding-inline-start: 2em;", Note: "Maps to `padding-left` for `ltr` and to `padding-right` for `rtl`", Group: GroupBox},
	{Name: "width", Values: "Length or percentage, on blocks, images and table cells", Example: "width: 100%;", Group: GroupBox},
	{Name: "height", Values: "Length, on blocks, images, table rows and table cells", Example: "height: 4cm;", Note: "A minimum: content taller than the height is never clipped, a table row grows to fit its cells", Group: GroupBox},
	{Name: "max-width", Values: "Length or percentage, on images", Example: "max-width: 100%;", Group: GroupBox},
	{Name: "background-color", Values: "Color value, painted on block elements, inline elements and table cells", Example: "background-color: #ffffcc;", Group: GroupBox},
	{Name: "background", Values: "Shorthand, only the color is read", Example: "background: #ffffcc;", Group: GroupBox},
	{Name: "display", Values: "`block`, `inline`, `none`", Example: "display: none;", Note: "Other values are ignored, the table parts follow from the HTML tags", Group: GroupBox},

	// Borders
	{Name: "border", Values: "Width, style and color in any order, all four sides", Example: "border: 1pt solid black;", Group: GroupBorder},
	{Name: "border-top", Aliases: []string{"border-right", "border-bottom", "border-left"}, Values: "Width, style and color for one side", Example: "border-top: 2pt solid red;", Group: GroupBorder},
	{Name: "border-width", Values: "One to four lengths or `thin`, `medium`, `thick`", Example: "border-width: 1pt 0;", Group: GroupBorder},
	{Name: "border-style", Values: "One to four border styles", Example: "border-style: solid none;", Group: GroupBorder},
	{Name: "border-color", Values: "One to four colors", Example: "border-color: red black;", Group: GroupBorder},
	{Name: "border-top-width", Aliases: []string{"border-right-width", "border-bottom-width", "border-left-width"}, Values: "Length", Example: "border-left-width: 3pt;", Group: GroupBorder},
	{Name: "border-top-style", Aliases: []string{"border-right-style", "border-bottom-style", "border-left-style"}, Values: "`none`, `solid`", Example: "border-left-style: solid;", Note: "Every other CSS border style is drawn as `solid`", Group: GroupBorder},
	{Name: "border-top-color", Aliases: []string{"border-right-color", "border-bottom-color", "border-left-color"}, Values: "Color value", Example: "border-left-color: red;", Group: GroupBorder},
	{Name: "border-radius", Values: "One length, the same for all four corners", Example: "border-radius: 3pt;", Note: "Several values, one per corner, are not supported", Group: GroupBorder},
	{Name: "border-top-left-radius", Aliases: []string{"border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"}, Values: "Length", Example: "border-top-left-radius: 3pt;", Group: GroupBorder},
	{Name: "border-collapse", Values: "`separate` (the default, as in CSS), `collapse`; on tables", Example: "border-collapse: collapse;", Group: GroupBorder},
	{Name: "border-spacing", Values: "One or two lengths, on tables in the separated model; the default is 2pt", Example: "border-spacing: 4pt 2pt;", Group: GroupBorder},

	// Layout
	{Name: "float", Values: "`left`, `right`, `none`; `inside` and `outside` pick the side towards or away from the binding on the page the float lands on, with the declared `margin-left`/`margin-right` swapped on left pages; `top` (or `before`) and `bottom` (or `after`) lift the element out of the flow to the top or bottom of the page", Example: "float: outside;", Group: GroupLayout},
	{Name: "clear", Values: "`left`, `right`, `inside`, `outside`, `both`, `none`", Example: "clear: both;", Group: GroupLayout},
	{Name: "position", Values: "`static`, `relative`, `absolute`, `running(name)` for running elements that repeat in page margin boxes", Example: "position: absolute;", Note: "`fixed` and `sticky` are not supported. A `relative` block is offset by `left` or `right` only; a `relative` inline element by `top` or `bottom` only, which moves its text after the lines are set", Group: GroupLayout},
	{Name: "top", Aliases: []string{"right", "bottom", "left"}, Values: "Length or `auto`, with `position`", Example: "top: 1cm;", Note: "On a `relative` inline element `top` wins over `bottom`, a percentage computes to `auto`, and `left` and `right` have no effect. The text and its background move; an underline stays where it was", Group: GroupLayout},
	{Name: "z-index", Values: "Integer or `auto`, with `position`", Example: "z-index: 1;", Group: GroupLayout},
	{Name: "page-break-before", Aliases: []string{"break-before"}, Values: "`auto`, `always`, `avoid`; `break-before` also takes `page`, `left`, `right`, `recto`, `verso`", Example: "page-break-before: always;", Note: "The page side variants break the page but do not pick a side. A forced break is taken at any depth; on a block's first child it applies to the block itself", Group: GroupLayout},
	{Name: "page-break-after", Aliases: []string{"break-after"}, Values: "Same values as `page-break-before`", Example: "page-break-after: avoid;", Note: "A forced break is taken at any depth; on a block's last child it applies to the block itself", Group: GroupLayout},
	{Name: "page-break-inside", Aliases: []string{"break-inside"}, Values: "`auto`, `avoid`", Example: "page-break-inside: avoid;", Note: "On a table row, `auto` lets the row break across pages; a row without the property stays whole, as with `avoid`", Group: GroupLayout},
	{Name: "widows", Values: "Positive integer, the default is 2; `1` switches the control off", Example: "widows: 3;", Note: "Counts the lines of a paragraph, a list item or `<pre>`, also inside a container; the blocks of a container are not counted", Group: GroupLayout},
	{Name: "orphans", Values: "Positive integer, the default is 2; `1` switches the control off", Example: "orphans: 3;", Note: "Counts as `widows` does", Group: GroupLayout},

	// Lists and generated content
	{Name: "list-style-type", Values: "`disc`, `circle`, `square`, `none`, `decimal`, `decimal-leading-zero`, `lower-alpha`, `upper-alpha`, `lower-latin`, `upper-latin`, `lower-roman`, `upper-roman`, `lower-greek`", Example: "list-style-type: lower-roman;", Group: GroupGenerated},
	{Name: "list-style", Values: "Shorthand for `list-style-type` and `list-style-position`", Example: "list-style: square;", Group: GroupGenerated},
	{Name: "list-style-position", Values: "`inside`, `outside`", Example: "list-style-position: outside;", Note: "Accepted for compatibility, markers are always placed outside", Group: GroupGenerated},
	{Name: "counter-reset", Values: "Counter name with an optional start value, repeated for several counters", Example: "counter-reset: section;", Group: GroupGenerated},
	{Name: "counter-increment", Values: "Counter name with an optional step, repeated for several counters", Example: "counter-increment: section;", Group: GroupGenerated},
	{Name: "content", Values: "On `::before`, `::after`, `::marker` and in page margin boxes: strings, `attr()`, `counter()`, `counters()`, `target-counter()`, `target-counters()`, `target-text()`, `element()`, `leader()`. The counter functions take a counter style as their last argument", Example: `content: counter(section, upper-roman) ". ";`, Group: GroupGenerated},

	// Custom properties
	{Name: "-bag-font-expansion", Values: "Percentage of allowed glyph stretching, `0%` turns it off", Example: "-bag-font-expansion: 0%;", Note: "Only justified text uses it; ragged text is set at its natural width", Group: GroupCustom},
	{Name: "-bag-horizontal-scale", Values: "A positive percentage or number, `90%` and `0.9` are the same, default `100%`", Example: "-bag-horizontal-scale: 90%;", Note: "Inherited. Draws the glyphs narrower or wider by a fixed factor (PDF horizontal scaling, `Tz`); the advances, kerns and spaces are scaled, so the line breaker sees the scaled width. The factor does not compound, `90%` inside `90%` stays 90%, so an element inside a scaled one returns to normal with `100%`. `letter-spacing` is not scaled, it stays an absolute length. Font expansion applies on top. Zero, negative or unparsable values are ignored with a warning", Group: GroupCustom},
	{Name: "-bag-italic-correction", Values: "`auto`, `none`", Example: "-bag-italic-correction: none;", Group: GroupCustom},
	{Name: "-bag-line-breaker", Values: "`auto` (Knuth-Plass, the default) or a name registered with `CSSBuilder.RegisterBreaker`", Example: "-bag-line-breaker: greedy;", Note: "Chooses where a paragraph breaks among its legal breakpoints; the lines are measured and set as with Knuth-Plass. Inherited. An unregistered name keeps Knuth-Plass and logs a warning once", Group: GroupCustom},
	{Name: "-bag-leading-model", Values: "`half` (CSS line boxes, the default), `trailing` (TeX style), or a name registered with `CSSBuilder.RegisterLineModel`", Example: "-bag-leading-model: trailing;", Note: "An unregistered name keeps the built-in leading and logs a warning once", Group: GroupCustom},
	{Name: "-bag-linebreak-tolerance", Values: "Number, the largest adjustment ratio a line may have, default 4", Example: "-bag-linebreak-tolerance: 8;", Note: "How far the spaces of a line may stretch, as a multiple of their stretchability. A ratio, not a badness as TeX's `\\tolerance`: TeX's 200 is about 1.26", Group: GroupCustom},
	{Name: "-bag-linebreak-hyphen-penalty", Values: "Number, the TeX hyphen penalty", Example: "-bag-linebreak-hyphen-penalty: 200;", Group: GroupCustom},
	{Name: "-bag-tab-stops", Values: "`none`, or a comma separated list of stops: a length or percentage of the line width, optionally `start`, `end`, `center`, `decimal` or `decimal(\",\")`, optionally `leader(\" . \")` (or `dotted`, `solid`, `space`)", Example: "-bag-tab-stops: 12mm, 100% end leader(dotted);", Note: "Inherited. A tab advances to the next stop past the text before it, `left` and `right` are synonyms of `start` and `end`. Tabs within a line survive any `white-space` mode, whitespace with a newline stays source formatting", Group: GroupCustom},
	{Name: "-bag-fixed-height", Values: "Length, or `none` (the default)", Example: "-bag-fixed-height: 16pt;", Note: "On table rows only, not inherited. The row is exactly this high and never breaks inside: content that does not fit draws past the row, background and borders keep the height. Wins over `height` on the same row, and a cell's `height` in a fixed row is ignored. On a cell the property has no effect", Group: GroupCustom},
	{Name: "-bag-float-gutter", Values: "Non-negative length, default `9pt`", Example: "-bag-float-gutter: 0;", Note: "Inherited. The space between a float and the text beside it where the float declares no positive margin on the side of the text; a declared margin replaces it. `margin: 0` cannot be told from no margin, so `-bag-float-gutter: 0` is how to get a browser's spacing", Group: GroupCustom},
	{Name: "-bag-trace", Values: "`boxmodel` or `none` (the default)", Example: "-bag-trace: boxmodel;", Note: "Not inherited. Paints the box model overlay (margin, border, padding and content) on the matched elements only, as the global box model trace does on every block. `none` turns it off again in a more specific rule; it cannot turn off the global trace", Group: GroupCustom},
	{Name: "-bag-bookmark", Values: "`none`, or a level number optionally followed by `open` or `closed`", Example: "-bag-bookmark: 2 closed;", Note: "Adds the element to the PDF outline", Group: GroupCustom},

	// @page descriptors
	{Name: "size", Values: "A paper name (`a0` to `a8`, `b0` to `b8`, `jis-b4`, `jis-b5`, `letter`, `legal`, `ledger`) with optional `portrait` or `landscape`, or width and height as two lengths", Example: "size: a4 landscape;", Group: GroupPage},
	{Name: "margin", Values: "One to four lengths, the page margins", Example: "margin: 2cm 1.5cm;", Group: GroupPage},
	{Name: "margin-top", Aliases: []string{"margin-right", "margin-bottom", "margin-left"}, Values: "Length, one page margin", Example: "margin-left: 3cm;", Group: GroupPage},
	{Name: "border", Aliases: []string{"border-top", "border-right", "border-bottom", "border-left", "border-width", "border-style", "border-color", "border-radius"}, Values: "As on elements, drawn around the page content area on every page", Example: "border-left: 4pt solid navy;", Group: GroupPage},
	{Name: "padding", Aliases: []string{"padding-top", "padding-right", "padding-bottom", "padding-left"}, Values: "Length, space between the page border and the content", Example: "padding: 5mm;", Group: GroupPage},
	{Name: "background-color", Values: "Color value, fills the sheet", Example: "background-color: #fafafa;", Note: "Painted on the first page only", Group: GroupPage},
	{Name: "background-image", Values: "`url()` of an image or PDF file, scaled to the full sheet on every page that uses this `@page` rule", Example: `background-image: url("letterhead.pdf");`, Group: GroupPage},
	{Name: "-bag-background-page", Values: "Page number of a multi page PDF used as `background-image`, the default is 1", Example: "-bag-background-page: 2;", Group: GroupPage},

	// @font-face descriptors
	{Name: "font-family", Values: "The name the font is used by", Example: `font-family: "Minion Pro";`, Group: GroupFontFace},
	{Name: "font-style", Values: "`normal`, `italic`", Example: "font-style: italic;", Group: GroupFontFace},
	{Name: "font-weight", Values: "A weight name or number, or two of them for the range a variable font covers", Example: "font-weight: 300 700;", Group: GroupFontFace},
	{Name: "src", Values: "Comma separated list of `url()` or `local()` sources, each optionally followed by `format()` and `tech()`", Example: `src: url("minion.otf");`, Group: GroupFontFace},
	{Name: "font-feature-settings", Values: "OpenType features switched on for this face, as on elements", Example: `font-feature-settings: "onum";`, Group: GroupFontFace},
	{Name: "font-variation-settings", Values: "Axis values of a variable font, as on elements", Example: `font-variation-settings: "wght" 450;`, Group: GroupFontFace},
	{Name: "size-adjust", Values: "Percentage that scales the glyphs of this face", Example: "size-adjust: 95%;", Group: GroupFontFace},
	{Name: "ascent-override", Values: "`normal` or a percentage of the em, in place of the face's ascent", Example: "ascent-override: 107.91%;", Note: "Read only by a line model registered with `CSSBuilder.RegisterLineModel` (see `-bag-leading-model`); the built-in `half` and `trailing` models do not use it", Group: GroupFontFace},
	{Name: "descent-override", Values: "`normal` or a percentage of the em, in place of the face's descent", Example: "descent-override: 25.1%;", Note: "Read only by a line model registered with `CSSBuilder.RegisterLineModel` (see `-bag-leading-model`); the built-in `half` and `trailing` models do not use it", Group: GroupFontFace},
	{Name: "line-gap-override", Values: "`normal` or a percentage of the em, in place of the face's line gap", Example: "line-gap-override: 0%;", Note: "Read only by a line model registered with `CSSBuilder.RegisterLineModel` (see `-bag-leading-model`); the built-in `half` and `trailing` models do not use it", Group: GroupFontFace},

	// @-bag-color descriptors
	{Name: "model", Values: "`cmyk`, `rgb`, `RGB`, `gray`, `GRAY`, `spotcolor`", Example: "model: cmyk;", Note: "Upper case `RGB` and `GRAY` take components from 0 to 255, lower case from 0 to 100", Group: GroupColor},
	{Name: "value", Values: "A CSS color such as `#ff0000` or `rgb(255, 0, 0)`, used instead of a model with components", Example: "value: #c00;", Group: GroupColor},
	{Name: "colorname", Values: "Name of the separation for a spot color", Example: "colorname: PANTONE 300 C;", Group: GroupColor},
	{Name: "c", Aliases: []string{"m", "y", "k", "r", "g", "b"}, Values: "Number, one component of the chosen model", Example: "c: 100;", Group: GroupColor},
}

// PropertiesInGroup returns the specs of one group in their declaration order.
func PropertiesInGroup(g PropertyGroup) []PropertySpec {
	var out []PropertySpec
	for _, p := range Properties {
		if p.Group == g {
			out = append(out, p)
		}
	}
	return out
}
