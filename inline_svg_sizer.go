package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/svgreader"
)

// newInlineSVGFormatter returns a frontend.FormatToVList closure that
// renders an inline <svg> element with width: pct% against whatever
// container width the consumer supplies. The svgreader.Document is
// parsed once at construction time (potentially expensive); the cheap-
// to-redo step — CreateSVGNodeFromDocument at the resolved width — is
// what the closure wraps. Successive calls at different widths produce
// independently correct renderings (idempotent contract).
//
// Probe vs. build calls: frontend.TableCell.minWidth and maxWidth
// invoke a cell's FormatToVList closure with extreme widths (1pt and
// MaxSP) to discover min-content and max-content widths. A naive
// formatter would respond with "1pt" and "MaxSP" — both wrong for an
// SVG with width: pct. The CSS intrinsic-sizing convention is:
// min-content = 0, max-content = natural viewBox width. The closure
// approximates that here: a tiny incoming width returns a near-zero
// rendering (the table sees minWidth ≈ 0, can squeeze the column); an
// absurdly large incoming width returns a natural-viewBox rendering
// (the table sees maxWidth = natural; if other cells want less, the
// SVG happily shrinks to share). The "normal" range materialises at
// pct of containerWidth as expected.
//
// Geometry trick. CreateSVGNodeFromDocument returns a Rule whose Pre
// stream uses the SVG-spec Y-flip (cm 0 0 -1 0 0) and paints from
// origin going "down" in flipped coords — which lands BELOW the origin
// in PDF coords. The default Rule-emit path expects Pre to paint UP
// from the line's baseline into the rule's Height slot. To match the
// downward-painting SVG without changing the backend, the returned
// Rule's reserved space is declared as Depth rather than Height — a
// pure-Depth rule's bottom edge sits at baseline-Depth, which is
// exactly where the downward Pre lands.
//
// Backend dispatch trick. The Rule is wrapped in an HList (not a bare
// Vpack) so outputVerticalItems → outputHorizontalItems dispatches
// correctly when the wrapper VList is nested inside the cell's
// "vertical cell part" VList. A bare Rule sitting directly under a
// nested VList would not reach the horizontal emit path that knows
// how to write the Pre stream with positioning.
func newInlineSVGFormatter(doc *svgreader.Document, pct float64, explicitHt bag.ScaledPoint, df *frontend.Document) frontend.FormatToVList {
	return func(containerWidth bag.ScaledPoint) (*node.VList, error) {
		tr := frontend.NewSVGTextRenderer(df)

		naturalW := bag.ScaledPointFromFloat(doc.Width)
		if naturalW <= 0 {
			naturalW = bag.MustSP("100pt")
		}

		// Min-content probe: return an honest zero-width box. CSS
		// min-content for replaced content with width: pct is zero —
		// the cell layout then knows it can squeeze the column.
		if containerWidth <= bag.MustSP("5pt") {
			empty := node.NewRule()
			empty.Hide = true
			hpack := node.Hpack(empty)
			vl := node.Vpack(hpack)
			vl.Width = 0
			vl.Height = 0
			vl.Depth = 0
			return vl, nil
		}

		var wd bag.ScaledPoint
		if containerWidth > naturalW*100 {
			// Max-content probe.
			wd = naturalW
		} else {
			wd = bag.ScaledPoint(float64(containerWidth) * pct / 100.0)
		}
		svgNode := df.Doc.CreateSVGNodeFromDocument(doc, wd, explicitHt, tr)

		// Pure-Depth rule (see Geometry trick above).
		svgHeight := svgNode.Height
		svgNode.Height = 0
		svgNode.Depth = svgHeight

		// HList carrier (see Backend dispatch trick above).
		hpack := node.Hpack(svgNode)
		hpack.Width = svgNode.Width
		hpack.Height = 0
		hpack.Depth = svgHeight

		vl := node.Vpack(hpack)
		vl.Width = svgNode.Width
		vl.Height = 0
		vl.Depth = svgHeight
		return vl, nil
	}
}

// parseSVGPercentWidth interprets an SVG width="…" attribute.
// Returns (pct, true) for "50%", "100 %", …; (0, false) for everything
// else (absolute lengths, unitless numbers, missing, malformed). The
// percent must be > 0 to count.
func parseSVGPercentWidth(raw string) (float64, bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	rest, ok := strings.CutSuffix(s, "%")
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return f, true
}
