package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
)

func TestParseHyphenCharacter(t *testing.T) {
	text := func(s string) *string { return &s }
	for _, tc := range []struct {
		value string
		want  *string
		valid bool
	}{
		{"auto", nil, true},
		{"AUTO", nil, true},
		{"initial", nil, true},
		{`"="`, text("="), true},
		{`'='`, text("="), true},
		{`""`, text(""), true},
		{`"=="`, text("=="), true},
		{`"\""`, text(`"`), true},
		{`"\\"`, text(`\`), true},
		{`"a" "b"`, nil, false},
		{"=", nil, false},
		{"inherit", nil, false},
		{"", nil, false},
	} {
		got, valid := parseHyphenCharacter(tc.value)
		if valid != tc.valid || (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
			t.Errorf("parseHyphenCharacter(%q) = %v, %v, want %v, %v", tc.value, got, valid, tc.want, tc.valid)
		}
	}
}

// lineTexts returns the characters of each line.
func lineTexts(t *testing.T, css, body string) []string {
	t.Helper()
	var texts []string
	for _, line := range lineModelLines(t, `p{font-family:serif;font-size:10pt;hyphens:auto}`+css, body, nil) {
		var b strings.Builder
		for e := line.List; e != nil; e = e.Next() {
			switch v := e.(type) {
			case *node.Glyph:
				b.WriteString(v.Components)
			case *node.Glue:
				if v.Width > 0 {
					b.WriteByte(' ')
				}
			}
		}
		texts = append(texts, strings.TrimSpace(b.String()))
	}
	return texts
}

const hyphenText = `<p lang="en">supercalifragilisticexpialidocious supercalifragilisticexpialidocious supercalifragilisticexpialidocious</p>`

func TestHyphenateCharacter(t *testing.T) {
	for _, tc := range []struct {
		name, css, end string
	}{
		{"the font's hyphen", ``, "-"},
		{"auto", `p{hyphenate-character:auto}`, "-"},
		{"a string", `p{hyphenate-character:"="}`, "="},
		{"several characters", `p{hyphenate-character:"=="}`, "=="},
		{"not ASCII", `p{hyphenate-character:"‐"}`, "‐"},
		{"a CSS escape", `p{hyphenate-character:"\2010"}`, "‐"},
		{"inherited", `body{hyphenate-character:"="}`, "="},
		{"auto replaces", `body{hyphenate-character:"="}p{hyphenate-character:auto}`, "-"},
		{"inherit", `body{hyphenate-character:"="}p{hyphenate-character:inherit}`, "="},
		{"an invalid value changes nothing", `body{hyphenate-character:"="}p{hyphenate-character:equals}`, "="},
		{"a glyph the font does not have", `p{hyphenate-character:"😀"}`, "-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := lineTexts(t, tc.css, hyphenText)
			if len(lines) < 2 {
				t.Fatalf("no hyphenation: %q", lines)
			}
			broken := 0
			for _, line := range lines[:len(lines)-1] {
				if strings.HasSuffix(line, "s") {
					continue
				}
				if !strings.HasSuffix(line, tc.end) {
					t.Errorf("line %q does not end in %q", line, tc.end)
				}
				broken++
			}
			if broken == 0 {
				t.Errorf("no line breaks in a word: %q", lines)
			}
		})
	}
}

// The empty string sets no text at all.
func TestHyphenateCharacterEmpty(t *testing.T) {
	lines := lineTexts(t, `p{hyphenate-character:""}`, hyphenText)
	for _, line := range lines {
		if strings.Contains(line, "-") {
			t.Errorf("a hyphen in %q", line)
		}
	}
	if joined := strings.Join(lines, ""); strings.Count(joined, "supercalifragilisticexpialidocious") != 3 {
		t.Errorf("text lost: %q", lines)
	}
}

// A soft hyphen takes it as well, also where the patterns are off.
func TestHyphenateCharacterSoftHyphen(t *testing.T) {
	lines := lineTexts(t, `p{hyphens:manual;hyphenate-character:"="}`, "<p>aaaa­bbbb­cccc­dddd­eeee­ffff­gggg­hhhh­iiii</p>")
	if len(lines) < 2 {
		t.Fatalf("no break: %q", lines)
	}
	for _, line := range lines[:len(lines)-1] {
		if !strings.HasSuffix(line, "=") {
			t.Errorf("line %q does not end in =", line)
		}
	}
}

// A span's value holds for the breaks inside the span.
func TestHyphenateCharacterOnASpan(t *testing.T) {
	lines := lineTexts(t, `span{hyphenate-character:"="}`, `<p lang="en"><span>supercalifragilisticexpialidocious supercalifragilisticexpialidocious</span> supercalifragilisticexpialidocious supercalifragilisticexpialidocious supercalifragilisticexpialidocious</p>`)
	var eq, dash int
	for _, line := range lines {
		switch {
		case strings.HasSuffix(line, "="):
			eq++
		case strings.HasSuffix(line, "-"):
			dash++
		}
	}
	if eq == 0 || dash == 0 {
		t.Errorf("want both, got %q", lines)
	}
}
