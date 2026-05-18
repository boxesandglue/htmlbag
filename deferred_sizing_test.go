package htmlbag

import (
	"errors"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// fakeFormatter is a test recorder. It returns a VList whose Width
// tracks the requested container width via a multiplier — proving that
// resolveDeferredSizing reaches the closure, that idempotent
// re-resolution works at different widths, and that an error path
// leaves the wrapper untouched.
type fakeFormatter struct {
	calls      []bag.ScaledPoint
	multiplier float64 // resulting width = containerWidth * multiplier
	failNext   bool
}

func (f *fakeFormatter) FormatToVList() frontend.FormatToVList {
	return func(containerWidth bag.ScaledPoint) (*node.VList, error) {
		f.calls = append(f.calls, containerWidth)
		if f.failNext {
			f.failNext = false
			return nil, errors.New("formatter failed")
		}
		w := bag.ScaledPoint(float64(containerWidth) * f.multiplier)
		h := bag.MustSP("20pt")
		g := &node.Glyph{Width: w, Height: h}
		hl := node.Hpack(g)
		vl := node.Vpack(hl)
		vl.Width = w
		vl.Height = h
		return vl, nil
	}
}

// TestDeferredFormatterAttachAndResolve covers the basic wiring:
// attach a closure to a wrapper, run the walker, observe the closure
// was called once with the expected width and that the wrapper's
// geometry now matches what the closure returned.
func TestDeferredFormatterAttachAndResolve(t *testing.T) {
	f := &fakeFormatter{multiplier: 0.5}
	wrapper := &node.VList{Width: bag.MustSP("1pt"), Height: bag.MustSP("1pt")}
	setDeferredFormatter(wrapper, f.FormatToVList())

	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))

	if len(f.calls) != 1 {
		t.Fatalf("expected 1 FormatToVList call, got %d", len(f.calls))
	}
	if got, want := f.calls[0], bag.MustSP("200pt"); got != want {
		t.Errorf("FormatToVList containerWidth = %s, want %s", got, want)
	}
	if got, want := wrapper.Width, bag.MustSP("100pt"); got != want {
		t.Errorf("wrapper.Width after resolve = %s, want %s (200pt * 0.5)", got, want)
	}
	if wrapper.List == nil {
		t.Error("wrapper.List was not rewritten")
	}
	// The marker must survive so a second pass can re-resolve.
	if getDeferredFormatter(wrapper) == nil {
		t.Error("deferred-formatter marker was stripped after resolve; idempotence broken")
	}
}

// TestDeferredFormatterIdempotent confirms that resolving the same
// wrapper twice at different widths produces two correct renderings,
// not accumulated state. Layout passes may revisit a wrapper.
func TestDeferredFormatterIdempotent(t *testing.T) {
	f := &fakeFormatter{multiplier: 1.0}
	wrapper := &node.VList{}
	setDeferredFormatter(wrapper, f.FormatToVList())

	resolveDeferredSizing([]any{wrapper}, bag.MustSP("300pt"))
	resolveDeferredSizing([]any{wrapper}, bag.MustSP("150pt"))

	if len(f.calls) != 2 {
		t.Fatalf("expected 2 FormatToVList calls, got %d", len(f.calls))
	}
	if got, want := wrapper.Width, bag.MustSP("150pt"); got != want {
		t.Errorf("wrapper.Width after second pass = %s, want %s (last call wins, not accumulated)", got, want)
	}
}

// TestDeferredFormatterErrorLeavesWrapperAlone confirms the
// documented behavior: a closure that returns an error must not
// corrupt the wrapper. The placeholder geometry stays in place, the
// document keeps rendering.
func TestDeferredFormatterErrorLeavesWrapperAlone(t *testing.T) {
	f := &fakeFormatter{multiplier: 0.5, failNext: true}
	originalWidth := bag.MustSP("42pt")
	wrapper := &node.VList{Width: originalWidth}
	setDeferredFormatter(wrapper, f.FormatToVList())

	resolveDeferredSizing([]any{wrapper}, bag.MustSP("200pt"))

	if wrapper.Width != originalWidth {
		t.Errorf("wrapper.Width = %s, want %s (error path must not rewrite)", wrapper.Width, originalWidth)
	}
	if wrapper.List != nil {
		t.Error("wrapper.List was rewritten despite formatter error")
	}
}

// TestResolveDeferredSizingRecursesIntoText verifies the walker
// descends into nested *frontend.Text items. A real wrapper often
// sits several Text levels deep (e.g. <body><div><figure><svg/>),
// so a flat walk wouldn't find it.
func TestResolveDeferredSizingRecursesIntoText(t *testing.T) {
	f := &fakeFormatter{multiplier: 1.0}
	wrapper := &node.VList{}
	setDeferredFormatter(wrapper, f.FormatToVList())

	inner := frontend.NewText()
	inner.Items = []any{wrapper}
	outer := frontend.NewText()
	outer.Items = []any{inner}

	resolveDeferredSizing(outer.Items, bag.MustSP("100pt"))

	if len(f.calls) != 1 {
		t.Errorf("expected walker to reach nested wrapper, got %d calls", len(f.calls))
	}
}

// TestResolveDeferredSizingIgnoresUnmarkedNodes confirms the walker
// is inert for items without a marker. The leaf branch in
// buildVlistInternal calls resolveDeferredSizing unconditionally on
// every Text it processes; a regression here would slow down every
// document.
func TestResolveDeferredSizingIgnoresUnmarkedNodes(t *testing.T) {
	plain := &node.VList{Width: bag.MustSP("50pt")}
	resolveDeferredSizing([]any{plain}, bag.MustSP("200pt"))
	if plain.Width != bag.MustSP("50pt") {
		t.Errorf("plain wrapper was mutated: Width = %s", plain.Width)
	}
}

// TestGetDeferredFormatterOnHList covers the HList branch of
// getDeferredFormatter. Both node kinds are supported because a
// future patch may attach formatters at row level.
func TestGetDeferredFormatterOnHList(t *testing.T) {
	f := &fakeFormatter{multiplier: 1.0}
	ftv := f.FormatToVList()
	hl := &node.HList{}
	hl.Attributes = node.H{deferredFormatterKey: ftv}
	got := getDeferredFormatter(hl)
	if got == nil {
		t.Fatal("getDeferredFormatter(HList) returned nil")
	}
	// Function identity comparison is unreliable in Go, so call it
	// and check the recorder.
	got(bag.MustSP("100pt"))
	if len(f.calls) != 1 {
		t.Errorf("retrieved closure did not record call; got %d, want 1", len(f.calls))
	}
}
