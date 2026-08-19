package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestInitialLetterDropcap — a paragraph with initial-letter gets its
// first letter carved out into a zero-width prepend box, the paragraph
// indents its first N rows, and the remaining text lost the letter.
func TestInitialLetterDropcap(t *testing.T) {
	te := renderToText(t, `<!DOCTYPE html><html><head><style>
	p { initial-letter: 3; }
	</style></head><body><p>Wonderful drop caps here.</p></body></html>`)

	// walk down until the Text carrying the prepend box is found
	var para *frontend.Text
	var walk func(*frontend.Text)
	walk = func(t *frontend.Text) {
		if para != nil {
			return
		}
		if _, ok := t.Settings[frontend.SettingPrepend]; ok {
			para = t
			return
		}
		for _, itm := range t.Items {
			if child, ok := itm.(*frontend.Text); ok {
				walk(child)
			}
		}
	}
	walk(te)
	if para == nil {
		t.Fatal("no text with prepend box found")
	}
	if para.Settings[frontend.SettingIndentLeftRows] != 3 {
		t.Fatalf("IndentLeftRows = %v, want 3 (settings: %v)", para.Settings[frontend.SettingIndentLeftRows], para.Settings)
	}
	if il, ok := para.Settings[frontend.SettingIndentLeft].(interface{}); !ok || il == nil {
		t.Errorf("IndentLeft missing")
	}
	hbox, ok := para.Settings[frontend.SettingPrepend].(*node.HList)
	if !ok {
		t.Fatal("no prepend hbox")
	}
	if hbox.Width != 0 || hbox.Height != 0 {
		t.Errorf("prepend box must be zero-sized: w=%v h=%v", hbox.Width, hbox.Height)
	}
	if outside, _ := hbox.Attributes["outside-marker"].(bool); !outside {
		t.Errorf("prepend box must carry the outside-marker attribute")
	}
	var firstString string
	var findString func(*frontend.Text)
	findString = func(t *frontend.Text) {
		for _, itm := range t.Items {
			if firstString != "" {
				return
			}
			switch v := itm.(type) {
			case string:
				firstString = v
				return
			case *frontend.Text:
				findString(v)
			}
		}
	}
	findString(para)
	if firstString != "onderful drop caps here." {
		t.Errorf("first string = %q, want text without the W", firstString)
	}
}
