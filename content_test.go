package htmlbag

import (
	"reflect"
	"testing"
)

// TestParseContentValue_CounterAndCounters keeps the pre-existing
// counter/counters parsing covered while we extend the same function with
// target-* branches — regression guard.
func TestParseContentValue_CounterAndCounters(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "literal string only",
			in:   `"Page "`,
			want: []ContentToken{{Type: ContentString, Value: "Page "}},
		},
		{
			name: "counter()",
			in:   `counter(page)`,
			want: []ContentToken{{Type: ContentCounter, Value: "page"}},
		},
		{
			name: "counters() with separator",
			in:   `counters(sec, ".")`,
			want: []ContentToken{{Type: ContentCounters, Value: "sec", Separator: "."}},
		},
		{
			name: "string + counter sequence",
			in:   `"Page " counter(page) " of " counter(pages)`,
			want: []ContentToken{
				{Type: ContentString, Value: "Page "},
				{Type: ContentCounter, Value: "page"},
				{Type: ContentString, Value: " of "},
				{Type: ContentCounter, Value: "pages"},
			},
		},
		{
			name: "leader()",
			in:   `leader(".")`,
			want: []ContentToken{{Type: ContentLeader, Value: "."}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_TargetCounter covers all argument forms accepted
// by target-counter: url(#id), "#id", and attr(name).
func TestParseContentValue_TargetCounter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "url(#id) form",
			in:   `target-counter(url(#chap1), page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetID: "chap1"},
			},
		},
		{
			name: "string \"#id\" form",
			in:   `target-counter("#chap1", page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetID: "chap1"},
			},
		},
		{
			name: "attr(href) form",
			in:   `target-counter(attr(href), page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetAttr: "href"},
			},
		},
		{
			name: "with literal prefix",
			in:   `"see page " target-counter(url(#foo), page)`,
			want: []ContentToken{
				{Type: ContentString, Value: "see page "},
				{Type: ContentTargetCounter, Value: "page", TargetID: "foo"},
			},
		},
		{
			name: "non-page counter (parsed, evaluator decides)",
			in:   `target-counter(url(#foo), chapter)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "chapter", TargetID: "foo"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_TargetCounters checks the three-arg form, in
// particular that the trailing separator string is captured.
func TestParseContentValue_TargetCounters(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "url + counter + separator",
			in:   `target-counters(url(#chap1), sec, ".")`,
			want: []ContentToken{
				{Type: ContentTargetCounters, Value: "sec", Separator: ".", TargetID: "chap1"},
			},
		},
		{
			name: "attr + counter + separator",
			in:   `target-counters(attr(href), sec, "-")`,
			want: []ContentToken{
				{Type: ContentTargetCounters, Value: "sec", Separator: "-", TargetAttr: "href"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_TargetText covers the recognition path. The
// evaluator may still not implement extraction in v1, but the parser
// must produce the token so future work doesn't need to revisit parsing.
func TestParseContentValue_TargetText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "no content-type (default content)",
			in:   `target-text(url(#chap1))`,
			want: []ContentToken{
				{Type: ContentTargetText, Value: "content", TargetID: "chap1"},
			},
		},
		{
			name: "explicit content-type",
			in:   `target-text(url(#chap1), before)`,
			want: []ContentToken{
				{Type: ContentTargetText, Value: "before", TargetID: "chap1"},
			},
		},
		{
			name: "attr form",
			in:   `target-text(attr(href))`,
			want: []ContentToken{
				{Type: ContentTargetText, Value: "content", TargetAttr: "href"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_Attr covers top-level attr(name) — CSS Values 4
// generated content from HTML attributes. The fallback / type forms of
// attr() are intentionally not parsed; the tail is skipped.
func TestParseContentValue_Attr(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "bare attr()",
			in:   `attr(vnumber)`,
			want: []ContentToken{{Type: ContentAttr, Value: "vnumber"}},
		},
		{
			name: "attr() with surrounding literal",
			in:   `"Vers " attr(vnumber) ":"`,
			want: []ContentToken{
				{Type: ContentString, Value: "Vers "},
				{Type: ContentAttr, Value: "vnumber"},
				{Type: ContentString, Value: ":"},
			},
		},
		{
			name: "attr() with whitespace inside parens",
			in:   `attr( data-label )`,
			want: []ContentToken{{Type: ContentAttr, Value: "data-label"}},
		},
		{
			name: "empty attr() drops the token",
			in:   `attr()`,
			want: nil,
		},
		{
			name: "attr() inside target-counter still uses TargetAttr (no ContentAttr)",
			in:   `target-counter(attr(href), page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetAttr: "href"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_TargetCounterMalformed makes sure malformed input
// produces no token rather than panicking — silent skip matches the
// existing counter/counters behaviour.
func TestParseContentValue_TargetCounterMalformed(t *testing.T) {
	cases := []string{
		`target-counter()`,             // no args
		`target-counter(url(#x))`,      // missing counter name
		`target-counter(, page)`,       // empty reference
		`target-counter(attr(), page)`, // empty attr name
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got := ParseContentValue(in)
			if len(got) != 0 {
				t.Errorf("ParseContentValue(%q) = %#v; want empty", in, got)
			}
		})
	}
}

// TestCSSQuoteString_PassesNonASCII guards a subtle bug where stringValue()
// used fmt.Sprintf("%q", ...), which escapes non-ASCII codepoints into Go
// `\uXXXX` literals — invalid in CSS (CSS uses `\HHHH ` with no `u`). A
// thin-space (U+2009) inside a content value silently corrupted into the
// literal string "u2009" by the time it reached the renderer.
func TestCSSQuoteString_PassesNonASCII(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ascii only", "abc", `"abc"`},
		{"thin space U+2009", " . ", "\" . \""},
		{"non-breaking space", "x y", "\"x y\""},
		{"embedded quote", `say "hi"`, `"say \"hi\""`},
		{"embedded backslash", `path\to`, `"path\\to"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cssQuoteString(tc.in)
			if got != tc.want {
				t.Errorf("cssQuoteString(%q) = %q (bytes %x); want %q (bytes %x)",
					tc.in, got, []byte(got), tc.want, []byte(tc.want))
			}
		})
	}
}

// TestParseContentValue_ThinSpaceLeader is the integration-level guard
// for the same bug: a leader pattern containing U+2009 must survive
// serialisation and re-tokenising with its bytes intact, not as the
// ASCII string "u2009". The renderer reads the cascade's tokens directly
// these days, but stringValue still produces the text view of every
// computed value, and that text has to parse back to what it came from.
func TestParseContentValue_ThinSpaceLeader(t *testing.T) {
	in := "leader(\" . \")"
	roundTripped := stringValue(tokenizeCSSString(in))
	got := ParseContentValue(roundTripped)
	want := []ContentToken{{Type: ContentLeader, Value: " . "}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip(%q) → %q:\n got  %#v\n want %#v",
			in, roundTripped, got, want)
	}
}

// TestParseContentValue_RoundTrip pins that stringValue's text view of a
// value re-parses into the same thing. Nothing in the render path takes
// that detour any more — the cascade hands the renderer tokens — but the
// text view is what StyleMap.Strings() exposes and what ParseContentValue
// callers outside the package hand in, so the two must agree.
func TestParseContentValue_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "target-counter url",
			in:   `target-counter(url(#chap1), page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetID: "chap1"},
			},
		},
		{
			name: "target-counter attr",
			in:   `target-counter(attr(href), page)`,
			want: []ContentToken{
				{Type: ContentTargetCounter, Value: "page", TargetAttr: "href"},
			},
		},
		{
			name: "target-counters",
			in:   `target-counters(url(#chap1), sec, ".")`,
			want: []ContentToken{
				{Type: ContentTargetCounters, Value: "sec", Separator: ".", TargetID: "chap1"},
			},
		},
		{
			name: "literal + target-counter",
			in:   `"p. " target-counter(url(#x), page)`,
			want: []ContentToken{
				{Type: ContentString, Value: "p. "},
				{Type: ContentTargetCounter, Value: "page", TargetID: "x"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripped := stringValue(tokenizeCSSString(tc.in))
			got := ParseContentValue(roundTripped)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("round-trip(%q) → %q:\n got  %#v\n want %#v",
					tc.in, roundTripped, got, tc.want)
			}
		})
	}
}

// TestParseContentValue_Element covers element(name) (CSS GCPM running
// elements), including the optional occurrence keyword, direct and
// through stringValue's text view.
func TestParseContentValue_Element(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []ContentToken
	}{
		{
			name: "element()",
			in:   `element(pagefooter)`,
			want: []ContentToken{{Type: ContentElement, Value: "pagefooter"}},
		},
		{
			name: "element() with occurrence keyword",
			in:   `element(pagefooter, first)`,
			want: []ContentToken{{Type: ContentElement, Value: "pagefooter"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseContentValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseContentValue(%q):\n got  %#v\n want %#v", tc.in, got, tc.want)
			}
			roundTripped := stringValue(tokenizeCSSString(tc.in))
			got = ParseContentValue(roundTripped)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("round-trip(%q) → %q:\n got  %#v\n want %#v",
					tc.in, roundTripped, got, tc.want)
			}
		})
	}
}
