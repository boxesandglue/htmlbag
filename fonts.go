package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/speedata/htmlbag/fonts/camingocodebold"
	"github.com/speedata/htmlbag/fonts/camingocodebolditalic"
	"github.com/speedata/htmlbag/fonts/camingocodeitalic"
	"github.com/speedata/htmlbag/fonts/camingocoderegular"
	"github.com/speedata/htmlbag/fonts/crimsonprobold"
	"github.com/speedata/htmlbag/fonts/crimsonprobolditalic"
	"github.com/speedata/htmlbag/fonts/crimsonproitalic"
	"github.com/speedata/htmlbag/fonts/crimsonproregular"
	"github.com/speedata/htmlbag/fonts/texgyreherosbold"
	"github.com/speedata/htmlbag/fonts/texgyreherosbolditalic"
	"github.com/speedata/htmlbag/fonts/texgyreherositalic"
	"github.com/speedata/htmlbag/fonts/texgyreherosregular"
)

var (
	tenpoint    = bag.MustSp("10pt")
	twelvepoint = bag.MustSp("12pt")
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
