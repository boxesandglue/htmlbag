package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// Replaced content (inline <svg>, raster <img>, embedded PDFs, …) often
// cannot determine its final size at HTML parse time because the size
// depends on the enclosing container's width. The DeferredSizer mechanism
// solves this by stashing a frontend.FormatToVList closure on a wrapper
// VList; the closure is invoked once the real container width is known
// and returns the formatted VList.
//
// FormatToVList is the same callback type already used by
// frontend.TableCell.Contents — table-cell content production is the
// canonical example of "give me a VList sized to my width" in
// boxesandglue. Reusing it here lets the cell-builder consume our
// deferred content directly: in buildTD the closure stashed on the
// wrapper goes straight into td.Contents, no wrapping layer needed.
//
// The wrapper VList carries the closure under a private node.Attributes
// key; the walker resolveDeferredSizing invokes it and replaces the
// wrapper's geometry in place. Closures MUST be idempotent — calling
// them twice at different widths must produce two correct renderings,
// not accumulate state, because layout passes may revisit the same
// wrapper.

// deferredFormatterKey is the node.Attributes key under which the
// deferred FormatToVList closure is stashed on a wrapper. Kept
// package-private so consumers route through the helpers below rather
// than poking at attributes directly.
const deferredFormatterKey = "_deferredFormatter"

// setDeferredFormatter attaches a FormatToVList closure to a wrapper
// VList. Allocates the Attributes map on demand. Idempotent: an
// existing closure is replaced.
func setDeferredFormatter(vl *node.VList, ftv frontend.FormatToVList) {
	if vl == nil {
		return
	}
	if vl.Attributes == nil {
		vl.Attributes = node.H{}
	}
	vl.Attributes[deferredFormatterKey] = ftv
}

// getDeferredFormatter returns the closure attached to a node, or nil
// if none. Both VList and HList carriers are recognised — VList is the
// canonical wrapper for replaced content in block flow, but a table
// row HList could carry one too once a future patch wires up
// row-level deferred sizing.
func getDeferredFormatter(n node.Node) frontend.FormatToVList {
	switch t := n.(type) {
	case *node.VList:
		if t.Attributes == nil {
			return nil
		}
		if ftv, ok := t.Attributes[deferredFormatterKey].(frontend.FormatToVList); ok {
			return ftv
		}
	case *node.HList:
		if t.Attributes == nil {
			return nil
		}
		if ftv, ok := t.Attributes[deferredFormatterKey].(frontend.FormatToVList); ok {
			return ftv
		}
	}
	return nil
}

// resolveDeferredSizing walks a slice of frontend.Text items and
// invokes the FormatToVList closure on any wrapper VList that carries
// one. The wrapper is rewritten in place from the closure's result,
// preserving the marker so a subsequent pass at a different width can
// re-materialise. Recurses into nested *frontend.Text items so
// closures inside wrappers (e.g. a deferred-sized image inside a
// styled <div> inside a cell) are reached.
//
// Items that aren't a *frontend.Text or a *node.VList with a marker
// are left untouched. Closures that return an error are also left
// untouched — callers see the placeholder, not a broken document.
func resolveDeferredSizing(items []any, containerWidth bag.ScaledPoint) {
	for _, itm := range items {
		switch v := itm.(type) {
		case *node.VList:
			if ftv := getDeferredFormatter(v); ftv != nil {
				result, err := ftv(containerWidth)
				if err != nil || result == nil {
					continue
				}
				// Mutate the wrapper in place with the result's
				// geometry and child list. The marker survives so a
				// subsequent layout pass can re-materialise.
				v.List = result.List
				v.Width = result.Width
				v.Height = result.Height
				v.Depth = result.Depth
			}
		case *frontend.Text:
			resolveDeferredSizing(v.Items, containerWidth)
		}
	}
}

// singleDeferredFormattedVListInText recognises the common case where
// a frontend.Text contains exactly one *node.VList carrying a deferred
// FormatToVList closure and nothing else (no other Items, no nested
// Texts). Such Texts arise from <td><svg width=…%/></td> or
// <td><img width=…%></td> — the cell holds one piece of replaced
// content and no text. Returns the wrapper VList in that case, nil
// otherwise. Whitespace-only strings are ignored; one level of
// nested-Text unwrapping is tolerated.
//
// buildTD uses this to short-circuit the cell pipeline: instead of
// invoking FormatParagraph (which mishandles inline-VList-only
// paragraphs by dropping the VList from line output), it hands the
// closure directly to frontend.TableCell.Contents.
func singleDeferredFormattedVListInText(t *frontend.Text) *node.VList {
	if t == nil {
		return nil
	}
	var realItems []any
	for _, itm := range t.Items {
		s, ok := itm.(string)
		if ok {
			if strings.TrimSpace(s) == "" {
				continue
			}
			realItems = append(realItems, itm)
			continue
		}
		realItems = append(realItems, itm)
	}
	if len(realItems) == 1 {
		if inner, ok := realItems[0].(*frontend.Text); ok {
			return singleDeferredFormattedVListInText(inner)
		}
	}
	if len(realItems) != 1 {
		return nil
	}
	vl, ok := realItems[0].(*node.VList)
	if !ok {
		return nil
	}
	if getDeferredFormatter(vl) == nil {
		return nil
	}
	return vl
}

// hasDeferredFormatterInTextTree reports whether the given
// frontend.Text (or any descendant Text reached through its Items)
// carries a node with an attached deferred FormatToVList closure. Used
// by buildTD to decide whether a non-box cell-content Text needs a
// FormatToVList wrapping closure so the deferred content can resolve
// against the real cell width before FormatParagraph runs.
func hasDeferredFormatterInTextTree(t *frontend.Text) bool {
	if t == nil {
		return false
	}
	for _, itm := range t.Items {
		switch v := itm.(type) {
		case *node.VList, *node.HList:
			if getDeferredFormatter(v.(node.Node)) != nil {
				return true
			}
		case *frontend.Text:
			if hasDeferredFormatterInTextTree(v) {
				return true
			}
		}
	}
	return false
}
