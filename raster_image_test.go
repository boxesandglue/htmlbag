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
	ftv := newRasterImageFormatter(img, intrinsicWd, intrinsicHt, imageDims{widthPct: 50})
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
	ftv := newRasterImageFormatter(img, bag.MustSP("100pt"), bag.MustSP("100pt"), imageDims{widthPct: 25, ht: bag.MustSP("60pt")})
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
	ftv := newRasterImageFormatter(img, bag.MustSP("100pt"), bag.MustSP("50pt"), imageDims{widthPct: 100})
	ftv(bag.MustSP("400pt"))
	if img.Width != bag.MustSP("400pt") {
		t.Fatalf("first pass: img.Width = %s, want 400pt", img.Width)
	}
	ftv(bag.MustSP("200pt"))
	if img.Width != bag.MustSP("200pt") {
		t.Errorf("second pass: img.Width = %s, want 200pt (last call wins)", img.Width)
	}
}

// writePNGSized writes a wpx×hpx single-color valid PNG to dir/name.
func writePNGSized(t *testing.T, dir, name string, wpx, hpx int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, wpx, hpx))
	for x := 0; x < wpx; x++ {
		for y := 0; y < hpx; y++ {
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

// writeTinyPNG writes a 4x2 valid PNG to dir/name.png. The 4:2 aspect
// (2:1) lets aspect-preservation be checked end-to-end without
// fractional rounding.
func writeTinyPNG(t *testing.T, dir, name string) string {
	t.Helper()
	return writePNGSized(t, dir, name, 4, 2)
}

// findDeferredImgWrapper walks a frontend.Text tree and returns the
// first VList wrapper stamped origin=img (the deferred raster-image
// carrier), or nil.
func findDeferredImgWrapper(te *frontend.Text) *node.VList {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case *node.VList:
			if v.Attributes != nil {
				if o, _ := v.Attributes["origin"].(string); o == "img" {
					return v
				}
			}
		case *frontend.Text:
			if w := findDeferredImgWrapper(v); w != nil {
				return w
			}
		}
	}
	return nil
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

// TestResolveWidthMaxWidthSemantics table-drives imageDims.resolveWidth:
// max-width caps but never upscales; percent caps resolve against the
// container; the max-content probe drops percent caps but keeps
// absolute ones.
func TestResolveWidthMaxWidthSemantics(t *testing.T) {
	sp := bag.MustSP
	intrinsic := sp("300pt")
	tests := []struct {
		name      string
		dims      imageDims
		container bag.ScaledPoint
		want      bag.ScaledPoint
	}{
		{"percent cap shrinks wider image", imageDims{maxPct: 100}, sp("200pt"), sp("200pt")},
		{"no upscale below cap", imageDims{maxPct: 100}, sp("400pt"), sp("300pt")},
		{"width pct capped by max pct", imageDims{widthPct: 50, maxPct: 30}, sp("400pt"), sp("120pt")},
		{"absolute width capped by percent cap", imageDims{wd: sp("300pt"), maxPct: 100}, sp("200pt"), sp("200pt")},
		{"absolute cap shrinks intrinsic", imageDims{maxWd: sp("150pt")}, sp("400pt"), sp("150pt")},
		{"max-content probe keeps absolute cap", imageDims{widthPct: 100, maxWd: sp("150pt")}, intrinsic * 1000, sp("150pt")},
		{"max-content probe drops percent cap", imageDims{maxPct: 50}, intrinsic * 1000, intrinsic},
	}
	for _, tc := range tests {
		if got := tc.dims.resolveWidth(tc.container, intrinsic); got != tc.want {
			t.Errorf("%s: resolveWidth = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestRasterImageFormatterMaxWidthAspect: a 200×100 image with
// max-width: 100% downscales to 150×75 in a 150pt container and keeps
// its natural 200×100 in a 400pt container; max-width never upscales.
func TestRasterImageFormatterMaxWidthAspect(t *testing.T) {
	intrinsicWd := bag.MustSP("200pt")
	intrinsicHt := bag.MustSP("100pt")
	img := &node.Image{Width: intrinsicWd, Height: intrinsicHt}
	ftv := newRasterImageFormatter(img, intrinsicWd, intrinsicHt, imageDims{maxPct: 100})

	vl, err := ftv(bag.MustSP("150pt"))
	if err != nil {
		t.Fatalf("FormatToVList: %v", err)
	}
	if want := bag.MustSP("150pt"); vl.Width != want {
		t.Errorf("capped width = %s, want %s", vl.Width, want)
	}
	if want := bag.MustSP("75pt"); vl.Height != want {
		t.Errorf("capped height = %s, want %s (preserved 2:1 aspect)", vl.Height, want)
	}

	vl, err = ftv(bag.MustSP("400pt"))
	if err != nil {
		t.Fatalf("FormatToVList: %v", err)
	}
	if vl.Width != intrinsicWd || vl.Height != intrinsicHt {
		t.Errorf("uncapped size = %s×%s, want natural %s×%s (no upscale)", vl.Width, vl.Height, intrinsicWd, intrinsicHt)
	}
}

// TestImgMaxWidthCSSAttachesSizer wires the whole chain: CSS
// `max-width: 100%` reaches the img case as a "!max-width" attribute
// (here via the style attribute) and must attach a deferred sizer that
// caps a too-wide image at the container width, aspect preserved.
func TestImgMaxWidthCSSAttachesSizer(t *testing.T) {
	dir := t.TempDir()
	imgPath := writePNGSized(t, dir, "wide.png", 800, 400)

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
	htmlStr := `<html><body><p><img src="` + imgPath + `" style="max-width: 100%"></p></body></html>`
	te, err := cb.HTMLToText(htmlStr)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	wrapper := findDeferredImgWrapper(te)
	if wrapper == nil {
		t.Fatal("no deferred-img wrapper found for max-width image")
	}
	if getDeferredFormatter(wrapper) == nil {
		t.Fatal("wrapper missing deferred FormatToVList closure")
	}
	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))
	if want := bag.MustSP("200pt"); wrapper.Width != want {
		t.Errorf("wrapper.Width = %s, want %s (capped at container)", wrapper.Width, want)
	}
	if want := bag.MustSP("100pt"); wrapper.Height != want {
		t.Errorf("wrapper.Height = %s, want %s (preserved 2:1 aspect)", wrapper.Height, want)
	}
}

// TestImgMaxWidthDoesNotUpscale: the same CSS on an image narrower
// than the container must leave the natural size untouched.
func TestImgMaxWidthDoesNotUpscale(t *testing.T) {
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
	htmlStr := `<html><body><p><img src="` + imgPath + `" style="max-width: 100%"></p></body></html>`
	te, err := cb.HTMLToText(htmlStr)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	wrapper := findDeferredImgWrapper(te)
	if wrapper == nil {
		t.Fatal("no deferred-img wrapper found for max-width image")
	}
	natural := wrapper.Width
	if natural <= 0 {
		t.Fatalf("placeholder width = %s, want > 0 (natural size)", natural)
	}
	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))
	if wrapper.Width != natural {
		t.Errorf("wrapper.Width = %s, want natural %s (max-width must not upscale)", wrapper.Width, natural)
	}
}
