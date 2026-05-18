package htmlbag

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestRasterImageFormatterAspectPreservation calls the formatter
// closure directly: a 200×100 (2:1) intrinsic image with a 50%
// formatter against a 400pt container should resize to 200×100pt.
// Aspect ratio is preserved when no explicit height is given.
func TestRasterImageFormatterAspectPreservation(t *testing.T) {
	intrinsicWd := bag.MustSP("200pt")
	intrinsicHt := bag.MustSP("100pt")
	img := &node.Image{Width: intrinsicWd, Height: intrinsicHt}
	ftv := newRasterImageFormatter(img, intrinsicWd, intrinsicHt, 50, 0)
	vl, err := ftv(bag.MustSP("400pt"))
	if err != nil {
		t.Fatalf("FormatToVList: %v", err)
	}
	if want := bag.MustSP("200pt"); vl.Width != want {
		t.Errorf("width = %s, want %s", vl.Width, want)
	}
	if want := bag.MustSP("100pt"); vl.Height != want {
		t.Errorf("height = %s, want %s (preserved 2:1 aspect)", vl.Height, want)
	}
	if img.Width != vl.Width || img.Height != vl.Height {
		t.Error("FormatToVList did not mutate the underlying *node.Image")
	}
}

// TestRasterImageFormatterExplicitHeightWins covers <img height="60pt"
// width="50%"> — explicit height must override aspect-derived height.
func TestRasterImageFormatterExplicitHeightWins(t *testing.T) {
	img := &node.Image{Width: bag.MustSP("100pt"), Height: bag.MustSP("100pt")}
	ftv := newRasterImageFormatter(img, bag.MustSP("100pt"), bag.MustSP("100pt"), 25, bag.MustSP("60pt"))
	vl, _ := ftv(bag.MustSP("400pt"))
	if want := bag.MustSP("100pt"); vl.Width != want {
		t.Errorf("width = %s, want %s (25%% of 400pt)", vl.Width, want)
	}
	if want := bag.MustSP("60pt"); vl.Height != want {
		t.Errorf("height = %s, want %s (explicit height wins over aspect)", vl.Height, want)
	}
}

// TestRasterImageFormatterIdempotent confirms two calls at different
// widths produce the right rescaling.
func TestRasterImageFormatterIdempotent(t *testing.T) {
	img := &node.Image{Width: bag.MustSP("100pt"), Height: bag.MustSP("50pt")}
	ftv := newRasterImageFormatter(img, bag.MustSP("100pt"), bag.MustSP("50pt"), 100, 0)
	ftv(bag.MustSP("400pt"))
	if img.Width != bag.MustSP("400pt") {
		t.Fatalf("first pass: img.Width = %s, want 400pt", img.Width)
	}
	ftv(bag.MustSP("200pt"))
	if img.Width != bag.MustSP("200pt") {
		t.Errorf("second pass: img.Width = %s, want 200pt (last call wins)", img.Width)
	}
}

// writeTinyPNG writes a 4x2 valid PNG to dir/name.png. The 4:2 aspect
// (2:1) lets aspect-preservation be checked end-to-end without
// fractional rounding.
func writeTinyPNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x := 0; x < 4; x++ {
		for y := 0; y < 2; y++ {
			img.Set(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestImgWidth100PercentDoesNotPanic is the regression marker: before
// Phase 3, <img width="100%"> hit bag.MustSP("100%") which panics. This
// test confirms the new parseSVGPercentWidth guard short-circuits before
// MustSP sees the percent.
func TestImgWidth100PercentDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	imgPath := writeTinyPNG(t, dir, "tiny.png")

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	htmlStr := `<html><body><p><img src="` + imgPath + `" width="100%"></p></body></html>`
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HTMLToText panicked on width=100%%: %v", r)
		}
	}()
	if _, err := cb.HTMLToText(htmlStr); err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
}

// TestImgPercentWidthAttachesSizer integration-tests the wiring: an
// <img width="50%"> in HTML must produce a wrapper VList carrying a
// rasterImageDeferred sizer. After running the walker against a known
// container, the underlying image's Width must reflect 50%.
func TestImgPercentWidthAttachesSizer(t *testing.T) {
	dir := t.TempDir()
	imgPath := writeTinyPNG(t, dir, "tiny.png")

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	htmlStr := `<html><body><p><img src="` + imgPath + `" width="50%"></p></body></html>`
	te, err := cb.HTMLToText(htmlStr)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}

	// Find the deferred-sized img wrapper.
	var wrapper *node.VList
	var walk func(*frontend.Text)
	walk = func(t *frontend.Text) {
		for _, itm := range t.Items {
			switch v := itm.(type) {
			case *node.VList:
				if v.Attributes != nil {
					if o, _ := v.Attributes["origin"].(string); o == "img" {
						wrapper = v
					}
				}
			case *frontend.Text:
				walk(v)
			}
		}
	}
	walk(te)
	if wrapper == nil {
		t.Fatal("no deferred-img wrapper found")
	}
	if ftv := getDeferredFormatter(wrapper); ftv == nil {
		t.Fatal("wrapper missing deferred FormatToVList closure")
	}

	// Walk the wrapper through the foundation walker and assert it
	// rescaled the underlying image.
	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))
	want := bag.MustSP("100pt")
	if wrapper.Width != want {
		t.Errorf("wrapper.Width = %s, want %s (50%% of 200pt)", wrapper.Width, want)
	}
}
