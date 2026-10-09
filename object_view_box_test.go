package htmlbag

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestParseObjectViewBox(t *testing.T) {
	sp := bag.MustSP
	fs := sp("10pt")
	natW, natH := sp("400pt"), sp("200pt")
	tests := []struct {
		value string
		want  node.ImageCrop
	}{
		{"inset(10pt)", node.ImageCrop{X: 10, Y: 10, Width: 380, Height: 180}},
		{"inset(10% 25%)", node.ImageCrop{X: 100, Y: 20, Width: 200, Height: 160}},
		{"inset(0 10pt 20pt)", node.ImageCrop{X: 10, Y: 0, Width: 380, Height: 180}},
		{"inset(1pt 2pt 3pt 4pt)", node.ImageCrop{X: 4, Y: 1, Width: 394, Height: 196}},
		{"inset(8px)", node.ImageCrop{X: 6, Y: 6, Width: 388, Height: 188}},
		{"inset(1em 0)", node.ImageCrop{X: 0, Y: 10, Width: 400, Height: 180}},
		{"INSET(50% 0 0 0)", node.ImageCrop{X: 0, Y: 100, Width: 400, Height: 100}},
	}
	for _, tc := range tests {
		ovb := parseObjectViewBox(tc.value, fs, fs)
		if ovb == nil {
			t.Errorf("%s: not parsed", tc.value)
			continue
		}
		got, ok := ovb.region(natW, natH)
		if !ok || *got != tc.want {
			t.Errorf("%s: region = %+v (%v), want %+v", tc.value, got, ok, tc.want)
		}
	}
}

// captureWarnings returns the warnings logged while fn runs.
func captureWarnings(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()
	fn()
	return buf.String()
}

func TestParseObjectViewBoxUnsupported(t *testing.T) {
	fs := bag.MustSP("10pt")
	for _, v := range []string{"none", ""} {
		if w := captureWarnings(t, func() {
			if parseObjectViewBox(v, fs, fs) != nil {
				t.Errorf("%q: got a view box, want none", v)
			}
		}); w != "" {
			t.Errorf("%q: unexpected warning %s", v, w)
		}
	}
	for _, v := range []string{"xywh(0 0 10pt 10pt)", "rect(0 10pt 10pt 0)", "inset()", "inset(1pt 2pt 3pt 4pt 5pt)", "inset(red)", "inset(10pt", "10pt"} {
		if w := captureWarnings(t, func() {
			if parseObjectViewBox(v, fs, fs) != nil {
				t.Errorf("%q: got a view box, want it ignored", v)
			}
		}); !strings.Contains(w, "object-view-box") {
			t.Errorf("%q: no warning", v)
		}
	}
	w := captureWarnings(t, func() {
		ovb := parseObjectViewBox("inset(10pt round 5pt)", fs, fs)
		if ovb == nil || ovb.inset[0].length != bag.MustSP("10pt") {
			t.Errorf("inset with round: got %+v, want the insets read", ovb)
		}
	})
	if !strings.Contains(w, "round") {
		t.Errorf("inset with round: no warning")
	}
}

// findImageNode returns the first image node in te, eager or deferred.
func findImageNode(te *frontend.Text) *node.Image {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case *node.Image:
			return v
		case *node.VList:
			for n := node.Node(v.List); n != nil; n = n.Next() {
				if img, ok := n.(*node.Image); ok {
					return img
				}
			}
		case *frontend.Text:
			if img := findImageNode(v); img != nil {
				return img
			}
		}
	}
	return nil
}

func TestObjectViewBoxBitmap(t *testing.T) {
	imgPath := writePNGSized(t, t.TempDir(), "img.png", 400, 200)
	sp := bag.MustSP
	tests := []struct {
		name, attrs   string
		wantW, wantHt bag.ScaledPoint
	}{
		{"auto size is the region", `style="object-view-box: inset(10% 25%)"`, sp("200pt"), sp("160pt")},
		{"width keeps the region's aspect", `style="object-view-box: inset(10% 25%); width: 100pt"`, sp("100pt"), sp("80pt")},
		{"height keeps the region's aspect", `height="40pt" style="object-view-box: inset(10% 25%)"`, sp("50pt"), sp("40pt")},
		{"both given", `width="30pt" height="40pt" style="object-view-box: inset(10% 25%)"`, sp("30pt"), sp("40pt")},
	}
	for _, tc := range tests {
		te := renderToText(t, `<p><img src="`+imgPath+`" `+tc.attrs+`></p>`)
		img := findImageNode(te)
		if img == nil {
			t.Fatalf("%s: no image node", tc.name)
		}
		if img.Crop == nil || *img.Crop != (node.ImageCrop{X: 100, Y: 20, Width: 200, Height: 160}) {
			t.Errorf("%s: crop = %+v, want {100 20 200 160}", tc.name, img.Crop)
		}
		if img.Width != tc.wantW || img.Height != tc.wantHt {
			t.Errorf("%s: size = %s×%s, want %s×%s", tc.name, img.Width, img.Height, tc.wantW, tc.wantHt)
		}
	}
}

func TestObjectViewBoxBitmapPercentWidth(t *testing.T) {
	imgPath := writePNGSized(t, t.TempDir(), "img.png", 400, 200)
	te := renderToText(t, `<p><img src="`+imgPath+`" width="50%" style="object-view-box: inset(10% 25%)"></p>`)
	wrapper := findDeferredImgWrapper(te)
	if wrapper == nil {
		t.Fatal("no deferred-img wrapper")
	}
	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))
	if wrapper.Width != bag.MustSP("100pt") || wrapper.Height != bag.MustSP("80pt") {
		t.Errorf("size = %s×%s, want 100pt×80pt (the region's 5:4)", wrapper.Width, wrapper.Height)
	}
}

func TestObjectViewBoxNotInherited(t *testing.T) {
	imgPath := writePNGSized(t, t.TempDir(), "img.png", 400, 200)
	te := renderToText(t, `<p style="object-view-box: inset(10%)"><img src="`+imgPath+`"></p>`)
	img := findImageNode(te)
	if img == nil {
		t.Fatal("no image node")
	}
	if img.Crop != nil || img.Width != bag.MustSP("400pt") {
		t.Errorf("crop = %+v, width = %s, want no crop at 400pt", img.Crop, img.Width)
	}
}

func TestObjectViewBoxInvalidIgnored(t *testing.T) {
	imgPath := writePNGSized(t, t.TempDir(), "img.png", 400, 200)
	for _, v := range []string{"xywh(0 0 10pt 10pt)", "inset(60%)"} {
		var img *node.Image
		w := captureWarnings(t, func() {
			img = findImageNode(renderToText(t, `<p><img src="`+imgPath+`" style="object-view-box: `+v+`"></p>`))
		})
		if !strings.Contains(w, "object-view-box") {
			t.Errorf("%s: no warning", v)
		}
		if img == nil || img.Crop != nil || img.Width != bag.MustSP("400pt") {
			t.Errorf("%s: image = %+v, want the whole image", v, img)
		}
	}
}

// renderPDF renders html to PDF with css, with a fixed date and IDs so two
// renders can be compared byte for byte.
func renderPDF(t *testing.T, css, html string) []byte {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	fe.Doc.CreationDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fe.Doc.SuppressInfo = true
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.AddCSS(css); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeTestPDF writes a PDF with a 200×100pt page to dir/name.
func writeTestPDF(t *testing.T, dir, name string) string {
	t.Helper()
	pdf := renderPDF(t, `@page { size: 200pt 100pt; margin: 0 }`, `<p>x</p>`)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestObjectViewBoxPDF(t *testing.T) {
	pdfPath := writeTestPDF(t, t.TempDir(), "page.pdf")
	te := renderToText(t, `<p><img src="`+pdfPath+`" style="object-view-box: inset(25%)"></p>`)
	img := findImageNode(te)
	if img == nil {
		t.Fatal("no image node")
	}
	if img.Crop == nil || *img.Crop != (node.ImageCrop{X: 50, Y: 25, Width: 100, Height: 50}) {
		t.Errorf("crop = %+v, want {50 25 100 50}", img.Crop)
	}
	if img.Width != bag.MustSP("100pt") || img.Height != bag.MustSP("50pt") {
		t.Errorf("size = %s×%s, want 100pt×50pt", img.Width, img.Height)
	}
}

// writeTestSVG writes a 200×100 SVG with a viewBox at twice that scale.
func writeTestSVG(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "img.svg")
	src := `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100" viewBox="0 0 400 200"><rect x="0" y="0" width="200" height="200" fill="red"/><rect x="200" y="0" width="200" height="200" fill="blue"/></svg>`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestObjectViewBoxSVG(t *testing.T) {
	svgPath := writeTestSVG(t, t.TempDir())
	sp := bag.MustSP
	tests := []struct {
		name, attrs   string
		wantW, wantHt bag.ScaledPoint
	}{
		{"auto size is the region", `style="object-view-box: inset(0 50% 0 0)"`, sp("100pt"), sp("100pt")},
		{"width keeps the region's aspect", `width="40pt" style="object-view-box: inset(0 50% 0 0)"`, sp("40pt"), sp("40pt")},
		{"whole image", ``, sp("200pt"), sp("100pt")},
	}
	for _, tc := range tests {
		te := renderToText(t, `<p><img src="`+svgPath+`" `+tc.attrs+`></p>`)
		var rule *node.Rule
		if vl := findSVGImage(te); vl != nil {
			rule = findFirstRule(vl.List)
		}
		if rule == nil {
			t.Fatalf("%s: no SVG rule", tc.name)
		}
		if rule.Width != tc.wantW || rule.Height != tc.wantHt {
			t.Errorf("%s: size = %s×%s, want %s×%s", tc.name, rule.Width, rule.Height, tc.wantW, tc.wantHt)
		}
	}
}

func TestObjectViewBoxSVGPercentWidth(t *testing.T) {
	svgPath := writeTestSVG(t, t.TempDir())
	te := renderToText(t, `<p><img src="`+svgPath+`" width="50%" style="object-view-box: inset(0 50% 0 0)"></p>`)
	vl := findSVGImage(te)
	if vl == nil || getDeferredFormatter(vl) == nil {
		t.Fatal("no deferred SVG wrapper")
	}
	resolveDeferredSizing([]any{vl}, bag.MustSP("200pt"))
	if vl.Width != bag.MustSP("100pt") || vl.Depth != bag.MustSP("100pt") {
		t.Errorf("size = %s×%s, want 100pt×100pt (the region's 1:1)", vl.Width, vl.Depth)
	}
}

// findSVGImage returns the wrapper of the first <img src=*.svg> in te.
func findSVGImage(te *frontend.Text) *node.VList {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case *node.VList:
			if o, _ := v.Attributes["origin"].(string); o == "svg" {
				return v
			}
		case *frontend.Text:
			if vl := findSVGImage(v); vl != nil {
				return vl
			}
		}
	}
	return nil
}

func TestObjectViewBoxNoneUnchanged(t *testing.T) {
	dir := t.TempDir()
	imgPath := writePNGSized(t, dir, "img.png", 40, 20)
	svgPath := writeTestSVG(t, dir)
	body := `<p><img src="` + imgPath + `"%s> <img src="` + svgPath + `"%s></p>`
	plain := renderPDF(t, "", strings.ReplaceAll(body, "%s", ""))
	none := renderPDF(t, "", strings.ReplaceAll(body, "%s", ` style="object-view-box: none"`))
	if !bytes.Equal(plain, none) {
		t.Error("object-view-box: none changes the PDF")
	}
	cropped := renderPDF(t, "", strings.ReplaceAll(body, "%s", ` style="object-view-box: inset(1pt)"`))
	if bytes.Equal(plain, cropped) {
		t.Error("test harness broken: inset(1pt) leaves the PDF unchanged")
	}
}

func TestObjectViewBoxSVGMaxWidth(t *testing.T) {
	svgPath := writeTestSVG(t, t.TempDir())
	te := renderToText(t, `<p><img src="`+svgPath+`" style="object-view-box: inset(0 50% 0 0); max-width: 150pt"></p>`)
	var rule *node.Rule
	if vl := findSVGImage(te); vl != nil {
		rule = findFirstRule(vl.List)
	}
	if rule == nil {
		t.Fatal("no SVG rule")
	}
	if rule.Width != bag.MustSP("100pt") || rule.Height != bag.MustSP("100pt") {
		t.Errorf("size = %s×%s, want 100pt×100pt (the region is under the cap)", rule.Width, rule.Height)
	}
}

func TestObjectViewBoxSVGPercentMaxWidth(t *testing.T) {
	svgPath := writeTestSVG(t, t.TempDir())
	te := renderToText(t, `<p><img src="`+svgPath+`" style="object-view-box: inset(0 50% 0 0); max-width: 100%"></p>`)
	vl := findSVGImage(te)
	if vl == nil || getDeferredFormatter(vl) == nil {
		t.Fatal("no deferred SVG wrapper")
	}
	resolveDeferredSizing([]any{vl}, bag.MustSP("150pt"))
	if vl.Width != bag.MustSP("100pt") || vl.Depth != bag.MustSP("100pt") {
		t.Errorf("size = %s×%s, want 100pt×100pt (the region is under the cap)", vl.Width, vl.Depth)
	}
}

func TestObjectViewBoxSVGRegion(t *testing.T) {
	svgPath := writeTestSVG(t, t.TempDir())
	render := func(inset string) []byte {
		return renderPDF(t, "", `<p><img src="`+svgPath+`" style="object-view-box: inset(`+inset+`)"></p>`)
	}
	if bytes.Equal(render("0 50% 0 0"), render("0 0 0 50%")) {
		t.Error("the left and right halves render the same PDF")
	}
}
