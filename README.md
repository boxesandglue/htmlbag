# htmlbag

[![Explore in Constellation](https://img.shields.io/badge/Explore%20in-Constellation-blue)](https://constellation.speedata.de/#htmlbag)

htmlbag is the HTML/CSS renderer within the Boxes and Glue stack. It parses CSS
stylesheets, matches them against an HTML document, and turns the styled DOM
into internal text and node structures (`frontend.Text`, `node.VList`, etc.)
that are later shipped to PDF. [bagme](https://github.com/boxesandglue/bagme)
and the HTML mode of [glu](https://github.com/boxesandglue/glu) are built on it.

<p align="center">
  <a href="https://github.com/boxesandglue/boxesandglue-examples/tree/main/glu/html/multi-column">
    <img src="https://raw.githubusercontent.com/boxesandglue/boxesandglue-examples/main/glu/html/multi-column/firstpage.png" width="320" alt="Multi-column page with a spanning title, rendered by htmlbag from HTML and CSS">
  </a>
</p>

The CSS parser and selector engine used to be the separate package
`github.com/boxesandglue/csshtml`, now archived. Its identifiers kept their
names, so `csshtml.NewCSSParserWithDefaults` became
`htmlbag.NewCSSParserWithDefaults`.

## Installation

```bash
go get github.com/boxesandglue/htmlbag
```

## Documentation

The [htmlbag manual](https://boxesandglue.dev/htmlbag/) describes the
supported HTML elements and CSS properties, tables, floats, footnotes, cross
references and the Go API.

## Core pieces
- CSS: `css.go` parses stylesheets (including `@import`, `@font-face` and
  `@page`), `tree.go` matches the rules against the DOM via
  [cascadia](https://github.com/andybalholm/cascadia) and writes the computed
  styles onto the nodes, `processhtml.go` loads HTML files or chunks together
  with their linked stylesheets, `nesting.go` flattens CSS nesting.
- `CSSBuilder` (cssbuilder.go): owns a `frontend.Document` and a `CSS`, parses
  HTML (`ParseHTMLFromNode`/`HTMLToText`), applies CSS, and builds vlists.
- Styles: `inheritablestyles.go` models CSS inheritance; list markers, indents,
  and table handling live here and in `htmltable.go`.
- Rendering: `vlistbuilder.go` builds vertical lists from `frontend.Text`;
  `output.go` ships pages via the frontend/pdfdraw backend.
- Fonts: `fonts.go` loads embedded webfonts; assets live under `fonts/`.

## Quick start
```go
css := htmlbag.NewCSSParserWithDefaults()
_ = css.AddCSSText(`@page { size: A4; } body { font-family: serif; }`)
doc := frontend.NewDocument()
cb, _ := htmlbag.New(doc, css)

te, _ := cb.HTMLToText(`<p><b>Hello</b> world</p>`)
vl, _ := cb.CreateVlist(te, bag.MustSP("15cm"))
doc.Doc.OutputAt(0, 0, vl) // example: output directly
```

## Development
- Go 1.25+. Import path: `github.com/boxesandglue/htmlbag`.
- Tests: `go test ./...`.
- Changes in the CSS/HTML pipeline should be validated against PDF references (downstream projects rely on stable output).

## Ecosystem

htmlbag is part of a broader ecosystem of PDF, typesetting and publishing technologies.

**[Explore the constellation →](https://constellation.speedata.de/#htmlbag)**
