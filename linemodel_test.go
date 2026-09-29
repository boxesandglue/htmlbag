package htmlbag

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
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
	if got[0] != want {
		t.Errorf("the function got %+v, want %+v", got[0], want)
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
		va   string
		want bag.ScaledPoint
	}{
		{"3pt", bag.MustSP("3pt")},
		{"-2pt", bag.MustSP("-2pt")},
		{"super", bag.MustSP("10pt") / 3},
		{"sub", -bag.MustSP("10pt") / 5},
	} {
		html := `<p>a <span style="vertical-align: ` + tc.va + `">b</span></p>`
		for _, model := range []string{"fixed", "half", "trailing"} {
			var cfg func(*CSSBuilder)
			if model == "fixed" {
				cfg = register
			}
			line := lineModelLines(t, `p { -bag-leading-model: `+model+` }`, html, cfg)[0]
			var g *node.Glyph
			for n := line.List; n != nil; n = n.Next() {
				if gl, ok := n.(*node.Glyph); ok && gl.Codepoint != 0 && gl.YOffset != 0 {
					g = gl
				}
			}
			if g == nil {
				t.Errorf("%s, %s: no glyph is shifted", model, tc.va)
				continue
			}
			wantShift := bag.ScaledPoint(0)
			if model == "fixed" {
				wantShift = g.YOffset
			}
			if g.YOffset != tc.want {
				t.Errorf("%s, %s: glyph YOffset %s, want %s", model, tc.va, g.YOffset, tc.want)
			}
			if g.LineShift != wantShift {
				t.Errorf("%s, %s: glyph LineShift %s, want %s", model, tc.va, g.LineShift, wantShift)
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
