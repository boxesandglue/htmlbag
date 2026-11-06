package htmlbag

// PropertyGroup names a section of the CSS property reference. The groups
// appear in the order of [PropertyGroups].
type PropertyGroup string

// The property groups. The first six hold the properties that apply to
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

// elementGroups are the groups of the properties that apply to elements.
var elementGroups = []PropertyGroup{GroupText, GroupBox, GroupBorder, GroupLayout, GroupGenerated, GroupCustom}

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
	{Name: "font-variant-numeric", Values: "`normal`, or any of `lining-nums` or `oldstyle-nums`, `proportional-nums` or `tabular-nums`, `diagonal-fractions` or `stacked-fractions`, `ordinal`, `slashed-zero`", Example: "font-variant-numeric: oldstyle-nums tabular-nums;", Note: "Inherited. Switches on the OpenType features `lnum`, `onum`, `pnum`, `tnum`, `frac`, `afrc`, `ordn` and `zero`, so it needs a face that has them, and nothing is synthesized. `font-feature-settings` wins over it", Group: GroupText},
	{Name: "font-variant", Values: "`normal`, or values including `small-caps` and the keywords of `font-variant-numeric`", Example: "font-variant: small-caps oldstyle-nums;", Note: "Only these are read, as `font-variant-caps` and `font-variant-numeric`. Other parts are ignored", Group: GroupText},
	{Name: "font", Values: "Shorthand: optional `font-style`, `small-caps` and `font-weight`, then the size with an optional `/line-height`, then the family", Example: "font: italic bold 10pt/12pt serif;", Note: "Style, caps, numeric figures, weight and line height reset to `normal` when omitted, as in CSS. Size and family are required", Group: GroupText},
	{Name: "font-feature-settings", Values: "Comma separated OpenType feature tags, each optionally followed by `on`, `off` or a number. `normal` removes all features", Example: `font-feature-settings: "smcp", "onum";`, Group: GroupText},
	{Name: "font-variation-settings", Values: "Comma separated pairs of an axis tag and a number, for variable fonts", Example: `font-variation-settings: "wght" 650;`, Group: GroupText},
	{Name: "color", Values: "Color value or a defined color name", Example: "color: #333;", Group: GroupText},
	{Name: "text-align", Values: "`left`, `right`, `center`, `justify`, `justify-all`, `start`, `end`", Example: "text-align: justify;", Note: "`justify-all` also sets `text-align-last` to `justify`. Other values leave `text-align-last` alone, as in browsers", Group: GroupText},
	{Name: "text-align-last", Values: "`auto`, `left`, `right`, `center`, `justify`, `start`, `end`", Example: "text-align-last: center;", Note: "Inherited. Aligns the last line of a paragraph and a line before a `<br>`. `auto` follows `text-align`, a justified paragraph ends at the start. Line breaks do not change. With `justify`, a line without a space stays at the start", Group: GroupText},
	{Name: "text-box", Values: "`normal`, or a `text-box-trim` and a `text-box-edge` value", Example: "text-box: trim-both text;", Note: "Shorthand; without a trim value it is `trim-both`", Group: GroupText},
	{Name: "text-box-edge", Values: "`auto`, `text`", Example: "text-box-edge: text;", Note: "The edges a block's lines are trimmed to: the block's font's text-over and text-under edges, its content area. Other edges warn and use `text`", Group: GroupText},
	{Name: "text-box-trim", Values: "`none`, `trim-start`, `trim-end`, `trim-both`", Example: "text-box-trim: trim-both;", Note: "Not inherited. Takes the space above the block's font's text-over edge off its first line and below its text-under edge off its last line, such as half-leading, so the block's box ends at its text. Lines set tighter than the content area make the block grow to its text. Split across pages, only the first fragment's start and the last fragment's end are trimmed under `box-decoration-break: slice`; under `clone` every fragment of a paragraph is, so the last line before a break may fit by its text. On a container of other blocks, the first and last line are those of its first and last block, through nested containers, unless padding or a border lies between. A table cell is such a container too, and a table in a container holds none of its lines", Group: GroupText},
	{Name: "text-indent", Values: "Length, indents the first line", Example: "text-indent: 1em;", Group: GroupText},
	{Name: "text-decoration", Values: "Shorthand for line, style and color", Example: "text-decoration: underline dotted red;", Group: GroupText},
	{Name: "text-decoration-line", Values: "`none`, `underline`, `overline`, `line-through`", Example: "text-decoration-line: underline;", Group: GroupText},
	{Name: "text-decoration-style", Values: "`solid`, `double`, `dotted`, `dashed`, `wavy`", Example: "text-decoration-style: wavy;", Group: GroupText},
	{Name: "text-decoration-color", Values: "Color value", Example: "text-decoration-color: red;", Group: GroupText},
	{Name: "line-height", Values: "`normal`, a number (factor of the font size) or a length", Example: "line-height: 1.4;", Group: GroupText},
	{Name: "letter-spacing", Values: "`normal` or a length", Example: "letter-spacing: 0.05em;", Group: GroupText},
	{Name: "white-space", Values: "`normal`, `nowrap`, `pre`, `pre-wrap`, `pre-line`", Example: "white-space: pre;", Group: GroupText},
	{Name: "hyphens", Values: "`auto`, `manual`, `none`", Example: "hyphens: none;", Note: "`manual` and `none` both switch the language patterns off, soft hyphens (U+00AD) stay break opportunities", Group: GroupText},
	{Name: "line-snap", Values: "`none` (the initial value), `baseline`", Example: "line-snap: baseline;", Note: "Inherited. The lines of the element stand on the line grid of the page, which starts at the top of the page area, with the `line-height` of the running text or `-bag-line-grid` as its distance, so lines stand level across columns. The rest between two snapped lines goes into the margins between them, which may shrink to zero. `contain` is not supported. A box with a border or background keeps its inside, only its first line is snapped. Tables and block images are not snapped. Has no effect in xts", Group: GroupText},
	{Name: "hyphenate-character", Values: "`auto` (the initial value) or a string", Example: `hyphenate-character: "=";`, Note: "Inherited. The text at the end of a line that breaks at a hyphenation point, also at a soft hyphen (U+00AD). `auto` is the font's hyphen, an empty string sets nothing. It is shaped as the text around it, and a font without its glyphs keeps the hyphen, with a warning", Group: GroupText},
	{Name: "hyphenate-limit-chars", Values: "`auto` (the initial value) or one to three integers or `auto`: the characters a word needs at least, those before and those after a hyphen", Example: "hyphenate-limit-chars: 6 3 2;", Note: "With two values, the third is the same as the second, with one the other two are `auto`. `auto` leaves a word of any length, two characters before a hyphen and three after it. Nonspacing marks and punctuation do not count. Applies to automatic hyphenation, soft hyphens (U+00AD) stay", Group: GroupText},
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
	{Name: "object-view-box", Values: "`none`, or `inset()` with one to four lengths or percentages, on images (`<img>`, also with an SVG or PDF source)", Example: "object-view-box: inset(10% 20%);", Note: "Not inherited. The region becomes the image's natural size and aspect ratio, and `width`, `height` and `max-width` size it as they size a whole image. `xywh()` and `rect()` are not supported and warn, as do the `round` corners of `inset()`, which are ignored. A bitmap's natural size is one point per pixel, as for its sizing, so `inset(10pt)` takes 10 pixels off each edge. Not read on an inline `<svg>`", Group: GroupBox},
	{Name: "background-color", Values: "Color value, painted on block elements, inline elements and table cells", Example: "background-color: #ffffcc;", Group: GroupBox},
	{Name: "background", Values: "Shorthand, only the color is read", Example: "background: #ffffcc;", Group: GroupBox},
	{Name: "display", Values: "`block`, `inline`, `flow-root`, `none`", Example: "display: none;", Note: "`flow-root` is a block that keeps clear of a float as a whole, where a block runs on under it. Other values are ignored with a warning, the table parts follow from the HTML tags", Group: GroupBox},

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
	{Name: "page-break-before", Aliases: []string{"break-before"}, Values: "`auto`, `always`, `avoid`; `break-before` also takes `page`, `left`, `right`, `recto`, `verso` and `column`", Example: "page-break-before: always;", Note: "The page side variants go on to a page of that side and leave a page empty if they have to, for which `@page :blank` applies. `column` starts the next column of a multi-column layout, and the next page after its last column; elsewhere it has no effect. A forced break is taken at any depth; on a block's first child it applies to the block itself", Group: GroupLayout},
	{Name: "page-break-after", Aliases: []string{"break-after"}, Values: "Same values as `page-break-before`", Example: "page-break-after: avoid;", Note: "A forced break is taken at any depth; on a block's last child it applies to the block itself", Group: GroupLayout},
	{Name: "page-break-inside", Aliases: []string{"break-inside"}, Values: "`auto`, `avoid`", Example: "page-break-inside: avoid;", Note: "On a table row, `auto` lets the row break across pages; a row without the property stays whole, as with `avoid`", Group: GroupLayout},
	{Name: "widows", Values: "Positive integer, the default is 2; `1` switches the control off", Example: "widows: 3;", Note: "Counts the lines of a paragraph, a list item or `<pre>`, also inside a container; the blocks of a container are not counted", Group: GroupLayout},
	{Name: "orphans", Values: "Positive integer, the default is 2; `1` switches the control off", Example: "orphans: 3;", Note: "Counts as `widows` does", Group: GroupLayout},
	{Name: "page", Values: "`auto` (the initial value) or a name", Example: "page: wide;", Note: "Sets a block on pages of that name, which `@page wide` applies to, so a part of a document can have its own size, margins or margin boxes. The value is not inherited, but a block with `auto` is set on the pages of its parent. A page break comes between two neighboring blocks with different names. No effect in xts, which selects its pages with master pages", Group: GroupLayout},
	{Name: "column-count", Values: "`auto` (the initial value) or a positive integer", Example: "column-count: 2;", Note: "Laid out on `body`, or on a direct child of `body` without a border, a background or padding, whose siblings then span the columns. Every page holds that many columns side by side, filled one after the other, and the floats and footnotes of a column are placed in it at its width. On another element the content is set in one column, with a warning", Group: GroupLayout},
	{Name: "columns", Values: "Shorthand for `column-width` and `column-count`", Example: "columns: 2;", Note: "Only the count is read, a width is ignored", Group: GroupLayout},
	{Name: "column-gap", Values: "`normal` (1em, the initial value) or a length", Example: "column-gap: 6mm;", Group: GroupLayout},
	{Name: "column-span", Values: "`none` (the initial value), `all`", Example: "column-span: all;", Note: "On a direct child of the element with `column-count`. The spanner stands below the tallest column before it, whose row is balanced, and the columns go on below it. Floats and footnotes inside a spanner are placed at the foot of the page, where the columns below do not make room for them", Group: GroupLayout},
	{Name: "column-fill", Values: "`balance` (the initial value), `auto`; `balance-all` counts as `balance`", Example: "column-fill: auto;", Note: "`balance` evens out the columns of the last page, `auto` fills them one after the other; the columns above a spanner are balanced either way. Floats and footnotes of a balanced row are placed at the foot of its columns. A row whose content starts in its last column stays full", Group: GroupLayout},
	{Name: "column-rule", Values: "Shorthand for `column-rule-width`, `column-rule-style` and `column-rule-color`, as `border`", Example: "column-rule: 0.5pt solid gray;", Note: "Drawn in the middle of the gap between two columns that both hold content, as high as the tallest column of the row. As with borders, `solid` is the only style drawn", Group: GroupLayout},
	{Name: "column-rule-width", Aliases: []string{"column-rule-style", "column-rule-color"}, Values: "As `border-width`, `border-style` and `border-color`", Example: "column-rule-color: #999;", Note: "The color defaults to the color of the element", Group: GroupLayout},
	{Name: "box-decoration-break", Values: "`slice` (the initial value), `clone`", Example: "box-decoration-break: clone;", Note: "Not inherited. On a block split across pages or regions, `clone` gives every fragment the block's padding and border on both sides, and its background. With `text-box-trim` on a paragraph, every fragment is trimmed. Applies to blocks and block containers; tables and inline boxes are not cloned", Group: GroupLayout},

	// Lists and generated content
	{Name: "list-style-type", Values: "`disc`, `circle`, `square`, `none`, `decimal`, `decimal-leading-zero`, `lower-alpha`, `upper-alpha`, `lower-latin`, `upper-latin`, `lower-roman`, `upper-roman`, `lower-greek`", Example: "list-style-type: lower-roman;", Group: GroupGenerated},
	{Name: "list-style", Values: "Shorthand for `list-style-type` and `list-style-position`", Example: "list-style: square;", Group: GroupGenerated},
	{Name: "list-style-position", Values: "`inside`, `outside`", Example: "list-style-position: outside;", Note: "Accepted for compatibility, markers are always placed outside", Group: GroupGenerated},
	{Name: "counter-reset", Values: "Counter name with an optional start value, repeated for several counters", Example: "counter-reset: section;", Group: GroupGenerated},
	{Name: "counter-increment", Values: "Counter name with an optional step, repeated for several counters", Example: "counter-increment: section;", Group: GroupGenerated},
	{Name: "content", Values: "On `::before`, `::after`, `::marker` and in page margin boxes: strings, `attr()`, `counter()`, `counters()`, `target-counter()`, `target-counters()`, `target-text()`, `element()`, `leader()`. The counter functions take a counter style as their last argument", Example: `content: counter(section, upper-roman) ". ";`, Group: GroupGenerated},

	// Custom properties
	{Name: "string-set", Values: "`none`, or a name and a value of strings, `content()`, `attr()` and `counter()`, several pairs separated by commas", Example: "string-set: chapter content();", Note: "Not inherited. Read in a page margin box with `string(name)`, optionally `string(name, first)`, `start`, `last` or `first-except`. `content()` is the text of the element. An empty block assigns nothing, as it has no box", Group: GroupGenerated},
	{Name: "-bag-font-expansion", Values: "Percentage of allowed glyph stretching, `0%` turns it off", Example: "-bag-font-expansion: 0%;", Note: "Only justified text uses it; ragged text is set at its natural width", Group: GroupCustom},
	{Name: "-bag-horizontal-scale", Values: "A positive percentage or number, `90%` and `0.9` are the same, default `100%`", Example: "-bag-horizontal-scale: 90%;", Note: "Inherited. Draws the glyphs narrower or wider by a fixed factor (PDF horizontal scaling, `Tz`); the advances, kerns and spaces are scaled, so the line breaker sees the scaled width. The factor does not compound, `90%` inside `90%` stays 90%, so an element inside a scaled one returns to normal with `100%`. `letter-spacing` is not scaled, it stays an absolute length. Font expansion applies on top. Zero, negative or unparsable values are ignored with a warning", Group: GroupCustom},
	{Name: "-bag-italic-correction", Values: "`auto`, `none`", Example: "-bag-italic-correction: none;", Group: GroupCustom},
	{Name: "-bag-line-breaker", Values: "`auto` (Knuth-Plass, the default) or a name registered with `CSSBuilder.RegisterBreaker`", Example: "-bag-line-breaker: greedy;", Note: "Chooses where a paragraph breaks among its legal breakpoints; the lines are measured and set as with Knuth-Plass. Inherited. An unregistered name keeps Knuth-Plass and logs a warning once", Group: GroupCustom},
	{Name: "-bag-leading-model", Values: "`half` (CSS line boxes, the default), `trailing` (TeX style), or a name registered with `CSSBuilder.RegisterLineModel`", Example: "-bag-leading-model: trailing;", Note: "An unregistered name keeps the built-in leading and logs a warning once", Group: GroupCustom},
	{Name: "-bag-linebreak-tolerance", Values: "Number, the largest adjustment ratio a line may have, default 4", Example: "-bag-linebreak-tolerance: 8;", Note: "How far the spaces of a line may stretch, as a multiple of their stretchability. A ratio, not a badness as TeX's `\\tolerance`: TeX's 200 is about 1.26. Below 1 it also limits how far the spaces may shrink, and 0 accepts only lines that fit exactly; a negative value is ignored", Group: GroupCustom},
	{Name: "-bag-linebreak-hyphen-penalty", Values: "Number, the TeX hyphen penalty, default 50", Example: "-bag-linebreak-hyphen-penalty: 200;", Note: "The penalty of a break at a hyphenation point, soft hyphens included. As in TeX, its square goes into the demerits of the line, and 10000 or more forbids breaks at hyphenation points", Group: GroupCustom},
	{Name: "-bag-tab-stops", Values: "`none`, or a comma separated list of stops: a length or percentage of the line width, optionally `start`, `end`, `center`, `decimal` or `decimal(\",\")`, optionally `leader(\" . \")` (or `dotted`, `solid`, `space`)", Example: "-bag-tab-stops: 12mm, 100% end leader(dotted);", Note: "Inherited. A tab advances to the next stop past the text before it, `left` and `right` are synonyms of `start` and `end`. Tabs within a line survive any `white-space` mode, whitespace with a newline stays source formatting", Group: GroupCustom},
	{Name: "-bag-fixed-height", Values: "Length, or `none` (the default)", Example: "-bag-fixed-height: 16pt;", Note: "On table rows only, not inherited. The row is exactly this high and never breaks inside: content that does not fit draws past the row, background and borders keep the height. Wins over `height` on the same row, and a cell's `height` in a fixed row is ignored. On a cell the property has no effect", Group: GroupCustom},
	{Name: "-bag-float-gutter", Values: "Non-negative length, default `9pt`", Example: "-bag-float-gutter: 0;", Note: "Inherited. The space between a float and the text beside it where the float's margin on the side of the text is zero; any other margin replaces it, a negative one lets the text run over the float as in CSS. Two floats next to each other are not held apart by it. `margin: 0` cannot be told from no margin, so `-bag-float-gutter: 0` is how to get a browser's spacing", Group: GroupCustom},
	{Name: "-bag-text-box-trim-at-break", Values: "`none` (the default), `trim-end`", Example: "-bag-text-box-trim-at-break: trim-end;", Note: "Not inherited. At an unforced break, the last line of the block before it may fit by its text: the space below its font's text-under edge (as `text-box-trim: trim-end` takes off) is trimmed there. Nothing else changes: the block keeps its height where it ends mid-page, its first line is not trimmed, and its decorations are not repeated. A negative trim (lines set tighter than the content area) gains no room and is left alone. Under `box-decoration-break: clone` with `text-box-trim: trim-end`, which trims every fragment of a paragraph already, it changes nothing. Applies only to a block that holds its own lines", Group: GroupCustom},
	{Name: "-bag-trace", Values: "`boxmodel`, `linegrid`, both, or `none` (the default)", Example: "-bag-trace: linegrid;", Note: "Not inherited. `boxmodel` paints the box model overlay (margin, border, padding and content) on the matched elements only, as the global box model trace does on every block. `linegrid` draws numbered lines on the baselines of every page, in xts of every band of a Flow, at the line-height of the matched element, on `body` of the running text. `none` turns the overlay off again in a more specific rule; it cannot turn off a global trace", Group: GroupCustom},
	{Name: "-bag-bookmark", Values: "`none`, or a level number optionally followed by `open` or `closed`", Example: "-bag-bookmark: 2 closed;", Note: "Adds the element to the PDF outline", Group: GroupCustom},

	// @page descriptors
	{Name: "size", Values: "A paper name (`a0` to `a8`, `b0` to `b8`, `jis-b4`, `jis-b5`, `letter`, `legal`, `ledger`) with optional `portrait` or `landscape`, or width and height as two lengths", Example: "size: a4 landscape;", Note: "Without a paper name or lengths the page is A4", Group: GroupPage},
	{Name: "margin", Values: "One to four lengths, the page margins", Example: "margin: 2cm 1.5cm;", Group: GroupPage},
	{Name: "margin-top", Aliases: []string{"margin-right", "margin-bottom", "margin-left"}, Values: "Length, one page margin", Example: "margin-left: 3cm;", Group: GroupPage},
	{Name: "border", Aliases: []string{"border-top", "border-right", "border-bottom", "border-left", "border-width", "border-style", "border-color", "border-radius"}, Values: "As on elements, drawn around the page content area on every page", Example: "border-left: 4pt solid navy;", Group: GroupPage},
	{Name: "padding", Aliases: []string{"padding-top", "padding-right", "padding-bottom", "padding-left"}, Values: "Length, space between the page border and the content", Example: "padding: 5mm;", Group: GroupPage},
	{Name: "bleed", Values: "`auto` (the default) or a length of zero or more", Example: "bleed: 3mm;", Note: "How far the content may run past the trimmed sheet. The page grows by it on all sides and gets a BleedBox, the page background fills it. `auto` is 6pt with crop marks and 0 otherwise. Holds for the whole document, the rule of the first page decides", Group: GroupPage},
	{Name: "marks", Values: "`none` (the default) or `crop`", Example: "marks: crop;", Note: "Crop marks outside the bleed, on every plate including spot colors. `cross` warns and is ignored. Holds for the whole document, the rule of the first page decides", Group: GroupPage},
	{Name: "background-color", Values: "Color value, fills the sheet and its bleed", Example: "background-color: #fafafa;", Group: GroupPage},
	{Name: "background-image", Values: "`url()` of an image or PDF file, scaled to the full sheet and its bleed on every page that uses this `@page` rule", Example: `background-image: url("letterhead.pdf");`, Group: GroupPage},
	{Name: "-bag-line-grid", Values: "Length above zero", Example: "-bag-line-grid: 12pt;", Note: "The distance of the lines of the line grid on these pages, the one `line-snap` snaps to and `-bag-trace: linegrid` draws. Without it the grid takes the `line-height` of the running text. Has no effect in xts", Group: GroupPage},
	{Name: "-bag-background-page", Values: "Page number of a multi page PDF used as `background-image`, the default is 1", Example: "-bag-background-page: 2;", Group: GroupPage},

	// @font-face descriptors
	{Name: "font-family", Values: "The name the font is used by", Example: `font-family: "Minion Pro";`, Group: GroupFontFace},
	{Name: "font-style", Values: "`normal`, `italic`, `oblique`", Example: "font-style: italic;", Note: "`oblique` counts as `italic`, an angle is ignored", Group: GroupFontFace},
	{Name: "font-weight", Values: "A weight name or number, or two of them for the range a variable font covers", Example: "font-weight: 300 700;", Group: GroupFontFace},
	{Name: "src", Values: "Comma separated list of `url()` or `local()` sources, each optionally followed by `format()` and `tech()`. A file can be TrueType, OpenType (also collections), WOFF or WOFF2", Example: `src: url("minion.otf");`, Note: "The first source that can be used wins: one with an unsupported `format()` or `tech()`, a missing file, or a `local()` name of a font the document does not know is skipped", Group: GroupFontFace},
	{Name: "font-feature-settings", Values: "OpenType features switched on for this face, as on elements", Example: `font-feature-settings: "onum";`, Group: GroupFontFace},
	{Name: "font-variation-settings", Values: "Axis values of a variable font, as on elements", Example: `font-variation-settings: "wght" 450;`, Group: GroupFontFace},
	{Name: "font-display", Values: "`auto`, `block`, `swap`, `fallback`, `optional`", Example: "font-display: block;", Note: "Accepted and ignored: it tells a browser what to show while the font loads, and a PDF always waits for its fonts", Group: GroupFontFace},
	{Name: "unicode-range", Values: "Comma separated code points (`U+20AC`), ranges (`U+0025-00FF`) and wildcards (`U+4??`)", Example: "unicode-range: U+0000-00FF;", Note: "Rules of one family with the same weight and style form one face: a character takes the last rule whose range contains it and whose font has the glyph. A font no character of the text falls into is not loaded", Group: GroupFontFace},
	{Name: "size-adjust", Values: "Percentage that scales the glyphs of this face", Example: "size-adjust: 95%;", Group: GroupFontFace},
	{Name: "ascent-override", Values: "`normal` or a percentage of the em, in place of the face's ascent", Example: "ascent-override: 107.91%;", Note: "Sets the content area of the face, which an inline background covers and from which `text-box-trim` measures. The line box reads it only in a line model registered with `CSSBuilder.RegisterLineModel` (see `-bag-leading-model`); the built-in `half` and `trailing` models do not use it", Group: GroupFontFace},
	{Name: "descent-override", Values: "`normal` or a percentage of the em, in place of the face's descent", Example: "descent-override: 25.1%;", Note: "Sets the content area of the face, which an inline background covers and from which `text-box-trim` measures. The line box reads it only in a line model registered with `CSSBuilder.RegisterLineModel` (see `-bag-leading-model`); the built-in `half` and `trailing` models do not use it", Group: GroupFontFace},
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
