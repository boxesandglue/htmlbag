package htmlbag

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/font"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// fixedModel sets every line 20pt above and 5pt below the baseline and
// records the largest line shift it saw.
type fixedModel struct {
	maxShift *bag.ScaledPoint
}

func (m fixedModel) LineBox(hl *node.HList, _ *node.LinebreakSettings) (bag.ScaledPoint, bag.ScaledPoint) {
	for n := hl.List; n != nil; n = n.Next() {
		if g, ok := n.(*node.Glyph); ok && g.LineShift > *m.maxShift {
			*m.maxShift = g.LineShift
		}
	}
	return bag.MustSP("20pt"), bag.MustSP("5pt")
}

func (fixedModel) Leading(*node.HList, *node.LinebreakSettings) *node.Glue { return nil }

func newLineModelBuilder(t *testing.T, css string, configure func(*CSSBuilder)) (*CSSBuilder, *frontend.Document) {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if configure != nil {
		configure(cb)
	}
	if css != "" {
		if err = cb.AddCSS(css); err != nil {
			t.Fatal(err)
		}
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	return cb, fe
}

// lineModelLines renders html and returns its lines.
func lineModelLines(t *testing.T, css, html string, configure func(*CSSBuilder)) []*node.HList {
	t.Helper()
	cb, _ := newLineModelBuilder(t, css, configure)
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("100pt"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []*node.HList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch v := e.(type) {
			case *node.VList:
				walk(v.List)
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin == "line" {
					lines = append(lines, v)
				} else {
					walk(v.List)
				}
			}
		}
	}
	walk(vl.List)
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	return lines
}

const lineModelPara = `<p lang="de">several words that wrap onto more than one line for sure</p>`

func TestRegisterLineModelRejectsReservedNames(t *testing.T) {
	cb, _ := newLineModelBuilder(t, "", nil)
	f := func(LineModelStyles) node.LineModel { return nil }
	for _, name := range []string{"", "  ", "half", "Trailing", " inherit "} {
		if err := cb.RegisterLineModel(name, f); err == nil {
			t.Errorf("RegisterLineModel(%q) accepted a reserved name", name)
		}
	}
	if err := cb.RegisterLineModel("fixed", nil); err == nil {
		t.Error("RegisterLineModel accepted a nil function")
	}
	if err := cb.RegisterLineModel("Fixed", f); err != nil {
		t.Errorf("RegisterLineModel(Fixed): %v", err)
	}
}

func TestARegisteredLineModelSetsTheLines(t *testing.T) {
	var got []LineModelStyles
	var shift bag.ScaledPoint
	register := func(cb *CSSBuilder) {
		if err := cb.RegisterLineModel("Fixed", func(s LineModelStyles) node.LineModel {
			got = append(got, s)
			return fixedModel{&shift}
		}); err != nil {
			t.Fatal(err)
		}
	}
	lines := lineModelLines(t, `p { -bag-leading-model: FIXED; font-size: 11pt; line-height: 15pt }`,
		`<p lang="de">several words <span style="vertical-align: 2pt">that <span style="vertical-align: 1pt">wrap</span></span> onto more than one line</p>`, register)
	if len(lines) < 2 {
		t.Fatalf("want a multi-line paragraph, got %d lines", len(lines))
	}
	for i, hl := range lines {
		if hl.Height != bag.MustSP("20pt") || hl.Depth != bag.MustSP("5pt") {
			t.Errorf("line %d is %s + %s, want the model's 20pt + 5pt", i, hl.Height, hl.Depth)
		}
	}
	if len(got) == 0 {
		t.Fatal("the model's function was never called")
	}
	want := LineModelStyles{Name: "fixed", FontSize: bag.MustSP("11pt"), LineHeight: bag.MustSP("15pt"), Language: "de"}
	gotFont := got[0].Font
	got[0].Font = nil
	if got[0] != want {
		t.Errorf("the function got %+v, want %+v", got[0], want)
	}
	if gotFont == nil || gotFont.Size != want.FontSize {
		t.Errorf("the function got the font %+v, want one at %s", gotFont, want.FontSize)
	}
	if shift != bag.MustSP("3pt") {
		t.Errorf("the model saw a largest line shift of %s, want the nested 2pt + 1pt", shift)
	}
}

func TestABuiltInLeadingModelOverridesAnInheritedRegisteredOne(t *testing.T) {
	register := func(cb *CSSBuilder) {
		_ = cb.RegisterLineModel("fixed", func(LineModelStyles) node.LineModel {
			var s bag.ScaledPoint
			return fixedModel{&s}
		})
	}
	lines := lineModelLines(t, `body { -bag-leading-model: fixed } p { -bag-leading-model: half }`, lineModelPara, register)
	for i, hl := range lines {
		if got := hl.Height + hl.Depth; got != bag.MustSP("12pt") {
			t.Errorf("line %d spans %s, want the half-leading 12pt", i, got)
		}
	}
}

func TestAnUnregisteredLineModelWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()

	lines := lineModelLines(t, `p { -bag-leading-model: fixd }`, lineModelPara+lineModelPara, nil)
	for i, hl := range lines {
		if got := hl.Height + hl.Depth; got != bag.MustSP("12pt") {
			t.Errorf("line %d spans %s, want the built-in 12pt", i, got)
		}
	}
	if n := strings.Count(buf.String(), "names no registered line model"); n != 1 {
		t.Errorf("logged %d warnings, want 1:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "name=fixd") {
		t.Errorf("the warning does not name fixd:\n%s", buf.String())
	}
}

// Under a registered model a vertical-align shift reaches the model as the
// glyphs' line shift, and the glyphs move as they would without one.
func TestVerticalAlignIsALineShiftUnderARegisteredModel(t *testing.T) {
	register := func(cb *CSSBuilder) {
		_ = cb.RegisterLineModel("fixed", func(LineModelStyles) node.LineModel {
			var s bag.ScaledPoint
			return fixedModel{&s}
		})
	}
	for _, tc := range []struct {
		html  string
		glyph string
		want  bag.ScaledPoint
	}{
		{`<span style="vertical-align: 3pt">b</span>`, "b", bag.MustSP("3pt")},
		{`<span style="vertical-align: -2pt">b</span>`, "b", bag.MustSP("-2pt")},
		{`<span style="vertical-align: super">b</span>`, "b", bag.MustSP("10pt") / 3},
		{`<span style="vertical-align: sub">b</span>`, "b", -bag.MustSP("10pt") / 5},
		// Shifts that add up to zero must not inherit the parent's line shift.
		{`<span style="vertical-align: 3pt">b<span style="vertical-align: -3pt">c</span></span>`, "c", 0},
	} {
		html := `<p>a ` + tc.html + `</p>`
		for _, model := range []string{"fixed", "half", "trailing"} {
			var cfg func(*CSSBuilder)
			if model == "fixed" {
				cfg = register
			}
			line := lineModelLines(t, `p { -bag-leading-model: `+model+` }`, html, cfg)[0]
			var g *node.Glyph
			for n := line.List; n != nil; n = n.Next() {
				if gl, ok := n.(*node.Glyph); ok && gl.Components == tc.glyph {
					g = gl
				}
			}
			if g == nil {
				t.Errorf("%s, %s: no glyph %q", model, tc.html, tc.glyph)
				continue
			}
			wantShift := bag.ScaledPoint(0)
			if model == "fixed" {
				wantShift = tc.want
			}
			if g.YOffset != tc.want {
				t.Errorf("%s, %s: glyph YOffset %s, want %s", model, tc.html, g.YOffset, tc.want)
			}
			if g.LineShift != wantShift {
				t.Errorf("%s, %s: glyph LineShift %s, want %s", model, tc.html, g.LineShift, wantShift)
			}
		}
	}
}

// A footnote call's rise replaces the shift around it, with or without a
// registered model.
func TestAFootnoteCallDropsTheLineShift(t *testing.T) {
	cb, _ := newLineModelBuilder(t, "", nil)
	parent := frontend.TypesettingSettings{
		frontend.SettingSize:      bag.MustSP("10pt"),
		frontend.SettingYOffset:   bag.ScaledPoint(0),
		frontend.SettingLineShift: bag.MustSP("3pt"),
	}
	call := cb.makeFootnoteCall(parent, 1)
	if _, ok := call.Settings[frontend.SettingLineShift]; ok {
		t.Error("the footnote call keeps the surrounding line shift")
	}
	if _, ok := parent[frontend.SettingLineShift]; !ok {
		t.Error("the parent's settings lost their line shift")
	}
}

func renderLineModelPDF(t *testing.T, css, html string, configure func(*CSSBuilder)) []byte {
	t.Helper()
	var out bytes.Buffer
	fe, err := frontend.NewForWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	fe.SetSuppressInfo(true)
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if configure != nil {
		configure(cb)
	}
	if err = cb.AddCSS(css); err != nil {
		t.Fatal(err)
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal(err)
	}
	if err = cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err = fe.Doc.Finish(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A document that names no registered model renders byte for byte as it did
// without any registered, whatever the builder holds.
func TestRegisteredLineModelsLeaveOtherDocumentsAlone(t *testing.T) {
	css := `p { -bag-leading-model: trailing } .h { -bag-leading-model: half }`
	html := `<p>plain <sup>2</sup> and <span style="vertical-align: 2pt">up</span></p><p class="h">` + strings.Repeat("words ", 30) + `</p>`
	plain := renderLineModelPDF(t, css, html, nil)
	registered := renderLineModelPDF(t, css, html, func(cb *CSSBuilder) {
		_ = cb.RegisterLineModel("fixed", func(LineModelStyles) node.LineModel {
			var s bag.ScaledPoint
			return fixedModel{&s}
		})
	})
	if len(plain) == 0 || !bytes.Equal(plain, registered) {
		t.Error("registering a line model changed a document that does not name it")
	}
}

// strutModel sets a line from the ascent and descent of the fonts on it, and
// a line without glyphs from those of the paragraph's font, its strut.
type strutModel struct {
	strut *font.Font
}

func (m strutModel) LineBox(hl *node.HList, _ *node.LinebreakSettings) (bag.ScaledPoint, bag.ScaledPoint) {
	var h, d bag.ScaledPoint
	glyphs := false
	for n := hl.List; n != nil; n = n.Next() {
		if g, ok := n.(*node.Glyph); ok && g.Font != nil {
			h, d = max(h, g.Font.Ascent), max(d, g.Font.Descent)
			glyphs = true
		}
	}
	if !glyphs && m.strut != nil {
		h, d = m.strut.Ascent, m.strut.Descent
	}
	return h, d
}

func (strutModel) Leading(*node.HList, *node.LinebreakSettings) *node.Glue { return nil }

// LineModelStyles.Font lets a model give a line without glyphs, the one
// between two <br>, the height of the paragraph's font (CSS 2.1 §10.8.1).
func TestLineModelStylesFontIsTheStrut(t *testing.T) {
	register := func(cb *CSSBuilder) {
		if err := cb.RegisterLineModel("strut", func(s LineModelStyles) node.LineModel {
			return strutModel{s.Font}
		}); err != nil {
			t.Fatal(err)
		}
	}
	lines := lineModelLines(t, `p { -bag-leading-model: strut; font-size: 11pt; line-height: 1.15 }`,
		`<p>Text<br><br>Text</p>`, register)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, hl := range lines {
		if hl.Height != lines[0].Height || hl.Depth != lines[0].Depth || hl.Height == 0 {
			t.Errorf("line %d is %s + %s, want the text lines' %s + %s", i+1, hl.Height, hl.Depth, lines[0].Height, lines[0].Depth)
		}
	}
}

// keepModel keeps the LineModelStyles.Font it was made with and sets lines
// as the built-in leading would not, which is enough to be called.
type keepModel struct{ fixedModel }

// strutFont builds the paragraph's font by the frontend's rules, which are
// unexported: LineModelStyles.Font has the face, size and vertical metrics of
// the glyphs' own font, for a plain face, a size-adjusted one and one with
// metric overrides, so that a change on either side shows here.
func TestLineModelStylesFontMatchesTheGlyphs(t *testing.T) {
	const src = `src: url("fontsource/crimsonpro/CrimsonPro-Regular.ttf");`
	cases := map[string]string{
		"plain":       ``,
		"size-adjust": `size-adjust: 120%;`,
		"ascent":      `ascent-override: 90%;`,
		"descent":     `descent-override: 30%;`,
		"line gap":    `line-gap-override: 10%;`,
		"all of them": `size-adjust: 120%; ascent-override: 90%; descent-override: 30%; line-gap-override: 10%;`,
	}
	for name, descriptors := range cases {
		t.Run(name, func(t *testing.T) {
			var kept []*font.Font
			var shift bag.ScaledPoint
			register := func(cb *CSSBuilder) {
				if err := cb.RegisterLineModel("keep", func(s LineModelStyles) node.LineModel {
					kept = append(kept, s.Font)
					return keepModel{fixedModel{&shift}}
				}); err != nil {
					t.Fatal(err)
				}
			}
			css := `@font-face { font-family: "Probe"; ` + src + ` ` + descriptors + ` }
p { font-family: "Probe"; font-size: 11pt; -bag-leading-model: keep }`
			lines := lineModelLines(t, css, `<p>Text</p>`, register)
			var g *node.Glyph
			for n := lines[0].List; n != nil && g == nil; n = n.Next() {
				g, _ = n.(*node.Glyph)
			}
			if g == nil || g.Font == nil {
				t.Fatal("no glyph on the first line")
			}
			if len(kept) == 0 || kept[0] == nil {
				t.Fatal("the model got no font")
			}
			s, want := kept[0], g.Font
			// The face is the probe's, with its descriptors applied.
			if adjusted := strings.Contains(descriptors, "size-adjust"); adjusted == (want.Size == bag.MustSP("11pt")) {
				t.Fatalf("the glyphs are set at %s, the @font-face did not apply", want.Size)
			}
			if strings.Contains(descriptors, "ascent-override") && want.Ascent != bag.ScaledPointFromFloat(want.Size.ToPT()*0.9) {
				t.Fatalf("the glyphs' ascent is %s at %s, the override did not apply", want.Ascent, want.Size)
			}
			if s.Face != want.Face || s.Size != want.Size || s.Ascent != want.Ascent || s.Descent != want.Descent || s.LineGap != want.LineGap {
				t.Errorf("strut font: face %p, size %s, ascent %s, descent %s, line gap %s; glyph font: face %p, size %s, ascent %s, descent %s, line gap %s",
					s.Face, s.Size, s.Ascent, s.Descent, s.LineGap, want.Face, want.Size, want.Ascent, want.Descent, want.LineGap)
			}
			if s.ContentAscent != want.ContentAscent || s.ContentDescent != want.ContentDescent {
				t.Errorf("strut font content area %s + %s, glyph font %s + %s", s.ContentAscent, s.ContentDescent, want.ContentAscent, want.ContentDescent)
			}
		})
	}
}

// Under font-synthesis-style the paragraph's font is a slanted upright where
// the family has no italic: LineModelStyles.Font is that synthetic oblique too,
// and the missing italic is said once, for the glyphs, not again for the strut.
func TestLineModelStylesFontFollowsStyleSynthesis(t *testing.T) {
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()
	var kept []*font.Font
	var shift bag.ScaledPoint
	register := func(cb *CSSBuilder) {
		if err := cb.RegisterLineModel("keep", func(s LineModelStyles) node.LineModel {
			kept = append(kept, s.Font)
			return keepModel{fixedModel{&shift}}
		}); err != nil {
			t.Fatal(err)
		}
	}
	css := `@font-face { font-family: "Upright"; src: url("fontsource/crimsonpro/CrimsonPro-Regular.ttf"); }
p { font-family: "Upright"; font-size: 11pt; font-style: italic; font-synthesis-style: auto; -bag-leading-model: keep }`
	lines := lineModelLines(t, css, `<p>Text</p>`, register)
	var g *node.Glyph
	for n := lines[0].List; n != nil && g == nil; n = n.Next() {
		g, _ = n.(*node.Glyph)
	}
	if g == nil || g.Font == nil || g.Font.Slant == 0 {
		t.Fatal("the glyphs are not set in a synthetic oblique")
	}
	if len(kept) == 0 || kept[0] == nil || kept[0].Slant != g.Font.Slant {
		t.Errorf("the strut font is not the glyphs' oblique: %+v", kept)
	}
	if n := strings.Count(buf.String(), "not found in font family"); n != 1 {
		t.Errorf("the missing italic was said %d times, want once:\n%s", n, buf.String())
	}
}
