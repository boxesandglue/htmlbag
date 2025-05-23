package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
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
// document.
func AddFontFamiliesFromCSS(cs *csshtml.CSS, fe *frontend.Document) error {
	for _, v := range cs.FontFaces {
		var fontfamily *frontend.FontFamily
		if ff := fe.FindFontFamily(v.Family); ff == nil {
			fontfamily = fe.NewFontFamily(v.Family)
		} else {
			fontfamily = ff
		}
		fs := &frontend.FontSource{}

		for _, src := range v.Source {
			if l := src.URI; l != "" {
				fs.Location = l
				fs.Name = l

			} else if l := src.Local; l != "" {
				fs.Name = l
				switch l {
				case "CamingoCode Regular":
					fs.Data = camingocoderegular.TTF
				}
			}
			fs.FontFeatures = v.Features
		}
		var fontstyle frontend.FontStyle
		switch v.Style {
		case "", "normal":
			fontstyle = frontend.FontStyleNormal
		case "italic":
			fontstyle = frontend.FontStyleItalic
		default:
			panic("nyi" + v.Style)
		}
		fontfamily.AddMember(fs, frontend.FontWeight(v.Weight), fontstyle)
	}
	return nil
}
