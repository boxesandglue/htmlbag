package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/textshape/ot"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// smallCapsScale is the size of a synthesised small capital as a fraction of
// the font size. CSS leaves it to the user agent; Chromium and Firefox use 0.7.
const smallCapsScale = 0.7

// faceHasSmcp reports whether the face sty's text is set in has small capitals
// of its own (the OpenType smcp feature).
func (cb *CSSBuilder) faceHasSmcp(df *frontend.Document, sty *FormattingStyles) bool {
	if sty.fontfamily == nil {
		return false
	}
	fs, err := sty.fontfamily.GetFontSource(sty.Fontweight, sty.fontstyle)
	if err != nil || fs == nil {
		return false
	}
	if has, ok := cb.smcpFaces[fs]; ok {
		return has
	}
	has := false
	if face, err := df.LoadFace(fs); err == nil && face.OTFace() != nil {
		if data, err := face.OTFace().Font.TableData(ot.TagGSUB); err == nil {
			if gsub, err := ot.ParseGSUB(data); err == nil {
				if fl, err := gsub.ParseFeatureList(); err == nil {
					has = len(fl.FindFeature(ot.MakeTag('s', 'm', 'c', 'p'))) > 0
				}
			}
		}
	}
	if cb.smcpFaces == nil {
		cb.smcpFaces = map[*frontend.FontSource]bool{}
	}
	cb.smcpFaces[fs] = has
	return has
}

// smallCapsItems is text s in small capitals (CSS Fonts 4 §6.4): the face's
// own through smcp when it has them, else synthesised, each lowercase letter
// as its capital at a reduced size. The capitals follow the full, language
// sensitive case mapping, so ß becomes SS and a Turkish i becomes İ.
// The synthesised capitals are what PDF text extraction sees; an /ActualText
// span in bag carrying the original string would fix that.
func (cb *CSSBuilder) smallCapsItems(df *frontend.Document, sty *FormattingStyles, s string) []any {
	if cb.faceHasSmcp(df, sty) {
		t := frontend.NewText()
		t.Settings[frontend.SettingOpenTypeFeature] = append(append([]string(nil), sty.fontfeatures...), "smcp=1")
		t.Items = []any{s}
		return []any{t}
	}
	if sty.smallCapsNoSynth {
		return []any{s}
	}
	small := bag.ScaledPoint(float64(sty.Fontsize) * smallCapsScale)
	upper := cases.Upper(language.Make(sty.language))
	shrinks := func(r rune) bool { return upper.String(string(r)) != string(r) }
	type piece struct {
		s     string
		small bool
	}
	var pieces []piece
	for _, r := range s {
		sm := shrinks(r)
		if n := len(pieces); n > 0 && pieces[n-1].small == sm {
			pieces[n-1].s += string(r)
		} else {
			pieces = append(pieces, piece{string(r), sm})
		}
	}
	if len(pieces) == 1 && !pieces[0].small {
		return []any{s}
	}
	// Each piece is a text of its own, and each string in a text gets the
	// inline padding; only the outer edges keep it.
	out := make([]any, len(pieces))
	for i, p := range pieces {
		t := frontend.NewText()
		if i > 0 {
			t.Settings[frontend.SettingPaddingLeft] = bag.ScaledPoint(0)
		}
		if i < len(pieces)-1 {
			t.Settings[frontend.SettingPaddingRight] = bag.ScaledPoint(0)
		}
		if p.small {
			t.Settings[frontend.SettingSize] = small
			p.s = upper.String(p.s)
		}
		t.Items = []any{p.s}
		out[i] = t
	}
	return out
}
