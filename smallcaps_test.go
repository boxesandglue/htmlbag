package htmlbag

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// smallCapsRuns flattens te into "text@size" runs, "+smcp" marking a run that
// asks for the feature, sizes inherited as bag inherits them.
func smallCapsRuns(te *frontend.Text, size bag.ScaledPoint, smcp bool) []string {
	if v, ok := te.Settings[frontend.SettingSize].(bag.ScaledPoint); ok && v > 0 {
		size = v
	}
	if v, ok := te.Settings[frontend.SettingOpenTypeFeature].([]string); ok {
		smcp = slices.Contains(v, "smcp=1")
	}
	var out []string
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			if strings.TrimSpace(t) == "" && !strings.Contains(t, " ") {
				continue
			}
			r := fmt.Sprintf("%s@%s", t, size)
			if smcp {
				r += "+smcp"
			}
			out = append(out, r)
		case *frontend.Text:
			out = append(out, smallCapsRuns(t, size, smcp)...)
		}
	}
	return out
}

func smallCapsText(t *testing.T, css, body string) []string {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err = cb.AddCSS(css); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><p>` + body + `</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	return smallCapsRuns(te, 0, false)
}

func TestSmallCaps(t *testing.T) {
	for _, tc := range []struct {
		name, css, body string
		want            []string
	}{
		{"off", `p{font-family:monospace;font-size:10pt}`, "Ab c", []string{"Ab c@10"}},
		{"synthesised without smcp", `p{font-family:monospace;font-size:10pt;font-variant:small-caps}`, "Ab c",
			[]string{"A@10", "B@7", " @10", "C@7"}},
		{"font-variant-caps", `p{font-family:monospace;font-size:10pt;font-variant-caps:small-caps}`, "xY",
			[]string{"X@7", "Y@10"}},
		{"inherited, cancelled", `p{font-family:monospace;font-size:10pt;font-variant:small-caps} span{font-variant:normal}`, "a<span>b</span>",
			[]string{"A@7", "b@10"}},
		{"no synthesis", `p{font-family:monospace;font-size:10pt;font-variant:small-caps;font-synthesis-small-caps:none}`, "Ab",
			[]string{"Ab@10"}},
		{"smcp when the face has it", `p{font-family:sans;font-size:10pt;font-variant:small-caps}`, "Ab",
			[]string{"Ab@10+smcp"}},
		{"font shorthand", `p{font:small-caps 10pt monospace}`, "Ab",
			[]string{"A@10", "B@7"}},
		{"font shorthand resets", `p{font-family:monospace;font-size:10pt;font-variant:small-caps} span{font:10pt monospace}`, "a<span>b</span>",
			[]string{"A@7", "b@10"}},
		{"font-variant, then font-variant-caps", `p{font-family:monospace;font-size:10pt;font-variant:normal;font-variant-caps:small-caps}`, "a",
			[]string{"A@7"}},
		{"font-synthesis without small-caps", `p{font-family:monospace;font-size:10pt;font-variant:small-caps;font-synthesis-small-caps:auto;font-synthesis:none}`, "a",
			[]string{"a@10"}},
		{"only capitals", `p{font-family:monospace;font-size:10pt;font-variant:small-caps}`, "AB 12",
			[]string{"AB 12@10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := smallCapsText(t, tc.css, tc.body); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
