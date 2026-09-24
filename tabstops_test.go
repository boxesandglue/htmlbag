package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestParseTabStops(t *testing.T) {
	fs := bag.MustSP("10pt")
	for _, tc := range []struct {
		in   string
		want []frontend.TabStop
	}{
		{"none", []frontend.TabStop{}},
		{"40mm", []frontend.TabStop{{Position: bag.MustSP("40mm")}}},
		{"2em end", []frontend.TabStop{{Position: bag.MustSP("20pt"), Align: node.TabAlignRight}}},
		{"12mm, 100% end leader(\" . \")", []frontend.TabStop{
			{Position: bag.MustSP("12mm")},
			{Fraction: 1, Align: node.TabAlignRight, Leader: " . "},
		}},
		{"right 50% leader(dotted)", []frontend.TabStop{{Fraction: 0.5, Align: node.TabAlignRight, Leader: ". "}}},
		{"60mm decimal, 110mm decimal(\",\")", []frontend.TabStop{
			{Position: bag.MustSP("60mm"), Align: node.TabAlignDecimal},
			{Position: bag.MustSP("110mm"), Align: node.TabAlignDecimal, Separator: ","},
		}},
		{"1cm left, 2cm start, 3cm center leader(solid)", []frontend.TabStop{
			{Position: bag.MustSP("1cm")},
			{Position: bag.MustSP("2cm")},
			{Position: bag.MustSP("3cm"), Align: node.TabAlignCenter, Leader: "_"},
		}},
		{"1cm leader(',')", []frontend.TabStop{{Position: bag.MustSP("1cm"), Leader: ","}}},
	} {
		got, err := parseTabStops(tc.in, fs, fs)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: stop %d is %+v, want %+v", tc.in, i, got[i], tc.want[i])
			}
		}
	}
	for _, in := range []string{"", "end", "1cm 2cm", "1cm end center", "1cm decimal()", "1cm leader(x)", "abc", "1cm leader('.') leader('-')"} {
		if _, err := parseTabStops(in, fs, fs); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

// TestTabStopsKeepTabs checks that with stops a tab survives collapsing while
// whitespace with a newline in it is still source formatting.
func TestTabStopsKeepTabs(t *testing.T) {
	for _, tc := range []struct {
		style, src, want string
	}{
		{"", "Name \t Alice", "Name Alice"},
		{"-bag-tab-stops: 4cm", "Name \t Alice", "Name\tAlice"},
		{"-bag-tab-stops: 4cm", "\tcentered", "\tcentered"},
		{"-bag-tab-stops: 4cm", "\n\t\tName\tAlice\n\t", "Name\tAlice "},
		{"-bag-tab-stops: 4cm", "a\t\tb", "a\t\tb"},
		{"-bag-tab-stops: 4cm; white-space: pre-line", "a\tb\n\tc", "a\tb\n\tc"},
		{"-bag-tab-stops: 4cm; white-space: pre", "a \t b", "a \t b"},
	} {
		te := renderToText(t, `<!DOCTYPE html><html><body><p style="`+tc.style+`">`+tc.src+`</p></body></html>`)
		got := strings.Join(collectStrings(te.Items), "")
		if got != tc.want {
			t.Errorf("%q with %q: got %q, want %q", tc.src, tc.style, got, tc.want)
		}
	}
	// Inherited, and switched off again with none.
	te := renderToText(t, `<!DOCTYPE html><html><body><div style="-bag-tab-stops: 4cm"><p>a `+"\t"+` b</p><p style="-bag-tab-stops: none">c `+"\t"+` d</p></div></body></html>`)
	if got := strings.Join(collectStrings(te.Items), "|"); !strings.Contains(got, "a\tb") || !strings.Contains(got, "c d") {
		t.Errorf("got %q, want a\\tb and c d", got)
	}
}

// TestTabStopsSetting checks the stops reach the typesetter.
func TestTabStopsSetting(t *testing.T) {
	te := renderToText(t, `<!DOCTYPE html><html><body><ul style="-bag-tab-stops: 12mm, 100% end leader(dotted)"><li>1`+"\t"+`Intro`+"\t"+`3</li></ul></body></html>`)
	got, ok := findSetting(te, frontend.SettingTabStops)
	if !ok {
		t.Fatal("no SettingTabStops")
	}
	stops, _ := got.([]frontend.TabStop)
	if len(stops) != 2 || stops[1].Fraction != 1 || stops[1].Leader != ". " {
		t.Errorf("got %+v", stops)
	}
}
