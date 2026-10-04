package htmlbag

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// everyBreak breaks at every legal breakpoint.
type everyBreak struct{}

func (everyBreak) Breaks(p *node.BreakProblem) []int {
	breaks := make([]int, len(p.Candidates))
	for i := range breaks {
		breaks[i] = i
	}
	return breaks
}

func TestRegisterBreakerRejectsReservedNames(t *testing.T) {
	cb, _ := newLineModelBuilder(t, "", nil)
	f := func(BreakerStyles) node.Breaker { return nil }
	for _, name := range []string{"", "  ", "auto", "Inherit", " unset "} {
		if err := cb.RegisterBreaker(name, f); err == nil {
			t.Errorf("RegisterBreaker(%q) accepted a reserved name", name)
		}
	}
	if err := cb.RegisterBreaker("every", nil); err == nil {
		t.Error("RegisterBreaker accepted a nil function")
	}
	if err := cb.RegisterBreaker("Every", f); err != nil {
		t.Errorf("RegisterBreaker(Every): %v", err)
	}
}

// -bag-line-breaker selects a registered breaker for the paragraph, which
// chooses its breaks; auto keeps Knuth-Plass, and the property is inherited.
func TestLineBreakerProperty(t *testing.T) {
	var got []BreakerStyles
	register := func(cb *CSSBuilder) {
		if err := cb.RegisterBreaker("Every", func(s BreakerStyles) node.Breaker {
			got = append(got, s)
			return everyBreak{}
		}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, css string
		lines     int
	}{
		{"none", `p { font-size: 10pt }`, 1},
		{"on the paragraph", `p { font-size: 10pt; -bag-line-breaker: EVERY }`, 4},
		{"inherited from the body", `body { -bag-line-breaker: every } p { font-size: 10pt }`, 4},
		{"auto below a breaker", `body { -bag-line-breaker: every } p { font-size: 10pt; -bag-line-breaker: auto }`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got = nil
			if n := len(lineModelLines(t, c.css, `<p lang="de">one two three four</p>`, register)); n != c.lines {
				t.Errorf("%d lines, want %d", n, c.lines)
			}
			if c.name != "on the paragraph" {
				return
			}
			want := BreakerStyles{Name: "every", FontSize: bag.MustSP("10pt"), Language: "de"}
			if len(got) == 0 || got[len(got)-1] != want {
				t.Errorf("the function got %+v, want %+v last", got, want)
			}
		})
	}
}

// An unregistered name keeps Knuth-Plass and is warned about once.
func TestLineBreakerUnregistered(t *testing.T) {
	var buf bytes.Buffer
	old := bag.Logger
	bag.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { bag.Logger = old }()
	lines := lineModelLines(t, `p { font-size: 10pt; -bag-line-breaker: missing }`, `<p>one two</p><p>three four</p>`, nil)
	if len(lines) != 2 {
		t.Errorf("%d lines, want 2, one a paragraph", len(lines))
	}
	if n := strings.Count(buf.String(), "names no registered breaker"); n != 1 {
		t.Errorf("warned %d times, want once:\n%s", n, buf.String())
	}
}
