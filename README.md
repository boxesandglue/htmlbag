# htmlbag

htmlbag is the HTML/CSS renderer within the Boxes and Glue stack. It turns HTML fragments plus CSS into internal text and node structures (`frontend.Text`, `node.VList`, etc.) that are later shipped to PDF.

## Core pieces
- `CSSBuilder` (cssbuilder.go): owns a `frontend.Document` and `csshtml.CSS`, parses HTML (`ParseHTMLFromNode`/`HTMLToText`), applies CSS, and builds vlists.
- Styles: `inheritablestyles.go` models CSS inheritance; list markers, indents, and table handling live here and in `htmltable.go`.
- Rendering: `vlistbuilder.go` builds vertical lists from `frontend.Text`; `output.go` ships pages via the frontend/pdfdraw backend.
- Fonts: `fonts.go` loads embedded webfonts; assets live under `fonts/`.

## Quick start
```go
css := csshtml.New()
_ = css.ParseString(`@page { size: A4; } body { font-family: serif; }`)
doc := frontend.NewDocument()
cb, _ := htmlbag.New(doc, css)

te, _ := cb.HTMLToText(`<p><b>Hello</b> world</p>`)
vl, _ := cb.CreateVlist(te, bag.MustSP("15cm"))
doc.Doc.OutputAt(0, 0, vl) // example: output directly
```

## Development
- Go 1.22+. Import path: `github.com/boxesandglue/htmlbag`.
- Tests/lint: no test at the moment.
- Changes in the CSS/HTML pipeline should be validated against PDF references (downstream projects rely on stable output).
