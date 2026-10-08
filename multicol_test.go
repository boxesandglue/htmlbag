package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// The column properties are read with their shorthands, and none of them
// passes to a child.
func TestMulticolProperties(t *testing.T) {
	ih := resolveOnto(t, baseStyles(), resolveCSSText(`columns: 12em 3; column-gap: 2em; column-rule: 0.5pt solid red; column-fill: auto`))
	m := ih.multicol
	if m.count != 3 {
		t.Errorf("column-count %d, want 3 from columns: 12em 3", m.count)
	}
	if !m.gapSet || m.gap != bag.MustSP("20pt") {
		t.Errorf("column-gap %v (set %v), want 20pt", m.gap, m.gapSet)
	}
	if m.ruleWidth != bag.MustSP("0.5pt") || !m.ruleSolid || rgb(t, m.ruleColor) != rgb(t, resolveOnto(t, baseStyles(), resolveCSSText(`color: red`)).color) {
		t.Errorf("column-rule %v solid %v %s, want 0.5pt solid red", m.ruleWidth, m.ruleSolid, rgb(t, m.ruleColor))
	}
	if !m.fillAuto {
		t.Error("column-fill: auto not read")
	}

	if span := resolveOnto(t, baseStyles(), resolveCSSText(`column-span: all`)).multicol; !span.spanAll {
		t.Error("column-span: all not read")
	}
	if normal := resolveOnto(t, baseStyles(), resolveCSSText(`column-count: 2; column-gap: normal`)).multicol; normal.gapSet || normal.count != 2 {
		t.Errorf("column-gap: normal set %v, count %d; want unset, 2", normal.gapSet, normal.count)
	}
	if child := ih.Clone().multicol; child != (multicol{}) {
		t.Errorf("a child inherits the column properties: %+v", child)
	}
}
