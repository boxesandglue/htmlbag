package htmlbag

import (
	"os"
	"path/filepath"
	"testing"
)

// The link elements of a document are read as a browser does (issue #112):
// only rel="stylesheet" loads CSS, in the head and in the body, and the
// stylesheets of link and style elements apply in document order.

// linkTestColor returns the color of the first p in html, read with
// ProcessHTMLChunk from a directory that holds the given CSS files.
func linkTestColor(t *testing.T, files map[string]string, html string) string {
	t.Helper()
	dir := t.TempDir()
	for name, css := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(css), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cssParser := NewCSSParserWithDefaults()
	cssParser.PushDir(dir)
	doc, err := cssParser.ProcessHTMLChunk(html)
	if err != nil {
		t.Fatal(err)
	}
	return cssParser.ComputedStyles(doc.Find("p").Nodes[0]).Get("color")
}

func TestLinkStylesheet(t *testing.T) {
	files := map[string]string{
		"red.css":  "p { color: red }",
		"blue.css": "p { color: blue }",
	}
	tests := []struct {
		name, html, want string
	}{
		{"rel stylesheet in the head",
			`<html><head><link rel="stylesheet" href="red.css"></head><body><p>x</p></body></html>`, "red"},
		{"rel stylesheet in the body",
			`<html><body><link rel="stylesheet" href="red.css"><p>x</p></body></html>`, "red"},
		{"rel icon is no stylesheet",
			`<html><head><link rel="icon" href="red.css"></head><body><p>x</p></body></html>`, ""},
		{"no rel is no stylesheet",
			`<html><head><link href="red.css"></head><body><p>x</p></body></html>`, ""},
		{"alternate stylesheet is not applied",
			`<html><head><link rel="alternate stylesheet" href="red.css"></head><body><p>x</p></body></html>`, ""},
		{"rel is a list without case",
			`<html><head><link rel="preload  Stylesheet" href="red.css"></head><body><p>x</p></body></html>`, "red"},
		{"link after style wins",
			`<html><head><style>p { color: blue }</style><link rel="stylesheet" href="red.css"></head><body><p>x</p></body></html>`, "red"},
		{"style after link wins",
			`<html><head><link rel="stylesheet" href="red.css"><style>p { color: blue }</style></head><body><p>x</p></body></html>`, "blue"},
		{"links in document order",
			`<html><head><link rel="stylesheet" href="blue.css"></head><body><link rel="stylesheet" href="red.css"><p>x</p></body></html>`, "red"},
		{"the example of the issue",
			`<html><head><link rel="icon" href="red.css"></head><body><link rel="stylesheet" href="blue.css"><p>x</p></body></html>`, "blue"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkTestColor(t, files, tc.html); got != tc.want {
				t.Errorf("color = %q, want %q", got, tc.want)
			}
		})
	}
}

// A link inside an SVG image belongs to the image.
func TestLinkInSVGStaysInSVG(t *testing.T) {
	got := linkTestColor(t, map[string]string{"red.css": "p { color: red }"},
		`<html><body><p>x</p><svg xmlns="http://www.w3.org/2000/svg"><link rel="stylesheet" href="red.css"/></svg></body></html>`)
	if got == "red" {
		t.Error("a link inside an SVG image applies to the document")
	}
}
