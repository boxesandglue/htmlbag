package htmlbag

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/lang"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
	"github.com/boxesandglue/svgreader"
	"golang.org/x/net/html"
)

var (
	tenpt    = bag.MustSP("10pt")
	tenptflt = bag.MustSP("10pt").ToPT()
)

// settingPageBreakInside is an htmlbag-private frontend.SettingType sentinel
// used to stash the CSS page-break-inside / break-inside value on a
// frontend.Text's Settings map so it survives the Text → VList/row
// materialization pipeline. Upstream frontend.SettingType constants are
// assigned via positive iota; a negative value cannot collide with any
// current or future upstream constant, which avoids widening the external
// frontend API for an htmlbag-internal concern. The sentinel is never
// emitted outside htmlbag: buildVlistInternal and buildTable read it,
// copy the value onto the resulting node's Attributes["pageBreakInside"],
// and delete the sentinel from the source Text's Settings so it cannot
// leak into frontend.FormatParagraph (whose setting-type switch has a
// strict "unknown setting" default that would otherwise error).
const settingPageBreakInside frontend.SettingType = -1

// ParseVerticalAlign parses the input ("top","middle",...) and returns the
// VerticalAlignment value.
func ParseVerticalAlign(align string, styles *FormattingStyles) frontend.VerticalAlignment {
	switch align {
	case "top":
		return frontend.VAlignTop
	case "middle":
		return frontend.VAlignMiddle
	case "bottom":
		return frontend.VAlignBottom
	case "inherit":
		return styles.Valign
	default:
		return styles.Valign
	}
}

// ParseHorizontalAlign parses the input ("left","center") and returns the
// HorizontalAlignment value. CSS Text 3 §7 logical keywords "start" and "end"
// stay logical here; FormatParagraph resolves them to physical Left/Right
// after paragraph direction is known.
func ParseHorizontalAlign(align string, styles *FormattingStyles) frontend.HorizontalAlignment {
	switch align {
	case "left":
		return frontend.HAlignLeft
	case "center":
		return frontend.HAlignCenter
	case "right":
		return frontend.HAlignRight
	case "justify":
		return frontend.HAlignJustified
	case "start":
		return frontend.HAlignStart
	case "end":
		return frontend.HAlignEnd
	case "inherit":
		return styles.Halign
	default:
		return styles.Halign
	}
}

// ParseRelativeSize converts the string fs to a scaled point. This can be an
// absolute size like 12pt but also a size like 1.2 or 2em. The provided dflt is
// the source size. The root is the document's default value.
func ParseRelativeSize(fs string, cur bag.ScaledPoint, root bag.ScaledPoint) bag.ScaledPoint {
	if p, ok := strings.CutSuffix(fs, "%"); ok {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			panic(err)
		}
		ret := bag.MultiplyFloat(cur, f/100)
		return ret
	}
	if prefix, ok := strings.CutSuffix(fs, "rem"); ok {
		if root == 0 {
			// logger.Warn("Calculating an rem size without a root font size results in a size of 0.")
			return 0
		}
		factor, err := strconv.ParseFloat(prefix, 32)
		if err != nil {
			// logger.Error(fmt.Sprintf("Cannot convert relative size %s", fs))
			return bag.MustSP("10pt")
		}
		return bag.ScaledPoint(float64(root) * factor)
	}
	if prefix, ok := strings.CutSuffix(fs, "em"); ok {
		if cur == 0 {
			// logger.Warn("Calculating an em size without a body font size results in a size of 0.")
			return 0
		}
		factor, err := strconv.ParseFloat(prefix, 32)
		if err != nil {
			// logger.Error(fmt.Sprintf("Cannot convert relative size %s", fs))
			return bag.MustSP("10pt")
		}
		return bag.ScaledPoint(float64(cur) * factor)
	}
	if unit, err := bag.SP(fs); err == nil {
		return unit
	}
	if factor, err := strconv.ParseFloat(fs, 64); err == nil {
		return bag.ScaledPointFromFloat(cur.ToPT() * factor)
	}
	switch fs {
	case "larger":
		return bag.ScaledPointFromFloat(cur.ToPT() * 1.2)
	case "smaller":
		return bag.ScaledPointFromFloat(cur.ToPT() / 1.2)
	case "xx-small":
		return bag.ScaledPointFromFloat(tenptflt / 1.2 / 1.2 / 1.2)
	case "x-small":
		return bag.ScaledPointFromFloat(tenptflt / 1.2 / 1.2)
	case "small":
		return bag.ScaledPointFromFloat(tenptflt / 1.2)
	case "medium":
		return tenpt
	case "large":
		return bag.ScaledPointFromFloat(tenptflt * 1.2)
	case "x-large":
		return bag.ScaledPointFromFloat(tenptflt * 1.2 * 1.2)
	case "xx-large":
		return bag.ScaledPointFromFloat(tenptflt * 1.2 * 1.2 * 1.2)
	case "xxx-large":
		return bag.ScaledPointFromFloat(tenptflt * 1.2 * 1.2 * 1.2 * 1.2)
	}
	// logger.Error(fmt.Sprintf("Could not convert %s from default %s", fs, cur))
	return cur
}

// cssGenericFontFamilyAliases maps CSS Fonts 4 generic-family keywords to the
// internal htmlbag family names. The codebase registers "sans" / "serif" /
// "monospace" (htmlbag/fonts.go); CSS uses "sans-serif" as the spec keyword,
// so a verbatim FindFontFamily lookup misses. Only sans-serif needs aliasing
// — "serif" and "monospace" already match by identity, and the remaining
// generics (cursive, fantasy, system-ui, …) are not registered, so they fall
// through to the surrounding fallback.
var cssGenericFontFamilyAliases = map[string]string{
	"sans-serif": "sans",
}

// resolveCSSFontFamily parses a CSS font-family value (a comma-separated
// prioritised list per CSS Fonts 4 §3.1) and returns the first family that
// resolves against the document's registered families. Each candidate is
// trimmed of surrounding whitespace and CSS string quotes, then looked up
// directly first and via the generic-keyword alias table if the direct
// lookup misses. Returns nil if no candidate resolves — the caller decides
// the fallback.
func resolveCSSFontFamily(v string, df *frontend.Document) *frontend.FontFamily {
	for _, part := range strings.Split(v, ",") {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, `"'`)
		if name == "" {
			continue
		}
		if ff := df.FindFontFamily(name); ff != nil {
			return ff
		}
		if alias, ok := cssGenericFontFamilyAliases[name]; ok {
			if ff := df.FindFontFamily(alias); ff != nil {
				return ff
			}
		}
	}
	return nil
}

// StylesToStyles updates the inheritable formattingStyles from the attributes
// (of the current HTML element).
func StylesToStyles(ih *FormattingStyles, attributes map[string]string, df *frontend.Document, curFontSize bag.ScaledPoint) error {
	// Resolve font size first, since some of the attributes depend on the
	// current font size.
	if v, ok := attributes["font-size"]; ok {
		ih.Fontsize = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
	}
	for k, v := range attributes {
		switch k {
		case "font-size":
			// already set
		case "hyphens":
			// CSS Text 3 §6: "none" suppresses automatic and soft-hyphen
			// breaks; "manual" allows only soft-hyphen (U+00AD) breaks;
			// "auto" lets the UA hyphenate per language patterns. We carry
			// the keyword as-is and translate to the no-op language at
			// ApplySettings when it's "none" or "manual".
			ih.hyphens = strings.ToLower(strings.TrimSpace(v))
		case "-bag-linebreak-hyphen-penalty":
			// boxesandglue-specific: Knuth-Plass hyphen penalty (int).
			// Lower values encourage hyphenation.
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				ih.hyphenPenalty = n
			}
		case "-bag-linebreak-tolerance":
			// boxesandglue-specific: Knuth-Plass tolerance (float).
			// Higher values allow looser lines.
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				ih.linebreakTolerance = f
			}
		case "display":
			ih.Hide = (v == "none")
		case "background-color":
			ih.BackgroundColor = df.GetColor(v)
		case "border-right-width", "border-left-width", "border-top-width", "border-bottom-width":
			size := ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
			switch k {
			case "border-right-width":
				ih.BorderRightWidth = size
			case "border-left-width":
				ih.BorderLeftWidth = size
			case "border-top-width":
				ih.BorderTopWidth = size
			case "border-bottom-width":
				ih.BorderBottomWidth = size
			}
		case "border-top-right-radius", "border-top-left-radius", "border-bottom-right-radius", "border-bottom-left-radius":
			size := ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
			switch k {
			case "border-top-right-radius":
				ih.BorderTopRightRadius = size
			case "border-top-left-radius":
				ih.BorderTopLeftRadius = size
			case "border-bottom-left-radius":
				ih.BorderBottomLeftRadius = size
			case "border-bottom-right-radius":
				ih.BorderBottomRightRadius = size
			}
		case "border-right-style", "border-left-style", "border-top-style", "border-bottom-style":
			var sty frontend.BorderStyle
			switch v {
			case "none":
				// default
			case "solid":
				sty = frontend.BorderStyleSolid
			default:
				// logger.Error(fmt.Sprintf("not implemented: border style %q", v))
			}
			switch k {
			case "border-right-style":
				ih.BorderRightStyle = sty
			case "border-left-style":
				ih.BorderLeftStyle = sty
			case "border-top-style":
				ih.BorderTopStyle = sty
			case "border-bottom-style":
				ih.BorderBottomStyle = sty
			}

		case "border-right-color":
			ih.BorderRightColor = df.GetColor(v)
		case "border-left-color":
			ih.BorderLeftColor = df.GetColor(v)
		case "border-top-color":
			ih.BorderTopColor = df.GetColor(v)
		case "border-bottom-color":
			ih.BorderBottomColor = df.GetColor(v)
		case "border-spacing":
			// ignore
		case "color":
			ih.color = df.GetColor(v)
		case "content":
			// Check for leader() function: leader('.') or leader(".")
			if strings.HasPrefix(v, "leader(") && strings.HasSuffix(v, ")") {
				inner := v[7 : len(v)-1]
				inner = strings.TrimSpace(inner)
				inner = strings.Trim(inner, "'\"")
				if inner != "" {
					ih.leaderContent = inner
				}
			}
		case "font-style":
			switch v {
			case "italic":
				ih.fontstyle = frontend.FontStyleItalic
			case "normal":
				ih.fontstyle = frontend.FontStyleNormal
			}
		case "font-weight":
			ih.Fontweight = frontend.ResolveFontWeight(v, ih.Fontweight)
		case "font-feature-settings":
			ih.fontfeatures = append(ih.fontfeatures, v)
		case "font-variation-settings":
			// Parse CSS syntax: "wght" 700, "wdth" 100
			if ih.variationSettings == nil {
				ih.variationSettings = make(map[string]float64)
			}
			for _, pair := range strings.Split(v, ",") {
				pair = strings.TrimSpace(pair)
				parts := strings.Fields(pair)
				if len(parts) >= 2 {
					// Remove quotes from axis tag
					tag := strings.Trim(parts[0], `"'`)
					if val, err := strconv.ParseFloat(parts[1], 64); err == nil {
						ih.variationSettings[tag] = val
					}
				}
			}
		case "list-style-type":
			ih.ListStyleType = v
		case "font-family":
			ih.fontfamily = resolveCSSFontFamily(v, df)
			if ih.fontfamily == nil {
				bag.Logger.Error("Font family not found, reverting to 'serif'", "requested family", v)
				ih.fontfamily = df.FindFontFamily("serif")
			}
		case "hanging-punctuation":
			switch v {
			case "allow-end":
				ih.hangingPunctuation = frontend.HangingPunctuationAllowEnd
			}
		case "letter-spacing":
			if v != "normal" {
				ih.letterSpacing = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
			}
		case "line-height":
			if v == "normal" {
				ih.lineheight = 0
				ih.lineheightFactor = 1.2
			} else if factor, err := strconv.ParseFloat(v, 64); err == nil {
				// Unitless value like "1.5" — store as factor, inherit per element
				ih.lineheight = 0
				ih.lineheightFactor = factor
			} else {
				// Absolute value like "18pt", "1.5em", "150%"
				ih.lineheight = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
				ih.lineheightFactor = 0
			}
		case "margin-bottom":
			ih.marginBottom = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "margin-left":
			ih.marginLeft = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "margin-right":
			ih.marginRight = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "margin-top":
			ih.marginTop = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "page-break-after", "break-after":
			ih.pageBreakAfter = v
		case "page-break-before", "break-before":
			ih.pageBreakBefore = v
		case "page-break-inside", "break-inside":
			ih.pageBreakInside = v
		case "padding-inline-start":
			ih.paddingInlineStart = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "padding-bottom":
			ih.PaddingBottom = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "padding-left":
			ih.PaddingLeft = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "padding-right":
			ih.PaddingRight = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "padding-top":
			ih.PaddingTop = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
		case "tab-size":
			if ts, err := strconv.Atoi(v); err == nil {
				ih.tabsizeSpaces = ts
			} else {
				ih.tabsize = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
			}
		case "text-align":
			ih.Halign = ParseHorizontalAlign(v, ih)
		case "border-collapse":
			// handled by table builder
		case "text-decoration-style":
			// not yet implemented
		case "text-decoration-line":
			switch v {
			case "underline":
				ih.TextDecorationLine = frontend.TextDecorationUnderline
			}
		case "text-indent":
			ih.indent = ParseRelativeSize(v, curFontSize, ih.DefaultFontSize)
			ih.indentRows = 1
		case "user-select":
			// ignore
		case "counter-reset":
			// CSS Lists 3 §3.1: counter-reset is a list of one or more
			// "<name> [<integer>]?" pairs. Each pair creates a counter
			// in this element's scope. The integer is optional and
			// defaults to 0.
			ih.counterReset = parseCounterList(v, 0)
		case "counter-increment":
			// CSS Lists 3 §3.2: counter-increment increments the nearest
			// counter of the given name in the ancestor chain. The
			// integer is optional and defaults to 1.
			ih.counterIncrement = parseCounterList(v, 1)
		case "vertical-align":
			switch v {
			case "sub":
				ih.yoffset = -1 * ih.Fontsize * 1000 / 5000
			case "super":
				ih.yoffset = ih.Fontsize * 1000 / 5000
			case "top":
				ih.Valign = frontend.VAlignTop
			case "middle":
				ih.Valign = frontend.VAlignMiddle
			case "bottom":
				ih.Valign = frontend.VAlignBottom
			}
		case "width":
			ih.width = v
		case "white-space":
			ih.preserveWhitespace = (v == "pre")
		case "-bag-font-expansion":
			if strings.HasSuffix(v, "%") {
				p := strings.TrimSuffix(v, "%")
				f, err := strconv.ParseFloat(p, 64)
				if err != nil {
					return err
				}
				fe := f / 100
				ih.fontexpansion = &fe
			}
		default:
			slog.Debug("unresolved attribute", k, v)
		}
	}
	return nil
}

// FormattingStyles are HTML formatting styles.
type FormattingStyles struct {
	BackgroundColor         *color.Color
	BorderLeftWidth         bag.ScaledPoint
	BorderRightWidth        bag.ScaledPoint
	BorderBottomWidth       bag.ScaledPoint
	BorderTopWidth          bag.ScaledPoint
	BorderTopLeftRadius     bag.ScaledPoint
	BorderTopRightRadius    bag.ScaledPoint
	BorderBottomLeftRadius  bag.ScaledPoint
	BorderBottomRightRadius bag.ScaledPoint
	BorderLeftColor         *color.Color
	BorderRightColor        *color.Color
	BorderBottomColor       *color.Color
	BorderTopColor          *color.Color
	BorderLeftStyle         frontend.BorderStyle
	BorderRightStyle        frontend.BorderStyle
	BorderBottomStyle       frontend.BorderStyle
	BorderTopStyle          frontend.BorderStyle
	DefaultFontSize         bag.ScaledPoint
	DefaultFontFamily       *frontend.FontFamily
	color                   *color.Color
	Hide                    bool
	fontfamily              *frontend.FontFamily
	fontfeatures            []string
	variationSettings       map[string]float64 // axis tag -> value (e.g., "wght" -> 700)
	Fontsize                bag.ScaledPoint
	fontstyle               frontend.FontStyle
	Fontweight              frontend.FontWeight
	fontexpansion           *float64
	Halign                  frontend.HorizontalAlignment
	hangingPunctuation      frontend.HangingPunctuation
	hyphens                 string  // CSS hyphens: "" (auto), "auto", "manual", "none"
	hyphenPenalty           int     // -bag-linebreak-hyphen-penalty (0 = inherit/default)
	linebreakTolerance      float64 // -bag-linebreak-tolerance (0 = inherit/default)
	indent                  bag.ScaledPoint
	indentRows              int
	language                string     // BCP47 tag (e.g. "en", "ar", "de-DE")
	langPattern             *lang.Lang // resolved hyphenator for {language, hyphens}; nil = use parent / doc default
	letterSpacing           bag.ScaledPoint
	lineheight              bag.ScaledPoint
	lineheightFactor        float64 // unitless line-height factor (e.g. 1.2); recalculated per element
	ListStyleType           string
	marginBottom            bag.ScaledPoint
	marginLeft              bag.ScaledPoint
	marginRight             bag.ScaledPoint
	marginTop               bag.ScaledPoint
	paddingInlineStart      bag.ScaledPoint
	OlCounter               int
	// LocalCounters holds CSS counter values defined in this element's
	// scope. Children look up counter values by walking the StylesStack
	// from the top down, so siblings share counters declared on the
	// nearest common ancestor (e.g. <ol counter-reset: list-item> seen
	// from each <li counter-increment: list-item>).
	LocalCounters      map[string]int
	counterReset       map[string]int // CSS counter-reset on THIS element
	counterIncrement   map[string]int // CSS counter-increment on THIS element
	ListPaddingLeft    bag.ScaledPoint
	PaddingBottom      bag.ScaledPoint
	PaddingLeft        bag.ScaledPoint
	PaddingRight       bag.ScaledPoint
	PaddingTop         bag.ScaledPoint
	TextDecorationLine frontend.TextDecorationLine
	leaderContent      string
	preserveWhitespace bool
	tabsize            bag.ScaledPoint
	tabsizeSpaces      int
	Valign             frontend.VerticalAlignment
	width              string
	pageBreakAfter     string
	pageBreakBefore    string
	pageBreakInside    string
	yoffset            bag.ScaledPoint
}

// Clone mimics style inheritance.
func (is *FormattingStyles) Clone() *FormattingStyles {
	// inherit
	newFontFeatures := make([]string, len(is.fontfeatures))
	copy(newFontFeatures, is.fontfeatures)
	var newVariationSettings map[string]float64
	if is.variationSettings != nil {
		newVariationSettings = make(map[string]float64, len(is.variationSettings))
		for k, v := range is.variationSettings {
			newVariationSettings[k] = v
		}
	}
	newis := &FormattingStyles{
		BackgroundColor:    is.BackgroundColor,
		color:              is.color,
		DefaultFontSize:    is.DefaultFontSize,
		DefaultFontFamily:  is.DefaultFontFamily,
		fontexpansion:      is.fontexpansion,
		fontfamily:         is.fontfamily,
		fontfeatures:       newFontFeatures,
		variationSettings:  newVariationSettings,
		Fontsize:           is.Fontsize,
		fontstyle:          is.fontstyle,
		Fontweight:         is.Fontweight,
		hangingPunctuation: is.hangingPunctuation,
		hyphens:            is.hyphens,
		hyphenPenalty:      is.hyphenPenalty,
		linebreakTolerance: is.linebreakTolerance,
		language:           is.language,
		langPattern:        is.langPattern,
		letterSpacing:      is.letterSpacing,
		lineheight:         is.lineheight,
		lineheightFactor:   is.lineheightFactor,
		ListStyleType:      is.ListStyleType,
		ListPaddingLeft:    is.ListPaddingLeft,
		OlCounter:          is.OlCounter,
		preserveWhitespace: is.preserveWhitespace,
		tabsize:            is.tabsize,
		tabsizeSpaces:      is.tabsizeSpaces,
		Valign:             is.Valign,
		Halign:             is.Halign,
	}
	return newis
}

// noHyphenationKey is the cache key used for the document-wide no-op
// hyphenator. Any string that does not match a known BCP47 tag would do; this
// one is reserved enough to avoid colliding with a real language id.
const noHyphenationKey = "x-htmlbag-nohyphenation"

// applyLangAndHyphens reads HTML lang= / xml:lang= from item.Attributes and
// resolves the effective hyphenation language for ih. The resolution follows
// CSS Text 3 §6:
//
//   - hyphens: "none"   → no-op hyphenator (no breakpoints inserted)
//   - hyphens: "manual" → no-op hyphenator. Soft-hyphen (U+00AD) breaks are
//     created at glyph-build time, independent of patterns.
//   - hyphens: "" / "auto" → frontend.GetLanguage(language). Unknown tags
//     resolve to a no-op hyphenator (UA must not
//     hyphenate without matching patterns).
//
// HTML5 treats lang as the inheritable language tag; xml:lang is honoured as
// a fallback only when lang is missing.
func applyLangAndHyphens(ih *FormattingStyles, attrs map[string]string, df *frontend.Document) {
	if v, ok := attrs["lang"]; ok {
		ih.language = strings.TrimSpace(v)
	} else if v, ok := attrs["xml:lang"]; ok {
		ih.language = strings.TrimSpace(v)
	}
	switch ih.hyphens {
	case "none", "manual":
		l, err := df.GetLanguageCached(noHyphenationKey)
		if err != nil {
			bag.Logger.Error("Cannot create no-op hyphenator", "err", err)
			return
		}
		ih.langPattern = l
	default:
		// "auto" or empty — honour the language tag.
		if ih.language == "" {
			return
		}
		l, err := df.GetLanguageCached(ih.language)
		if err != nil {
			bag.Logger.Error("Cannot resolve language", "tag", ih.language, "err", err)
			return
		}
		ih.langPattern = l
	}
}

// ApplySettings converts the inheritable settings to boxes and glue text
// settings.
func ApplySettings(settings frontend.TypesettingSettings, ih *FormattingStyles) {
	if ih.Fontweight > 0 {
		settings[frontend.SettingFontWeight] = ih.Fontweight
	}
	settings[frontend.SettingBackgroundColor] = ih.BackgroundColor
	settings[frontend.SettingBorderTopWidth] = ih.BorderTopWidth
	settings[frontend.SettingBorderLeftWidth] = ih.BorderLeftWidth
	settings[frontend.SettingBorderRightWidth] = ih.BorderRightWidth
	settings[frontend.SettingBorderBottomWidth] = ih.BorderBottomWidth
	settings[frontend.SettingBorderTopColor] = ih.BorderTopColor
	settings[frontend.SettingBorderLeftColor] = ih.BorderLeftColor
	settings[frontend.SettingBorderRightColor] = ih.BorderRightColor
	settings[frontend.SettingBorderBottomColor] = ih.BorderBottomColor
	settings[frontend.SettingBorderTopStyle] = ih.BorderTopStyle
	settings[frontend.SettingBorderLeftStyle] = ih.BorderLeftStyle
	settings[frontend.SettingBorderRightStyle] = ih.BorderRightStyle
	settings[frontend.SettingBorderBottomStyle] = ih.BorderBottomStyle
	settings[frontend.SettingBorderTopLeftRadius] = ih.BorderTopLeftRadius
	settings[frontend.SettingBorderTopRightRadius] = ih.BorderTopRightRadius
	settings[frontend.SettingBorderBottomLeftRadius] = ih.BorderBottomLeftRadius
	settings[frontend.SettingBorderBottomRightRadius] = ih.BorderBottomRightRadius
	settings[frontend.SettingColor] = ih.color
	if ih.fontexpansion != nil {
		settings[frontend.SettingFontExpansion] = *ih.fontexpansion
	} else {
		settings[frontend.SettingFontExpansion] = 0.05
	}
	settings[frontend.SettingFontFamily] = ih.fontfamily
	settings[frontend.SettingHAlign] = ih.Halign
	settings[frontend.SettingHangingPunctuation] = ih.hangingPunctuation
	settings[frontend.SettingIndentLeft] = ih.indent
	settings[frontend.SettingIndentLeftRows] = ih.indentRows
	if ih.lineheightFactor != 0 {
		settings[frontend.SettingLeading] = bag.ScaledPoint(float64(ih.Fontsize) * ih.lineheightFactor)
	} else {
		settings[frontend.SettingLeading] = ih.lineheight
	}
	settings[frontend.SettingLetterSpacing] = ih.letterSpacing
	settings[frontend.SettingMarginBottom] = ih.marginBottom
	settings[frontend.SettingMarginRight] = ih.marginRight
	settings[frontend.SettingMarginLeft] = ih.marginLeft
	settings[frontend.SettingMarginTop] = ih.marginTop
	settings[frontend.SettingOpenTypeFeature] = ih.fontfeatures
	if ih.variationSettings != nil {
		settings[frontend.SettingFontVariationSettings] = ih.variationSettings
	}
	settings[frontend.SettingPaddingRight] = ih.PaddingRight
	settings[frontend.SettingPaddingLeft] = ih.PaddingLeft
	settings[frontend.SettingPaddingTop] = ih.PaddingTop
	settings[frontend.SettingPaddingBottom] = ih.PaddingBottom
	settings[frontend.SettingPreserveWhitespace] = ih.preserveWhitespace
	settings[frontend.SettingSize] = ih.Fontsize
	settings[frontend.SettingStyle] = ih.fontstyle
	settings[frontend.SettingYOffset] = ih.yoffset
	settings[frontend.SettingTabSize] = ih.tabsize
	settings[frontend.SettingTabSizeSpaces] = ih.tabsizeSpaces
	settings[frontend.SettingTextDecorationLine] = ih.TextDecorationLine

	if ih.Valign != frontend.VAlignDefault {
		settings[frontend.SettingVAlign] = ih.Valign
	}

	if ih.pageBreakAfter != "" {
		settings[frontend.SettingPageBreakAfter] = ih.pageBreakAfter
	}
	if ih.pageBreakBefore != "" {
		settings[frontend.SettingPageBreakBefore] = ih.pageBreakBefore
	}
	if ih.pageBreakInside != "" {
		settings[settingPageBreakInside] = ih.pageBreakInside
	}
	if ih.width != "" {
		settings[frontend.SettingWidth] = ih.width
	}
	if ih.leaderContent != "" {
		settings[frontend.SettingLeader] = ih.leaderContent
	}
	if ih.langPattern != nil {
		settings[frontend.SettingLanguage] = ih.langPattern
	}
	if ih.hyphens != "" {
		settings[frontend.SettingHyphens] = ih.hyphens
	}
	if ih.hyphenPenalty != 0 {
		settings[frontend.SettingHyphenPenalty] = ih.hyphenPenalty
	}
	if ih.linebreakTolerance != 0 {
		settings[frontend.SettingLinebreakTolerance] = ih.linebreakTolerance
	}
}

// parseCounterList parses a CSS counter-reset / counter-increment value
// like "section" or "section 1 sub 0" — a whitespace-separated list of
// names, each optionally followed by an integer. Names without a number
// take defaultValue (0 for counter-reset, 1 for counter-increment).
func parseCounterList(v string, defaultValue int) map[string]int {
	out := map[string]int{}
	fields := strings.Fields(v)
	i := 0
	for i < len(fields) {
		name := fields[i]
		i++
		val := defaultValue
		if i < len(fields) {
			if n, err := strconv.Atoi(fields[i]); err == nil {
				val = n
				i++
			}
		}
		out[name] = val
	}
	return out
}

// StylesStack mimics CSS style inheritance.
type StylesStack []*FormattingStyles

// applyCounters performs the per-element counter bookkeeping for the
// styles at the top of the stack. counter-reset creates a counter in
// the current scope; counter-increment finds the innermost counter of
// the given name on the ancestor chain (creating one at the parent
// scope when none exists, per CSS Lists 3 §3.2) and bumps it.
func (ss *StylesStack) applyCounters() {
	if len(*ss) == 0 {
		return
	}
	cur := (*ss)[len(*ss)-1]
	for name, n := range cur.counterReset {
		if cur.LocalCounters == nil {
			cur.LocalCounters = map[string]int{}
		}
		cur.LocalCounters[name] = n
	}
	for name, n := range cur.counterIncrement {
		// Walk up the stack to find an existing counter of this name.
		idx := -1
		for j := len(*ss) - 1; j >= 0; j-- {
			if _, ok := (*ss)[j].LocalCounters[name]; ok {
				idx = j
				break
			}
		}
		if idx == -1 {
			// Implicit reset at the parent (or root if no parent) per
			// the spec. We anchor it at the parent scope so siblings
			// share the counter; if we're already at root, anchor here.
			anchor := 0
			if len(*ss) >= 2 {
				anchor = len(*ss) - 2
			}
			if (*ss)[anchor].LocalCounters == nil {
				(*ss)[anchor].LocalCounters = map[string]int{}
			}
			(*ss)[anchor].LocalCounters[name] = 0
			idx = anchor
		}
		(*ss)[idx].LocalCounters[name] += n
	}
}

// CounterValue returns the value of the innermost counter with the given
// name (walking the stack top-down). Returns 0 when no such counter
// exists, matching the CSS fallback for counter(name).
func (ss StylesStack) CounterValue(name string) int {
	for i := len(ss) - 1; i >= 0; i-- {
		if v, ok := ss[i].LocalCounters[name]; ok {
			return v
		}
	}
	return 0
}

// CounterValues returns every counter with the given name along the
// ancestor chain, root-first. counters(name, sep) uses this for nested
// numbering like "2.1.1".
func (ss StylesStack) CounterValues(name string) []int {
	var out []int
	for i := 0; i < len(ss); i++ {
		if v, ok := ss[i].LocalCounters[name]; ok {
			out = append(out, v)
		}
	}
	return out
}

// PushStyles creates a new style instance, pushes it onto the stack and returns
// the new style.
func (ss *StylesStack) PushStyles() *FormattingStyles {
	var is *FormattingStyles
	if len(*ss) == 0 {
		is = &FormattingStyles{Halign: frontend.HAlignStart}
	} else {
		is = (*ss)[len(*ss)-1].Clone()
	}
	*ss = append(*ss, is)
	return is
}

// PopStyles removes the top style from the stack.
func (ss *StylesStack) PopStyles() {
	*ss = (*ss)[:len(*ss)-1]
}

// CurrentStyle returns the current style from the stack. CurrentStyle does not
// change the stack.
func (ss StylesStack) CurrentStyle() *FormattingStyles {
	return ss[len(ss)-1]
}

// SetDefaultFontFamily sets the font family that should be used as a default
// for the document.
func (ss *StylesStack) SetDefaultFontFamily(ff *frontend.FontFamily) {
	for _, sty := range *ss {
		sty.DefaultFontFamily = ff
	}
}

// SetDefaultFontSize sets the document font size which should be used for rem
// calculation.
func (ss *StylesStack) SetDefaultFontSize(size bag.ScaledPoint) {
	for _, sty := range *ss {
		sty.DefaultFontSize = size
	}
}

// parseCSSContentValue parses a CSS content value string, handling quoted
// strings and CSS unicode escapes like \2022 (→ "•").
func parseCSSContentValue(val string) string {
	val = strings.TrimSpace(val)
	// Remove surrounding quotes
	if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
		(strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`)) {
		val = val[1 : len(val)-1]
	}
	// Resolve CSS unicode escapes: \HHHH
	var b strings.Builder
	for i := 0; i < len(val); i++ {
		if val[i] == '\\' && i+1 < len(val) {
			// Collect hex digits (up to 6)
			j := i + 1
			for j < len(val) && j < i+7 && isHexDigit(val[j]) {
				j++
			}
			if j > i+1 {
				cp, err := strconv.ParseInt(val[i+1:j], 16, 32)
				if err == nil {
					b.WriteRune(rune(cp))
				}
				// Skip optional trailing space after hex escape
				if j < len(val) && val[j] == ' ' {
					j++
				}
				i = j - 1
				continue
			}
		}
		b.WriteByte(val[i])
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Output turns HTML structure into a nested frontend.Text element.
// anchorPages provides the id → page map from a previous render pass
// (used to resolve CSS target-counter() references). Pass nil on the
// first pass or when the call is not in a target-counter context.
func Output(cb *CSSBuilder, item *HTMLItem, ss StylesStack, df *frontend.Document, anchorPages map[string]int) (*frontend.Text, error) {
	// item is guaranteed to be in vertical direction
	newte := frontend.NewText()
	styles := ss.PushStyles()
	if err := StylesToStyles(styles, item.Styles, df, ss.CurrentStyle().Fontsize); err != nil {
		return nil, err
	}
	applyLangAndHyphens(styles, item.Attributes, df)
	// CSS Lists 3: process counter-reset / counter-increment on this
	// element before any child sees the resulting counter values. The
	// stack walks performed by counter()/counters() at content time read
	// these values directly off the styles in the stack.
	ss.applyCounters()
	ApplySettings(newte.Settings, styles)
	newte.Settings[frontend.SettingDebug] = item.Data
	// Any element with an id attribute creates a named PDF destination.
	if id, ok := item.Attributes["id"]; ok {
		newte.Settings[frontend.SettingDest] = id
	}
	switch item.Data {
	case "html":
		if fs, ok := item.Styles["font-size"]; ok {
			rfs := ParseRelativeSize(fs, 0, 0)
			ss.SetDefaultFontSize(rfs)
		}
		if ffs, ok := item.Styles["font-family"]; ok {
			ff := resolveCSSFontFamily(ffs, df)
			if ff == nil {
				ff = df.FindFontFamily("serif")
			}
			ss.SetDefaultFontFamily(ff)
		}
	case "body":
		if ffs, ok := item.Styles["font-family"]; ok {
			ff := resolveCSSFontFamily(ffs, df)
			if ff == nil {
				ff = df.FindFontFamily("serif")
			}
			ss.SetDefaultFontFamily(ff)
		}
	case "td", "th":
		if cs, ok := item.Attributes["colspan"]; ok {
			if colspan, err := strconv.Atoi(cs); err == nil {
				newte.Settings[frontend.SettingColspan] = colspan
			}
		}
		if rs, ok := item.Attributes["rowspan"]; ok {
			if rowspan, err := strconv.Atoi(rs); err == nil {
				newte.Settings[frontend.SettingRowspan] = rowspan
			}
		}
		if vlid, ok := item.Attributes["data-vlist-id"]; ok {
			newte.Settings[frontend.SettingPrerenderedVListID] = vlid
		}
	case "col":
		// First check data-width (from XTS), then CSS width
		if wd, ok := item.Attributes["data-width"]; ok {
			newte.Settings[frontend.SettingColumnWidth] = wd
		} else if wd, ok := item.Styles["width"]; ok {
			newte.Settings[frontend.SettingColumnWidth] = wd
		}
	// case "table":
	// 	tbl, err := processTable(item, ss, df)
	// 	ss.PopStyles()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	newte.Items = append(newte.Items, tbl)
	// 	return newte, nil
	case "ol", "ul":
		styles.OlCounter = 0
		// ListPaddingLeft is the gutter into which <li>'s marker hangs.
		// Only overwrite it when THIS <ol>/<ul> declares an explicit
		// padding-left; a nested list with padding-left: 0 keeps the
		// outer list's gutter so its markers stay aligned with the
		// outer markers rather than overlapping the body text.
		if styles.PaddingLeft > 0 {
			styles.ListPaddingLeft = styles.PaddingLeft
		}
	case "li":
		var marker string
		// CSS Lists 3 / CSS Pseudo 4 distinguish two pseudos for list
		// items: ::marker is the dedicated marker pseudo (the bullet or
		// number), ::before is generated content between the marker and
		// the body. This codebase historically used ::before for both
		// because ::marker was unimplemented; we keep that as a legacy
		// path and let ::marker win when both are set.
		resolveContent := func(raw string) string {
			tokens := csshtml.ParseContentValue(raw)
			attrLookup := func(name string) string {
				return item.Attributes[name]
			}
			return evaluateContentWithStack(tokens, ss, anchorPages, cb.anchorTexts, attrLookup)
		}
		if markerContent, ok := item.Styles["marker::content"]; ok {
			marker = resolveContent(markerContent)
		} else if beforeContent, ok := item.Styles["before::content"]; ok {
			marker = resolveContent(beforeContent)
		} else if strings.HasPrefix(styles.ListStyleType, `"`) && strings.HasSuffix(styles.ListStyleType, `"`) {
			marker = strings.TrimPrefix(styles.ListStyleType, `"`)
			marker = strings.TrimSuffix(marker, `"`)
		} else {
			switch styles.ListStyleType {
			case "disc":
				marker = "•"
			case "circle":
				marker = "◦"
			case "none":
				marker = ""
			case "square":
				marker = "□"
			case "decimal":
				marker = fmt.Sprintf("%d.", styles.OlCounter)
			default:
				marker = "•"
			}
		}
		markerSettings := make(frontend.TypesettingSettings, len(newte.Settings))
		for k, v := range newte.Settings {
			markerSettings[k] = v
		}
		// text-align on the ::before pseudo controls how the marker is
		// laid out inside the gutter. CSS browsers render markers as if
		// `text-align: right` (numbers right-aligned to the padding
		// edge); we keep that as the default. `text-align: left` gives
		// the legal-code look where every marker starts at the same X.
		markerAlign := "right"
		// Apply ::before then ::marker properties to the marker
		// settings. ::marker wins when both pseudos set the same
		// property; the spec treats ::marker as the dedicated marker
		// pseudo, ::before remains supported for legacy stylesheets.
		applyMarkerProps := func(prefix string) {
			for sKey, sVal := range item.Styles {
				if !strings.HasPrefix(sKey, prefix) {
					continue
				}
				switch strings.TrimPrefix(sKey, prefix) {
				case "color":
					if c := df.GetColor(sVal); c != nil {
						markerSettings[frontend.SettingColor] = c
					}
				case "font-weight":
					if fw, err := strconv.Atoi(sVal); err == nil {
						markerSettings[frontend.SettingFontWeight] = frontend.FontWeight(fw)
					} else if sVal == "bold" {
						markerSettings[frontend.SettingFontWeight] = frontend.FontWeight700
					}
				case "font-style":
					switch sVal {
					case "italic", "oblique":
						markerSettings[frontend.SettingStyle] = frontend.FontStyleItalic
					case "normal":
						markerSettings[frontend.SettingStyle] = frontend.FontStyleNormal
					}
				case "font-family":
					if ff := resolveCSSFontFamily(sVal, df); ff != nil {
						markerSettings[frontend.SettingFontFamily] = ff
					}
				case "font-size":
					// em/% resolve against the <li>'s own font size,
					// not the document root — a marker stays in scale
					// with its surrounding line.
					sz := ParseRelativeSize(sVal, styles.Fontsize, styles.Fontsize)
					if sz > 0 {
						markerSettings[frontend.SettingSize] = sz
					}
				case "text-align":
					if sVal == "left" || sVal == "right" {
						markerAlign = sVal
					}
				}
			}
		}
		applyMarkerProps("before::")
		applyMarkerProps("marker::")
		if marker != "" {
			n, err := df.BuildNodelistFromString(markerSettings, marker)
			if err != nil {
				return nil, err
			}
			gap := node.NewKern()
			gap.Kern = styles.Fontsize / 3 // ~0.33em

			var hbox *node.HList
			if markerAlign == "left" {
				// Left-aligned: a hard -ListPaddingLeft shift puts the
				// marker's leftmost edge flush at X = -ListPaddingLeft,
				// then a fil-stretch glue after the gap absorbs the
				// remaining space up to the body anchor.
				leftShift := node.NewGlue()
				leftShift.Width = -styles.ListPaddingLeft
				fill := node.NewGlue()
				fill.Stretch = 1 * bag.Factor
				fill.StretchOrder = node.StretchFil

				node.InsertBefore(n, n, leftShift)
				markerTail := node.Tail(n) // last glyph of the marker
				node.InsertAfter(leftShift, markerTail, gap)
				node.InsertAfter(leftShift, gap, fill)
				hbox = node.HpackTo(leftShift, 0)
			} else {
				// Right-aligned (default): a fil-stretch glue absorbs
				// the space before the marker, so its rightmost edge
				// stays anchored at X = 0 (minus the gap).
				glue1 := node.NewGlue()
				glue1.Width = -styles.ListPaddingLeft
				glue1.Stretch = 1 * bag.Factor
				glue1.StretchOrder = node.StretchFil

				node.InsertBefore(n, n, glue1)
				node.InsertAfter(glue1, node.Tail(n), gap)
				hbox = node.HpackTo(glue1, 0)
			}
			// CSS list-style-position: outside — anchor the marker at
			// the line's content origin (line.x + IndentLeft), so it
			// stays in the gutter even when the body uses text-align:
			// center/right. FormatParagraph stamps the IndentLeft
			// value onto the second attribute just before Mknodes.
			if hbox.Attributes == nil {
				hbox.Attributes = node.H{}
			}
			hbox.Attributes["outside-marker"] = true
			newte.Settings[frontend.SettingPrepend] = hbox
		}
	}

	var te *frontend.Text
	cur := ModeVertical

	// display = "none"
	if styles.Hide {
		ss.PopStyles()
		return newte, nil
	}

	for _, itm := range item.Children {
		if itm.Dir == ModeHorizontal {
			// Going from vertical to horizontal.
			if cur == ModeVertical && itm.Data == " " {
				// there is only a whitespace element.
				continue
			}
			// now in horizontal mode, there can be more children in horizontal
			// mode, so append all of them to a single frontend.Text element
			if itm.Typ == html.TextNode && cur == ModeVertical {
				itm.Data = strings.TrimLeft(itm.Data, " ")
			}
			if te == nil {
				te = frontend.NewText()
				styles = ss.PushStyles()
			}
			ApplySettings(te.Settings, styles)
			if isFootnoteElement(itm) {
				// Footnote inline element: collect its contents into a
				// separate Text and append a sentinel to te. extractFootnotes
				// will later replace the sentinel with a marker call and
				// format the body as a standalone paragraph.
				fnText := frontend.NewText()
				ApplySettings(fnText.Settings, styles)
				if err := collectHorizontalNodes(cb, fnText, itm, ss, ss.CurrentStyle().Fontsize, ss.CurrentStyle().DefaultFontSize, df, anchorPages); err != nil {
					return nil, err
				}
				te.Items = append(te.Items, insertMarker{Class: InsertFootnote, Body: fnText})
			} else if isFloatElement(itm) {
				// Float element (top or bottom, per position attribute):
				// collect contents into a separate Text and leave a
				// sentinel. extractFloats replaces the sentinel with an
				// empty placeholder (no in-text glyph) and formats the
				// body for placement at the appropriate page edge.
				flText := frontend.NewText()
				ApplySettings(flText.Settings, styles)
				if err := collectHorizontalNodes(cb, flText, itm, ss, ss.CurrentStyle().Fontsize, ss.CurrentStyle().DefaultFontSize, df, anchorPages); err != nil {
					return nil, err
				}
				te.Items = append(te.Items, insertMarker{Class: floatClassFor(itm), Body: flText})
			} else {
				if err := collectHorizontalNodes(cb, te, itm, ss, ss.CurrentStyle().Fontsize, ss.CurrentStyle().DefaultFontSize, df, anchorPages); err != nil {
					return nil, err
				}
			}
			cur = ModeHorizontal
		} else {
			// still vertical
			if itm.Data == "li" {
				styles.OlCounter++
			}
			if te != nil {
				newte.Items = append(newte.Items, te)
				newte.Settings[frontend.SettingBox] = true
				te = nil
			}
			// Block-level float: build the body via a recursive Output()
			// call (treats float children as block-level), and append a
			// marker to the parent. extractFloats picks up the marker at
			// paragraph-formatting time and lifts the body into the
			// page-level insert system.
			if isFloatElement(itm) {
				floatBody, err := Output(cb, itm, ss, df, anchorPages)
				if err != nil {
					return nil, err
				}
				newte.Items = append(newte.Items, insertMarker{Class: floatClassFor(itm), Body: floatBody})
				continue
			}
			te, err := Output(cb, itm, ss, df, anchorPages)
			if err != nil {
				return nil, err
			}
			// Always include td/th/col elements even if empty (for table structure)
			if len(te.Items) > 0 || itm.Data == "td" || itm.Data == "th" || itm.Data == "col" {
				newte.Items = append(newte.Items, te)
			}
		}
	}
	if item.Dir == ModeVertical && cur == ModeVertical {
		newte.Settings[frontend.SettingBox] = true
	}
	switch item.Data {
	case "ul", "ol":
		ulte := frontend.NewText()
		ApplySettings(ulte.Settings, styles)
		ulte.Settings[frontend.SettingDebug] = item.Data
		ulte.Settings[frontend.SettingBox] = true
	}
	if te != nil {
		newte.Items = append(newte.Items, te)
		ss.PopStyles()
		te = nil
	}
	ss.PopStyles()
	return newte, nil
}

func collectHorizontalNodes(cb *CSSBuilder, te *frontend.Text, item *HTMLItem, ss StylesStack, currentFontsize bag.ScaledPoint, defaultFontsize bag.ScaledPoint, df *frontend.Document, anchorPages map[string]int) error {
	switch item.Typ {
	case html.TextNode:
		te.Items = append(te.Items, item.Data)
	case html.ElementNode:
		childSettings := make(frontend.TypesettingSettings, 8)

		// Inline element with id="..." → record as anchor target for
		// CSS target-counter() / target-text() and plant an anchorMarker
		// in te.Items. The marker is pulled out before FormatParagraph
		// runs by extractAnchorMarkers; the page assignment happens at
		// shipout through the enclosing paragraph's _anchor_indices.
		// Block-level ids are caught by Output() instead (different
		// code path), so this only sees actually-inline elements.
		if id, ok := item.Attributes["id"]; ok && id != "" {
			childSettings[frontend.SettingDest] = id
			cb.Anchors = append(cb.Anchors, AnchorEntry{
				ID:   id,
				Text: truncateAnchorText(extractTextFromHTMLItem(item)),
			})
			te.Items = append(te.Items, anchorMarker{Idx: cb.anchorCount})
			cb.anchorCount++
		}

		// emitGeneratedContent renders a CSS content value (from
		// ::before or ::after) into te.Items as one or more sub-Texts:
		// strings accumulate, ContentLeader emits its own SettingLeader
		// sub-Text so Mknodes can build the fil³ glue. Used for both
		// pseudo elements; <li>::before goes through its own marker
		// path elsewhere.
		emitGeneratedContent := func(contentValue string) error {
			tokens := csshtml.ParseContentValue(contentValue)
			if len(tokens) == 0 {
				return nil
			}
			attrLookup := func(name string) string {
				return item.Attributes[name]
			}
			sty := ss.PushStyles()
			if err := StylesToStyles(sty, item.Styles, df, currentFontsize); err != nil {
				ss.PopStyles()
				return err
			}
			applyLangAndHyphens(sty, item.Attributes, df)

			flushString := func(s string) {
				if s == "" {
					return
				}
				txt := frontend.NewText()
				ApplySettings(txt.Settings, sty)
				txt.Items = append(txt.Items, s)
				te.Items = append(te.Items, txt)
			}

			var buf strings.Builder
			single := make([]csshtml.ContentToken, 1)
			for _, tok := range tokens {
				if tok.Type == csshtml.ContentLeader {
					flushString(buf.String())
					buf.Reset()
					leaderTxt := frontend.NewText()
					ApplySettings(leaderTxt.Settings, sty)
					leaderTxt.Settings[frontend.SettingLeader] = tok.Value
					te.Items = append(te.Items, leaderTxt)
					continue
				}
				single[0] = tok
				buf.WriteString(evaluateContentWithStack(single, ss, anchorPages, cb.anchorTexts, attrLookup))
			}
			flushString(buf.String())
			ss.PopStyles()
			return nil
		}

		// ::before pseudo-element on inline elements. Renders before
		// the children. Skipped on <li> because the marker pseudo-
		// content path handles ::before there with its own gutter
		// positioning and would otherwise double-render.
		if item.Data != "li" {
			if beforeContent, ok := item.Styles["before::content"]; ok && beforeContent != "" {
				if err := emitGeneratedContent(beforeContent); err != nil {
					return err
				}
			}
		}

		switch item.Data {
		case "a":
			var href, link string
			for k, v := range item.Attributes {
				switch k {
				case "href":
					href = v
				case "link":
					link = v
				}
			}
			if strings.HasPrefix(href, "#") {
				link = strings.TrimPrefix(href, "#")
				href = ""
			}
			if href != "" || link != "" {
				hl := document.Hyperlink{URI: href, Local: link}
				childSettings[frontend.SettingHyperlink] = hl
			}
		case "img":
			cs := ss.CurrentStyle()
			var filename string
			var wd, ht bag.ScaledPoint

			for k, v := range item.Attributes {
				switch k {
				case "width":
					wd = bag.MustSP(v)
				case "!width":
					if !strings.HasSuffix(v, "%") {
						wd = ParseRelativeSize(v, cs.Fontsize, defaultFontsize)
					}
				case "height":
					ht = bag.MustSP(v)
				case "src":
					filename = v
				}
			}

			if strings.ToLower(filepath.Ext(filename)) == ".svg" {
				// SVG image
				f, err := os.Open(filename)
				if err != nil {
					return fmt.Errorf("opening SVG %s: %w", filename, err)
				}
				svgDoc, err := svgreader.Parse(f)
				f.Close()
				if err != nil {
					return fmt.Errorf("parsing SVG %s: %w", filename, err)
				}
				textRenderer := frontend.NewSVGTextRenderer(df)
				svgNode := df.Doc.CreateSVGNodeFromDocument(svgDoc, wd, ht, textRenderer)
				// Wrap in VList so the SVG is correctly positioned in
				// horizontal mode. The SVG renderer draws from (0,0)
				// downward; a VList in an HList starts output from the
				// top, which matches the SVG coordinate system.
				svgVL := node.Vpack(svgNode)
				svgVL.Attributes = node.H{
					"origin": "svg",
					"attr":   item.Attributes,
				}
				if alt, ok := item.Attributes["alt"]; ok {
					svgVL.Attributes["alt"] = alt
				}
				te.Items = append(te.Items, svgVL)
			} else {
				// Raster image (PNG, JPEG, PDF)
				imgfile, err := df.Doc.LoadImageFile(filename)
				if err != nil {
					return err
				}
				imgNode := df.Doc.CreateImageNodeFromImagefile(imgfile, 1, "/MediaBox")
				// Apply user-specified dimensions
				if wd > 0 && ht > 0 {
					imgNode.Width = wd
					imgNode.Height = ht
				} else if wd > 0 {
					// Scale height proportionally
					imgNode.Height = bag.ScaledPoint(float64(imgNode.Height) * float64(wd) / float64(imgNode.Width))
					imgNode.Width = wd
				} else if ht > 0 {
					// Scale width proportionally
					imgNode.Width = bag.ScaledPoint(float64(imgNode.Width) * float64(ht) / float64(imgNode.Height))
					imgNode.Height = ht
				}
				imgNode.Attributes = node.H{}
				imgNode.Attributes["wd"] = wd
				imgNode.Attributes["ht"] = ht
				imgNode.Attributes["attr"] = item.Attributes
				if alt, ok := item.Attributes["alt"]; ok {
					imgNode.Attributes["alt"] = alt
				}
				te.Items = append(te.Items, imgNode)
			}
		case "barcode":
			var value, typ, eclevelStr string
			var wd, ht bag.ScaledPoint
			for k, v := range item.Attributes {
				switch k {
				case "value":
					value = v
				case "type":
					typ = v
				case "width":
					if sp, err := bag.SP(v); err == nil {
						wd = sp
					} else {
						return fmt.Errorf("barcode: invalid width %q: %w", v, err)
					}
				case "!width":
					cs := ss.CurrentStyle()
					if !strings.HasSuffix(v, "%") {
						wd = ParseRelativeSize(v, cs.Fontsize, defaultFontsize)
					}
				case "height":
					if sp, err := bag.SP(v); err == nil {
						ht = sp
					} else {
						return fmt.Errorf("barcode: invalid height %q: %w", v, err)
					}
				case "eclevel":
					eclevelStr = v
				}
			}
			if value == "" {
				return fmt.Errorf("barcode: missing value attribute")
			}
			if wd == 0 {
				wd = bag.MustSP("3cm")
			}
			bcType, err := parseBarcodeType(typ)
			if err != nil {
				return err
			}
			ecl := parseQRECLevel(eclevelStr)
			bcNode, err := createBarcode(bcType, value, wd, ht, df, ecl)
			if err != nil {
				return err
			}
			te.Items = append(te.Items, bcNode)
		case "br":
			te.Items = append(te.Items, node.NewHardBreak())
			return nil
		}

		// Handle content-generated leaders on empty elements.
		if contentVal, ok := item.Styles["content"]; ok && strings.HasPrefix(contentVal, "leader(") {
			leaderText := frontend.NewText()
			sty := ss.PushStyles()
			if err := StylesToStyles(sty, item.Styles, df, currentFontsize); err != nil {
				ss.PopStyles()
				return err
			}
			applyLangAndHyphens(sty, item.Attributes, df)
			ApplySettings(leaderText.Settings, sty)
			te.Items = append(te.Items, leaderText)
			ss.PopStyles()
			return nil
		}

		for _, itm := range item.Children {
			cld := frontend.NewText()
			sty := ss.PushStyles()
			if err := StylesToStyles(sty, item.Styles, df, currentFontsize); err != nil {
				return err
			}
			applyLangAndHyphens(sty, item.Attributes, df)
			ApplySettings(cld.Settings, sty)
			for k, v := range childSettings {
				cld.Settings[k] = v
			}
			if err := collectHorizontalNodes(cb, cld, itm, ss, currentFontsize, defaultFontsize, df, anchorPages); err != nil {
				return err
			}
			if isFootnoteElement(itm) {
				te.Items = append(te.Items, insertMarker{Class: InsertFootnote, Body: cld})
			} else if isFloatElement(itm) {
				te.Items = append(te.Items, insertMarker{Class: floatClassFor(itm), Body: cld})
			} else {
				te.Items = append(te.Items, cld)
			}
			ss.PopStyles()
		}

		// ::after pseudo-element on inline elements. Renders after the
		// children, inheriting the element's typesetting settings.
		// Skipped on <li> (marker path renders ::before through a
		// different gutter mechanism).
		if item.Data != "li" {
			if afterContent, ok := item.Styles["after::content"]; ok && afterContent != "" {
				if err := emitGeneratedContent(afterContent); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
