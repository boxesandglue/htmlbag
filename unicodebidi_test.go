package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// textWith returns the innermost Text whose own strings contain s.
func textWith(te *frontend.Text, s string) *frontend.Text {
	for _, itm := range te.Items {
		switch v := itm.(type) {
		case string:
			if strings.Contains(v, s) {
				return te
			}
		case *frontend.Text:
			if found := textWith(v, s); found != nil {
				return found
			}
		}
	}
	return nil
}

// TestUnicodeBidiSettings checks the UA rules of HTML §15.3.5 for dir=, bdi
// and bdo, and that unicode-bidi reaches only the element that declares it.
func TestUnicodeBidiSettings(t *testing.T) {
	ub := func(ub frontend.UnicodeBidi) *frontend.UnicodeBidi { return &ub }
	dir := func(d frontend.Direction) *frontend.Direction { return &d }
	testcases := []struct {
		name    string
		body    string
		text    string
		wantUB  *frontend.UnicodeBidi
		wantDir *frontend.Direction
	}{
		{"bdi", `<p>User <bdi>NAME</bdi>: 3 posts</p>`, "NAME", ub(frontend.UnicodeBidiPlaintext), nil},
		{"bdi with dir", `<p>User <bdi dir="rtl">NAME</bdi></p>`, "NAME", ub(frontend.UnicodeBidiIsolate), dir(frontend.DirectionRTL)},
		{"span dir=rtl", `<p>a <span dir="rtl">NAME</span> b</p>`, "NAME", ub(frontend.UnicodeBidiIsolate), dir(frontend.DirectionRTL)},
		{"dir case-insensitive", `<p>a <span dir="RTL">NAME</span> b</p>`, "NAME", ub(frontend.UnicodeBidiIsolate), dir(frontend.DirectionRTL)},
		{"span dir=auto", `<p>a <span dir="auto">NAME</span> b</p>`, "NAME", ub(frontend.UnicodeBidiPlaintext), nil},
		{"bdo", `<p>a <bdo dir="rtl">NAME</bdo> b</p>`, "NAME", ub(frontend.UnicodeBidiIsolateOverride), dir(frontend.DirectionRTL)},
		// An inherited direction does not hide the dir attribute.
		{"dir=ltr in rtl paragraph", `<p dir="rtl">א <span dir="ltr">NAME</span> ב</p>`, "NAME", ub(frontend.UnicodeBidiIsolate), dir(frontend.DirectionLTR)},
		// CSS on the element wins over the UA rule.
		{"author unicode-bidi", `<p>a <span dir="rtl" style="unicode-bidi: embed">NAME</span> b</p>`, "NAME", ub(frontend.UnicodeBidiEmbed), dir(frontend.DirectionRTL)},
		{"author normal", `<p>a <span dir="rtl" style="unicode-bidi: normal">NAME</span> b</p>`, "NAME", nil, dir(frontend.DirectionRTL)},
		// unicode-bidi is not inherited.
		{"not inherited", `<p>a <span style="unicode-bidi: bidi-override; direction: rtl">x <em>NAME</em></span></p>`, "NAME", nil, dir(frontend.DirectionRTL)},
		{"plain span", `<p>a <span>NAME</span> b</p>`, "NAME", nil, dir(frontend.DirectionLTR)},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			_, te := runStylePass(t, `<!DOCTYPE html><html><body>`+tc.body+`</body></html>`)
			found := textWith(te, tc.text)
			if found == nil {
				t.Fatalf("no Text holds %q", tc.text)
			}
			gotUB, hasUB := found.Settings[frontend.SettingUnicodeBidi].(frontend.UnicodeBidi)
			switch {
			case tc.wantUB == nil && hasUB:
				t.Errorf("SettingUnicodeBidi %v, want none", gotUB)
			case tc.wantUB != nil && (!hasUB || gotUB != *tc.wantUB):
				t.Errorf("SettingUnicodeBidi %v (set %v), want %v", gotUB, hasUB, *tc.wantUB)
			}
			gotDir, hasDir := found.Settings[frontend.SettingDirection].(frontend.Direction)
			switch {
			case tc.wantDir == nil && hasDir:
				t.Errorf("SettingDirection %v, want none", gotDir)
			case tc.wantDir != nil && (!hasDir || gotDir != *tc.wantDir):
				t.Errorf("SettingDirection %v (set %v), want %v", gotDir, hasDir, *tc.wantDir)
			}
		})
	}
}

// TestBdiVisualOrder sets the example of HTML's bdi element from the HTML
// to the line: without the isolate the colon and the number after the
// Hebrew name join its right-to-left run.
func TestBdiVisualOrder(t *testing.T) {
	for _, tc := range []struct {
		body string
		want string
	}{
		{`<p>User <span>אבג</span>: 3 posts</p>`, "User 3 :גבא posts"},
		{`<p>User <bdi>אבג</bdi>: 3 posts</p>`, "User גבא: 3 posts"},
	} {
		fe, te := runStylePass(t, `<!DOCTYPE html><html><body>`+tc.body+`</body></html>`)
		p := textWith(te, "User ")
		if p == nil {
			t.Fatal("no paragraph")
		}
		vl, _, err := fe.FormatParagraph(p, bag.MustSP("400pt"))
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for n := vl.List; n != nil; n = n.Next() {
			hl, ok := n.(*node.HList)
			if !ok {
				continue
			}
			for c := hl.List; c != nil; c = c.Next() {
				switch v := c.(type) {
				case *node.Glyph:
					b.WriteString(v.Components)
				case *node.Glue:
					if v.Width > 0 && v.StretchOrder == 0 {
						b.WriteString(" ")
					}
				}
			}
		}
		if got := strings.TrimSpace(b.String()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.body, got, tc.want)
		}
	}
}
