package htmlbag

import (
	"path/filepath"
	"testing"
)

// A floated <img src="x.svg"> floats as a raster image does: the SVG paths
// build their own node, which has to carry the float side and margins too,
// at an absolute width and at a percentage width that waits for the
// container.
func TestAFloatedSVGImageFloats(t *testing.T) {
	svg, err := filepath.Abs("testdata/float.svg")
	if err != nil {
		t.Fatal(err)
	}
	img := func(style string) string {
		return `<img src="` + svg + `" style="float:left; ` + style + `">`
	}
	for name, body := range map[string]string{
		"before a paragraph":      img("width:60pt") + `<p>` + floatProse + `</p>`,
		"at a paragraph's start":  `<p>` + img("width:60pt") + floatProse + `</p>`,
		"with a percentage width": img("width:30%") + `<p>` + floatProse + `</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			indents := lineIndents(buildHTML(t, floatBuilder(t), `<div>`+body+`</div>`))
			if len(indents) == 0 {
				t.Fatal("no lines")
			}
			if indents[0] == 0 {
				t.Errorf("the first line beside a floated SVG image is not indented: %v", indents)
			}
		})
	}
}
