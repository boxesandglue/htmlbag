package htmlbag

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestParseUnicodeRange(t *testing.T) {
	type cr = frontend.CodepointRange
	tests := []struct {
		css  string
		want []cr
		ok   bool
	}{
		{"U+0025-00FF", []cr{{Low: 0x25, High: 0xFF}}, true},
		{"U+20AC, U+0400-045F", []cr{{Low: 0x20AC, High: 0x20AC}, {Low: 0x400, High: 0x45F}}, true},
		{"U+4??", []cr{{Low: 0x400, High: 0x4FF}}, true},
		// Font Awesome's minified CSS writes the ranges in lower case,
		// which the scanner splits into several tokens.
		{"u+f003,u+f016-f017,u+0-7f,u+1e00-1e9f,u+00e9", []cr{{Low: 0xF003, High: 0xF003}, {Low: 0xF016, High: 0xF017}, {Low: 0, High: 0x7F}, {Low: 0x1E00, High: 0x1E9F}, {Low: 0xE9, High: 0xE9}}, true},
		// Clipped to the last code point, an empty interval dropped.
		{"U+10FF00-1FFFFF, U+0050-0040", []cr{{Low: 0x10FF00, High: 0x10FFFF}}, true},
		{"U+", nil, false},
		{"U+12345678", nil, false},
		{"U+4?5", nil, false},
		{"latin", nil, false},
	}
	for _, tc := range tests {
		cp := NewCSSParser()
		if err := cp.AddCSSText(`@font-face { font-family: "F"; src: url("f.ttf"); unicode-range: ` + tc.css + `; }`); err != nil {
			t.Fatalf("%s: %v", tc.css, err)
		}
		got := cp.FontFaces[0].UnicodeRange
		if !tc.ok {
			if got != nil {
				t.Errorf("%s: got %v, want the descriptor dropped", tc.css, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.css, got, tc.want)
		}
	}
}

// TestFontFaceSrcEntriesSeparate guards the src list parser: an entry must
// not inherit the format() or local() of the entry before it.
func TestFontFaceSrcEntriesSeparate(t *testing.T) {
	cp := NewCSSParser()
	if err := cp.AddCSSText(`@font-face { font-family: "F"; src: local("F Regular"), url("f.woff2") format("woff2"), url("f.ttf"); }`); err != nil {
		t.Fatal(err)
	}
	src := cp.FontFaces[0].Source
	if len(src) != 3 {
		t.Fatalf("got %d src entries, want 3", len(src))
	}
	if src[1].Local != "" || src[2].Format != "" {
		t.Errorf("entries leak into the next: %+v", src)
	}
}

// TestFontSourceFromCSSTakesFirstUsable checks that the first src entry the
// typesetter can use wins, not the last one: unsupported formats and
// technologies and missing files are skipped.
func TestFontSourceFromCSSTakesFirstUsable(t *testing.T) {
	dir := t.TempDir()
	woff2 := filepath.Join(dir, "f.woff2")
	ttf := filepath.Join(dir, "f.ttf")
	for _, f := range []string{woff2, ttf} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.otf")
	tests := []struct {
		name string
		srcs []FontSource
		want string
	}{
		{"first wins", []FontSource{{URI: woff2, Format: "woff2"}, {URI: ttf, Format: "truetype"}}, woff2},
		{"unsupported format", []FontSource{{URI: missing, Format: "embedded-opentype"}, {URI: ttf, Format: "truetype"}}, ttf},
		{"unsupported tech", []FontSource{{URI: woff2, Format: "opentype", Tech: "color-COLRv1"}, {URI: ttf}}, ttf},
		{"legacy variations format", []FontSource{{URI: woff2, Format: `"woff2-variations"`}}, woff2},
		{"missing file", []FontSource{{URI: missing}, {URI: ttf}}, ttf},
		{"unknown local", []FontSource{{Local: "No Such Font"}, {URI: ttf}}, ttf},
		{"built-in local", []FontSource{{Local: "TeXGyreHeros Bold"}, {URI: ttf}}, "TeXGyreHeros Bold"},
		{"nothing available", []FontSource{{URI: missing}}, missing},
	}
	for _, tc := range tests {
		fs, ok := fontSourceFromCSS(tc.srcs, fe)
		if !ok || fs.Name != tc.want {
			t.Errorf("%s: got %v, want %s", tc.name, fs, tc.want)
		}
	}
	if fs, ok := fontSourceFromCSS([]FontSource{{URI: woff2, Format: "svg"}}, fe); ok {
		t.Errorf("only unsupported formats: got %v, want none", fs)
	}
}

// TestAddFontFamiliesFromCSSComposite checks that @font-face rules of one
// family with the same descriptors combine instead of replacing each other,
// and that calling AddFontFamiliesFromCSS again, as HTMLToText and InitPage
// do, does not add them twice.
func TestAddFontFamiliesFromCSSComposite(t *testing.T) {
	cp := NewCSSParser()
	css := `
	@font-face { font-family: "Roboto"; src: url("ext.woff2"); unicode-range: U+0100-024F; }
	@font-face { font-family: "Roboto"; src: url("latin.woff2"); unicode-range: U+0000-00FF; }`
	if err := cp.AddCSSText(css); err != nil {
		t.Fatal(err)
	}
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := AddFontFamiliesFromCSS(cp, fe); err != nil {
			t.Fatal(err)
		}
	}
	ff := fe.FindFontFamily("Roboto")
	fs, err := ff.GetFontSource(400, frontend.FontStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	if fs.Name != "latin.woff2" || len(fs.UnicodeRange) != 1 || fs.UnicodeRange[0].High != 0xFF {
		t.Errorf("first available font = %v %v, want latin.woff2 with its range", fs, fs.UnicodeRange)
	}

	// A rule parsed later joins the face once.
	if err := cp.AddCSSText(`@font-face { font-family: "Roboto"; src: url("cyrillic.woff2"); unicode-range: U+0400-045F; }`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := AddFontFamiliesFromCSS(cp, fe); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(cp.FontFaces); got != 3 {
		t.Fatalf("got %d font faces, want 3", got)
	}
	if got := cp.fontFacesAdded[fe]; got != 3 {
		t.Errorf("rules added = %d, want 3", got)
	}
}
