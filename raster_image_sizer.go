package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// newRasterImageFormatter returns a frontend.FormatToVList closure
// that rescales an already-loaded raster image (PNG/JPEG, or PDF
// import) to fit pct% of whatever container width the consumer
// supplies. Unlike newInlineSVGFormatter — which re-renders SVG
// geometry on every call — a raster image is loaded exactly once at
// construction time (LoadImageFile + CreateImageNodeFromImagefile);
// the closure only mutates the existing *node.Image's display
// dimensions, which is essentially free.
//
// Aspect ratio is preserved unless explicitHt > 0; then the explicit
// height wins and the width-derived scale is ignored.
//
// Same probe-vs-build behaviour as the SVG formatter: tiny
// containerWidth returns a zero-width box (min-content), absurdly
// large returns the image at its natural intrinsic dimensions
// (max-content). The image's natural width/height are captured into
// the closure at construction time from the loaded *node.Image.
func newRasterImageFormatter(img *node.Image, intrinsicWd, intrinsicHt bag.ScaledPoint, pct float64, explicitHt bag.ScaledPoint) frontend.FormatToVList {
	return func(containerWidth bag.ScaledPoint) (*node.VList, error) {
		// Min-content probe: zero-width box.
		if containerWidth <= bag.MustSP("5pt") {
			img.Width = 0
			img.Height = 0
			vl := node.Vpack(img)
			vl.Width = 0
			vl.Height = 0
			return vl, nil
		}

		var newWd bag.ScaledPoint
		if containerWidth > intrinsicWd*100 {
			// Max-content probe: natural intrinsic width.
			newWd = intrinsicWd
		} else {
			newWd = bag.ScaledPoint(float64(containerWidth) * pct / 100.0)
		}

		var newHt bag.ScaledPoint
		switch {
		case explicitHt > 0:
			newHt = explicitHt
		case intrinsicWd > 0:
			newHt = bag.ScaledPoint(float64(intrinsicHt) * float64(newWd) / float64(intrinsicWd))
		default:
			newHt = intrinsicHt
		}
		img.Width = newWd
		img.Height = newHt
		vl := node.Vpack(img)
		vl.Width = newWd
		vl.Height = newHt
		return vl, nil
	}
}
