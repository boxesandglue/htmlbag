//go:generate go run ./internal/genfonts

package htmlbag

import (
	"os"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag/fonts/camingocodebold"
	"github.com/boxesandglue/htmlbag/fonts/camingocodebolditalic"
	"github.com/boxesandglue/htmlbag/fonts/camingocodeitalic"
	"github.com/boxesandglue/htmlbag/fonts/camingocoderegular"
	"github.com/boxesandglue/htmlbag/fonts/crimsonprobold"
	"github.com/boxesandglue/htmlbag/fonts/crimsonprobolditalic"
	"github.com/boxesandglue/htmlbag/fonts/crimsonproitalic"
	"github.com/boxesandglue/htmlbag/fonts/crimsonproregular"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherosbold"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherosbolditalic"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherositalic"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherosregular"
)

var (
	tenpoint    = bag.MustSP("10pt")
	twelvepoint = bag.MustSP("12pt")
)

// LoadIncludedFonts creates the font families monospace, sans and serif for
// default fonts.
func LoadIncludedFonts(fe *frontend.Document) error {
	var err error
	monospace := fe.NewFontFamily("monospace")
	if err = monospace.AddMember(&frontend.FontSource{Data: camingocoderegular.TTF, Name: "CamingoCode Regular"}, 400, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = monospace.AddMember(&frontend.FontSource{Data: camingocodebold.TTF, Name: "CamingoCode Bold"}, 700, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = monospace.AddMember(&frontend.FontSource{Data: camingocodeitalic.TTF, Name: "CamingoCode Italic"}, 400, frontend.FontStyleItalic); err != nil {
		return err
	}
	if err = monospace.AddMember(&frontend.FontSource{Data: camingocodebolditalic.TTF, Name: "CamingoCode Bold Italic"}, 700, frontend.FontStyleItalic); err != nil {
		return err
	}

	sans := fe.NewFontFamily("sans")
	if err = sans.AddMember(&frontend.FontSource{Data: texgyreherosregular.TTF, Name: "TeXGyreHeros Regular"}, 400, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = sans.AddMember(&frontend.FontSource{Data: texgyreherosbold.TTF, Name: "TeXGyreHeros Bold"}, 700, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = sans.AddMember(&frontend.FontSource{Data: texgyreherositalic.TTF, Name: "TeXGyreHeros Italic"}, 400, frontend.FontStyleItalic); err != nil {
		return err
	}
	if err = sans.AddMember(&frontend.FontSource{Data: texgyreherosbolditalic.TTF, Name: "TeXGyreHeros BoldItalic"}, 700, frontend.FontStyleItalic); err != nil {
		return err
	}
	serif := fe.NewFontFamily("serif")
	if err = serif.AddMember(&frontend.FontSource{Data: crimsonproregular.TTF, Name: "CrimsonPro Regular"}, 400, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = serif.AddMember(&frontend.FontSource{Data: crimsonprobold.TTF, Name: "CrimsonPro Bold"}, 700, frontend.FontStyleNormal); err != nil {
		return err
	}
	if err = serif.AddMember(&frontend.FontSource{Data: crimsonproitalic.TTF, Name: "CrimsonPro Italic"}, 400, frontend.FontStyleItalic); err != nil {
		return err
	}
	if err = serif.AddMember(&frontend.FontSource{Data: crimsonprobolditalic.TTF, Name: "CrimsonPro BoldItalic"}, 700, frontend.FontStyleItalic); err != nil {
		return err
	}
	return nil
}

// AddFontFamiliesFromCSS adds entries to the font families of the frontend
// document. Called again for the same document, it adds only the @font-face
// rules parsed since: rules of one family with the same weight and style
// combine into a composite face (unicode-range), so a rule added twice would
// count twice.
func AddFontFamiliesFromCSS(cs *CSS, fe *frontend.Document) error {
	done := cs.fontFacesAdded[fe]
	if done > len(cs.FontFaces) {
		done = 0
	}
	if cs.fontFacesAdded == nil {
		cs.fontFacesAdded = make(map[*frontend.Document]int)
	}
	cs.fontFacesAdded[fe] = len(cs.FontFaces)
	for _, v := range cs.FontFaces[done:] {
		var fontfamily *frontend.FontFamily
		if ff := fe.FindFontFamily(v.Family); ff == nil {
			fontfamily = fe.NewFontFamily(v.Family)
		} else {
			fontfamily = ff
		}
		fs, ok := fontSourceFromCSS(v.Source, fe)
		if !ok {
			bag.Logger.Warn("@font-face: no usable source in src, face ignored", "family", v.Family)
			continue
		}
		fs.FontFeatures = v.Features
		fs.VariationSettings = v.VariationSettings
		fs.UnicodeRange = v.UnicodeRange
		// CSS size-adjust (the parser stores 1 - percentage/100, the same
		// encoding FontSource expects); applied at shape time via
		// frontend.shapeFontFor.
		fs.SizeAdjust = v.SizeAdjust
		if v.AscentOverride >= 0 || v.DescentOverride >= 0 || v.LineGapOverride >= 0 {
			fs.Metrics = &frontend.MetricsOverride{Ascent: v.AscentOverride, Descent: v.DescentOverride, LineGap: v.LineGapOverride}
			bag.Logger.Debug("@font-face metric overrides do not affect the layout yet", "family", v.Family)
		}
		bag.Logger.Debug("AddFontFamiliesFromCSS", "family", v.Family, "variations", fs.VariationSettings)
		var fontstyle frontend.FontStyle
		switch v.Style {
		case "", "normal":
			fontstyle = frontend.FontStyleNormal
		case "italic":
			fontstyle = frontend.FontStyleItalic
		default:
			panic("nyi" + v.Style)
		}
		if v.WeightMax > v.Weight {
			// CSS Fonts 4 weight range (`font-weight: 200 900`), typically
			// a variable font: matched by containment, the used weight pins
			// the wght axis.
			fontfamily.AddMemberRange(fs, frontend.FontWeight(v.Weight), frontend.FontWeight(v.WeightMax), fontstyle)
		} else {
			fontfamily.AddMember(fs, frontend.FontWeight(v.Weight), fontstyle)
		}
	}
	return nil
}

// fontSourceFromCSS returns the font source for the first entry of an
// @font-face src list that can be used, as a browser takes the first it can
// load: an entry whose format() and tech() are supported, and whose file
// exists or whose local() name is a font the document knows. When no entry
// is available, the first supported one is returned so loading it reports
// the error. ok is false when no entry is supported at all.
func fontSourceFromCSS(srcs []FontSource, fe *frontend.Document) (*frontend.FontSource, bool) {
	var first *frontend.FontSource
	for _, src := range srcs {
		if !supportedFontFormat(src.Format) || !supportedFontTech(src.Tech) {
			continue
		}
		fs := &frontend.FontSource{}
		available := false
		switch {
		case src.URI != "":
			fs.Location = src.URI
			fs.Name = src.URI
			if strings.Contains(src.URI, "://") {
				available = true
			} else if _, err := os.Stat(src.URI); err == nil {
				available = true
			}
		case src.Local != "":
			fs.Name = src.Local
			if src.Local == "CamingoCode Regular" {
				fs.Data = camingocoderegular.TTF
			} else if err := fe.AddDataToFontsource(fs, src.Local); err != nil {
				fs.Data = nil
			}
			available = fs.Data != nil
		default:
			continue
		}
		if available {
			return fs, true
		}
		if first == nil {
			first = fs
		}
	}
	return first, first != nil
}

// supportedFontFormat reports whether format, the argument of an @font-face
// src format(), names a font file format the font loader reads. An entry
// without format() is tried.
func supportedFontFormat(format string) bool {
	format = strings.ToLower(strings.Trim(format, `"' `))
	// CSS Fonts 4 dropped the "-variations" forms, but stylesheets still
	// carry them.
	format = strings.TrimSuffix(format, "-variations")
	switch format {
	case "", "woff", "woff2", "truetype", "opentype", "collection":
		return true
	}
	return false
}

// supportedFontTech reports whether every technology in tech, the argument
// of an @font-face src tech(), is one the typesetter handles. An entry
// without tech() is tried.
func supportedFontTech(tech string) bool {
	for t := range strings.SplitSeq(tech, ",") {
		switch strings.ToLower(strings.Trim(t, `"' `)) {
		case "", "features-opentype", "features-graphite", "variations", "color-colrv0":
		default:
			return false
		}
	}
	return true
}
