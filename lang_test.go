package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/lang"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// runStylePass renders an HTML fragment through the same path used by the
// production renderer up to the point where settings are populated, then
// returns the resulting *frontend.Text tree without ever touching a PDF
// writer. Tests use this to observe how lang= and hyphens flow through.
func runStylePass(t *testing.T, html string) (*frontend.Document, *frontend.Text) {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cssParser := csshtml.NewCSSParserWithDefaults()
	cb, err := New(fe, cssParser)
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	return fe, te
}

// findSettingLanguage walks a Text tree and returns the first SettingLanguage
// it sees that matches wantTag. Returns nil if no such setting was found.
func findSettingLanguage(te *frontend.Text, wantTag string) *lang.Lang {
	if te == nil {
		return nil
	}
	if v, ok := te.Settings[frontend.SettingLanguage]; ok {
		if l, ok := v.(*lang.Lang); ok && l != nil && l.Name == wantTag {
			return l
		}
	}
	for _, itm := range te.Items {
		if sub, ok := itm.(*frontend.Text); ok {
			if l := findSettingLanguage(sub, wantTag); l != nil {
				return l
			}
		}
	}
	return nil
}

// TestLangAttributeRouting confirms HTML lang= reaches frontend.Settings as a
// resolved *lang.Lang (cached on the document). This is the wiring that was
// dead before the lang/hyphens patch.
func TestLangAttributeRouting(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p lang="de">deutsch</p></body></html>`
	_, te := runStylePass(t, html)
	if l := findSettingLanguage(te, "de"); l == nil {
		t.Fatal(`SettingLanguage with Name="de" not found in Text tree`)
	}
}

// TestXMLLangFallback ensures xml:lang= is honoured when no plain lang= is
// present, matching the HTML5 inheritance rules used by foproc.lua.
func TestXMLLangFallback(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p xml:lang="ar">عربي</p></body></html>`
	_, te := runStylePass(t, html)
	if l := findSettingLanguage(te, "ar"); l == nil {
		t.Fatal(`SettingLanguage with Name="ar" not found in Text tree`)
	}
}

// TestHyphensNoneOverridesLang verifies CSS hyphens: none picks the no-op
// hyphenator regardless of the language tag in scope. The expected Name in
// settings is the noHyphenationKey sentinel, not the language tag.
func TestHyphensNoneOverridesLang(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p lang="de" style="hyphens:none">x</p></body></html>`
	_, te := runStylePass(t, html)
	if l := findSettingLanguage(te, noHyphenationKey); l == nil {
		t.Fatal(`hyphens:none did not route SettingLanguage to no-op sentinel`)
	}
}

// hasSettingDirection walks a Text tree and reports whether any node in the
// subtree carries SettingDirection == want. Children's settings are stored
// on nested *frontend.Text items; the outer body and html nodes each carry
// their own SettingDirection (LTR by default), so we cannot just look at
// the root.
func hasSettingDirection(te *frontend.Text, want frontend.Direction) bool {
	if te == nil {
		return false
	}
	if v, ok := te.Settings[frontend.SettingDirection]; ok {
		if d, ok := v.(frontend.Direction); ok && d == want {
			return true
		}
	}
	for _, itm := range te.Items {
		if sub, ok := itm.(*frontend.Text); ok {
			if hasSettingDirection(sub, want) {
				return true
			}
		}
	}
	return false
}

// TestDirAttributeRouting confirms the HTML dir= attribute reaches the
// SettingDirection in the same way lang= reaches SettingLanguage. Before the
// dir-honoring patch this was a dead path: dir="rtl" on <p> was silently
// dropped and the backend's auto-detect picked LTR for paragraphs whose
// content started with a Latin label, producing left-aligned output even
// though the author asked for RTL.
func TestDirAttributeRouting(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p dir="rtl" lang="ar">عربي</p></body></html>`
	_, te := runStylePass(t, html)
	if !hasSettingDirection(te, frontend.DirectionRTL) {
		t.Fatal(`SettingDirection=DirectionRTL not found in Text tree for dir="rtl"`)
	}
}

// TestCSSDirectionWinsOverDirAttribute verifies HTML §3.2.6.2: dir= maps to
// CSS direction via a UA-stylesheet rule, so an author CSS direction
// declaration outranks the attribute. Author CSS direction:rtl on a
// dir="ltr" element must end up RTL (the element-level RTL must be
// observable in the tree even though body's default LTR is also present).
func TestCSSDirectionWinsOverDirAttribute(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p dir="ltr" style="direction:rtl" lang="ar">عربي</p></body></html>`
	_, te := runStylePass(t, html)
	if !hasSettingDirection(te, frontend.DirectionRTL) {
		t.Fatal(`SettingDirection=DirectionRTL not found — CSS direction:rtl should win over dir="ltr"`)
	}
}

// TestPerInlineLangSwitch checks that an inline lang= different from its
// parent block flows into the Text tree as a distinct sub-Text setting,
// which is the precondition for Mknodes emitting node.Lang switches around
// the inline run.
func TestPerInlineLangSwitch(t *testing.T) {
	html := `<!DOCTYPE html><html><body>` +
		`<p lang="ar">عربي <span lang="en">english</span> عربي</p>` +
		`</body></html>`
	_, te := runStylePass(t, html)
	if findSettingLanguage(te, "ar") == nil {
		t.Error(`outer SettingLanguage with Name="ar" not found`)
	}
	if findSettingLanguage(te, "en") == nil {
		t.Error(`inner SettingLanguage with Name="en" not found in Text tree`)
	}
}

// findSettingHAlign returns the HAlign value carried at the deepest
// element matching predicate. Used to verify that "text-align: start"
// flows through as the logical HAlignStart, leaving the physical
// resolution to FormatParagraph (where direction is known).
func findSettingHAlign(te *frontend.Text) frontend.HorizontalAlignment {
	if te == nil {
		return frontend.HAlignDefault
	}
	if v, ok := te.Settings[frontend.SettingHAlign]; ok {
		if h, ok := v.(frontend.HorizontalAlignment); ok {
			// Recurse first; deepest setting wins.
			for _, itm := range te.Items {
				if sub, ok := itm.(*frontend.Text); ok {
					if hh := findSettingHAlign(sub); hh != frontend.HAlignDefault {
						return hh
					}
				}
			}
			return h
		}
	}
	for _, itm := range te.Items {
		if sub, ok := itm.(*frontend.Text); ok {
			if hh := findSettingHAlign(sub); hh != frontend.HAlignDefault {
				return hh
			}
		}
	}
	return frontend.HAlignDefault
}

// TestLogicalTextAlignStart guards the CSS Text 3 §7 contract: "start"
// stays logical inside the Settings tree. FormatParagraph turns it into
// HAlignLeft / HAlignRight after the paragraph direction is known. If
// the value were eagerly resolved to HAlignLeft here, an RTL block would
// end up left-aligned — the original 08-mixed-ltr-rtl bug.
func TestLogicalTextAlignStart(t *testing.T) {
	// CSS reset sets text-align: start on body, so any unstyled paragraph
	// inherits it.
	html := `<!DOCTYPE html><html><body><p>text</p></body></html>`
	_, te := runStylePass(t, html)
	if got := findSettingHAlign(te); got != frontend.HAlignStart {
		t.Errorf("text-align: start propagated as %v, want HAlignStart", got)
	}
}

// TestExplicitTextAlignLeftStaysPhysical ensures author-specified physical
// keywords remain physical (no direction-dependent flipping).
func TestExplicitTextAlignLeftStaysPhysical(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p style="text-align:left">text</p></body></html>`
	_, te := runStylePass(t, html)
	if got := findSettingHAlign(te); got != frontend.HAlignLeft {
		t.Errorf("text-align: left propagated as %v, want HAlignLeft", got)
	}
}
