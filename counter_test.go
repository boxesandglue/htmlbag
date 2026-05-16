package htmlbag

import (
	"reflect"
	"testing"
)

// TestParseCounterList covers the whitespace-separated "name [n]" pairs
// that show up in counter-reset / counter-increment values.
func TestParseCounterList(t *testing.T) {
	cases := []struct {
		in           string
		defaultValue int
		want         map[string]int
	}{
		{"section", 0, map[string]int{"section": 0}},
		{"section", 1, map[string]int{"section": 1}},
		{"section 5", 0, map[string]int{"section": 5}},
		{"section 2 sub", 1, map[string]int{"section": 2, "sub": 1}},
		{"a 1 b 2 c 3", 0, map[string]int{"a": 1, "b": 2, "c": 3}},
		{"", 0, map[string]int{}},
	}
	for _, tc := range cases {
		got := parseCounterList(tc.in, tc.defaultValue)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseCounterList(%q, %d) = %v; want %v", tc.in, tc.defaultValue, got, tc.want)
		}
	}
}

// pushWithCounters is a tiny helper that pushes a new style onto the stack
// and seeds its counter-reset / counter-increment maps, then runs
// applyCounters — mirroring what Output() does for one element.
func pushWithCounters(ss *StylesStack, resets, increments map[string]int) {
	s := ss.PushStyles()
	s.counterReset = resets
	s.counterIncrement = increments
	ss.applyCounters()
}

// TestApplyCounters_ResetCreatesInScope checks that counter-reset on an
// element creates the counter in THAT element's scope; siblings reset
// independently.
func TestApplyCounters_ResetCreatesInScope(t *testing.T) {
	ss := &StylesStack{}
	pushWithCounters(ss, map[string]int{"sec": 0}, nil) // <ol counter-reset: sec>
	if got := ss.CounterValue("sec"); got != 0 {
		t.Errorf("after reset: CounterValue(sec) = %d; want 0", got)
	}
}

// TestApplyCounters_IncrementWalksUpStack checks that counter-increment
// finds the innermost ancestor counter (e.g. <li> increments the counter
// declared on its parent <ol>) — the load-bearing case for list-item
// numbering.
func TestApplyCounters_IncrementWalksUpStack(t *testing.T) {
	ss := &StylesStack{}
	pushWithCounters(ss, map[string]int{"sec": 0}, nil) // <ol>
	pushWithCounters(ss, nil, map[string]int{"sec": 1}) // first <li>
	if got := ss.CounterValue("sec"); got != 1 {
		t.Errorf("first li: CounterValue(sec) = %d; want 1", got)
	}
	(*ss).PopStyles()                                   // close first <li>
	pushWithCounters(ss, nil, map[string]int{"sec": 1}) // second <li>
	if got := ss.CounterValue("sec"); got != 2 {
		t.Errorf("second li: CounterValue(sec) = %d; want 2 (sibling should see incremented ancestor counter)", got)
	}
}

// TestApplyCounters_NestedCountersValues exercises the counters(name, sep)
// path: a counter of the same name declared on multiple ancestor levels
// must yield ONE value per declaration, joined root-first.
func TestApplyCounters_NestedCountersValues(t *testing.T) {
	ss := &StylesStack{}
	// outer <ol counter-reset: sec 1> → root scope, value 1
	pushWithCounters(ss, map[string]int{"sec": 1}, nil)
	// outer <li counter-increment: sec> → goes to 2
	pushWithCounters(ss, nil, map[string]int{"sec": 1})
	// inner <ol counter-reset: sec> → fresh counter in this scope
	pushWithCounters(ss, map[string]int{"sec": 0}, nil)
	// inner <li counter-increment: sec> → goes to 1
	pushWithCounters(ss, nil, map[string]int{"sec": 1})
	// deepest <ol counter-reset: sec> → another fresh counter
	pushWithCounters(ss, map[string]int{"sec": 0}, nil)
	// deepest <li counter-increment: sec> → goes to 1
	pushWithCounters(ss, nil, map[string]int{"sec": 1})

	got := ss.CounterValues("sec")
	want := []int{2, 1, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CounterValues(sec) = %v; want %v (outer li, inner li, deepest li)", got, want)
	}

	// And CounterValue returns the innermost (1), not the root (2).
	if got := ss.CounterValue("sec"); got != 1 {
		t.Errorf("CounterValue(sec) = %d; want 1 (innermost wins)", got)
	}
}

// TestApplyCounters_IncrementWithoutResetCreatesParentScopeCounter
// covers the CSS Lists 3 §3.2 fallback: if counter-increment finds no
// existing counter of that name on the ancestor chain, it implicitly
// resets one at the parent scope (or current scope if at root). Subsequent
// siblings then share that implicit counter.
func TestApplyCounters_IncrementWithoutResetCreatesParentScopeCounter(t *testing.T) {
	ss := &StylesStack{}
	pushWithCounters(ss, nil, nil)                      // parent <div>
	pushWithCounters(ss, nil, map[string]int{"foo": 1}) // first <li counter-increment: foo>
	if got := ss.CounterValue("foo"); got != 1 {
		t.Errorf("first child: CounterValue(foo) = %d; want 1", got)
	}
	(*ss).PopStyles()
	pushWithCounters(ss, nil, map[string]int{"foo": 1}) // second <li>
	if got := ss.CounterValue("foo"); got != 2 {
		t.Errorf("second child: CounterValue(foo) = %d; want 2 (implicit reset at parent must outlive each child)", got)
	}
}
