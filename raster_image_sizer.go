package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// imageDims bundles the sizing inputs for replaced content (<img>,
// <img src="x.svg">, inline <svg>): the specified width (absolute wd
// or percent-of-container widthPct), the specified absolute height,
// and the CSS max-width constraint, again absolute (maxWd) or percent
// (maxPct). Zero values mean "not specified". When both wd and
// widthPct are set (HTML width attribute plus CSS width), the percent
// wins: CSS overrides the presentation attribute.
type imageDims struct {
	wd       bag.ScaledPoint
	widthPct float64
	ht       bag.ScaledPoint
	maxWd    bag.ScaledPoint
	maxPct   float64
}

// needsContainerWidth reports whether any dimension is
// container-relative, forcing the deferred-sizing path: the used width
// can only be computed once the real container width is known.
func (d imageDims) needsContainerWidth() bool {
	return d.widthPct > 0 || d.maxPct > 0
}

// resolveWidth maps a container width to the used width, CSS 2.1
// §10.4 style: base width (percent of container, explicit absolute,
// or natural intrinsic width), then capped by max-width. Unlike
// `width`, a max-width cap never scales an image up: a natural-size
// image narrower than the cap keeps its natural size.
//
// The max-content probe (absurdly large container, see the formatter
// comments below) keeps the pre-max-width behaviour: a percent width
// resolves to the natural width, and a percent max-width resolves to
// "no constraint"; both match the CSS max-content contribution of
// container-relative dimensions. An absolute max-width stays in force
// during the probe, so a capped image reports the capped width as its
// max-content size.
func (d imageDims) resolveWidth(containerWidth, intrinsicWd bag.ScaledPoint) bag.ScaledPoint {
	probe := containerWidth > intrinsicWd*100
	var wd bag.ScaledPoint
	switch {
	case d.widthPct > 0 && probe:
		wd = intrinsicWd
	case d.widthPct > 0:
		wd = bag.ScaledPoint(float64(containerWidth) * d.widthPct / 100.0)
	case d.wd > 0:
		wd = d.wd
	default:
		wd = intrinsicWd
	}
	if d.maxPct > 0 && !probe {
		if maxW := bag.ScaledPoint(float64(containerWidth) * d.maxPct / 100.0); wd > maxW {
			wd = maxW
		}
	}
	if d.maxWd > 0 && wd > d.maxWd {
		wd = d.maxWd
	}
	return wd
}

// newRasterImageFormatter returns a frontend.FormatToVList closure
// that rescales an already-loaded raster image (PNG/JPEG, or PDF
// import) to the width dims resolves against whatever container width
// the consumer supplies. Unlike newInlineSVGFormatter — which
// re-renders SVG geometry on every call — a raster image is loaded
// exactly once at construction time (LoadImageFile +
// CreateImageNodeFromImagefile); the closure only mutates the existing
// *node.Image's display dimensions, which is essentially free.
//
// Aspect ratio is preserved unless dims.ht > 0; then the explicit
// height wins and the width-derived scale is ignored.
//
// Same probe-vs-build behaviour as the SVG formatter: tiny
// containerWidth returns a zero-width box (min-content), absurdly
// large resolves per imageDims.resolveWidth (max-content). The image's
// natural width/height are captured into the closure at construction
// time from the loaded *node.Image.
func newRasterImageFormatter(img *node.Image, intrinsicWd, intrinsicHt bag.ScaledPoint, dims imageDims) frontend.FormatToVList {
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

		newWd := dims.resolveWidth(containerWidth, intrinsicWd)

		var newHt bag.ScaledPoint
		switch {
		case dims.ht > 0:
			newHt = dims.ht
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
