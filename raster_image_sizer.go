package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// imageDims bundles the sizing inputs for replaced content (<img>,
// <img src="x.svg">, inline <svg>): the specified width (absolute wd
// or percent-of-container widthPct), the specified absolute height,
// and the CSS max-width constraint, again absolute (maxWd) or percent
// (maxPct). Zero values mean "not specified". replacedElementDims fills
// it from the attributes and the CSS of the element; at most one of wd
// and widthPct is set then.
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

// replacedElementDims reads the size an <img> or inline <svg> asks for.
// The width and height attributes are presentational hints, the lowest
// level of the cascade, so CSS width, height and max-width override
// them. CSS auto drops the hint, the side then follows from the other
// one or from the natural size. A percentage height resolves against
// the height of the containing block, which is not known here, and
// counts as auto, as it does in a browser for a block of auto height.
// em resolves against fontsize, the font size of the element itself,
// rem against rootsize.
func replacedElementDims(item *HTMLItem, fontsize, rootsize bag.ScaledPoint) imageDims {
	var d imageDims
	if v := item.Attributes["width"]; v != "" {
		// bag.MustSP panics on "100%", so the percentage goes first.
		if pct, isPct := parseSVGPercentWidth(v); isPct {
			d.widthPct = pct
		} else if sp, err := bag.SP(v); err == nil {
			d.wd = sp
		}
	}
	if v := item.Attributes["height"]; v != "" {
		if sp, err := bag.SP(v); err == nil {
			d.ht = sp
		}
	}
	if v := item.Styles.Get("width"); v != "" {
		if pct, isPct := parseSVGPercentWidth(v); isPct {
			d.wd, d.widthPct = 0, pct
		} else if sp, ok := replacedLength(v, fontsize, rootsize); ok {
			d.wd, d.widthPct = sp, 0
		} else if strings.EqualFold(strings.TrimSpace(v), "auto") {
			d.wd, d.widthPct = 0, 0
		}
	}
	if v := item.Styles.Get("height"); v != "" {
		if sp, ok := replacedLength(v, fontsize, rootsize); ok {
			d.ht = sp
		} else if strings.EqualFold(strings.TrimSpace(v), "auto") || strings.HasSuffix(strings.TrimSpace(v), "%") {
			d.ht = 0
		}
	}
	if v := item.Styles.Get("max-width"); v != "" {
		// A percentage resolves against the container (deferred path),
		// a length caps eagerly. Keywords (none, max-content, …) mean
		// no cap.
		if pct, isPct := parseSVGPercentWidth(v); isPct {
			d.maxPct = pct
		} else if sp, ok := replacedLength(v, fontsize, rootsize); ok {
			d.maxWd = sp
		}
	}
	return d
}

// replacedLength reads a CSS length, absolute or in em or rem. Keywords,
// percentages and numbers without a unit report false. Unlike
// ParseRelativeSize, which reads font sizes, it does not take a bare
// number as a factor or an unknown value as the font size.
func replacedLength(v string, fontsize, rootsize bag.ScaledPoint) (bag.ScaledPoint, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if sp, err := bag.SP(v); err == nil {
		return sp, true
	}
	for _, unit := range []string{"rem", "em"} {
		if num, ok := strings.CutSuffix(v, unit); ok {
			if _, err := strconv.ParseFloat(num, 64); err != nil {
				return 0, false
			}
			return ParseRelativeSize(v, fontsize, rootsize), true
		}
	}
	return 0, false
}

// eagerWidth is the width to render an image at that needs no container
// width: wd, or the absolute max-width if the width asked for, or the
// natural width, exceeds it. Zero leaves the width to the renderer,
// which derives it from the height or takes the natural size.
func (d imageDims) eagerWidth(intrinsicWd, intrinsicHt bag.ScaledPoint) bag.ScaledPoint {
	if d.maxWd <= 0 {
		return d.wd
	}
	if d.naturalWidth(intrinsicWd, intrinsicHt) > d.maxWd {
		return d.maxWd
	}
	return d.wd
}

// naturalWidth is the width without a container: wd, or the width the
// height gives at the aspect ratio of the image, or the intrinsic width.
func (d imageDims) naturalWidth(intrinsicWd, intrinsicHt bag.ScaledPoint) bag.ScaledPoint {
	switch {
	case d.wd > 0:
		return d.wd
	case d.ht > 0 && intrinsicHt > 0:
		return bag.ScaledPoint(float64(intrinsicWd) * float64(d.ht) / float64(intrinsicHt))
	}
	return intrinsicWd
}

// resolveWidth maps a container width to the used width, CSS 2.1
// §10.4 style: base width (percent of container, explicit absolute,
// the width the height gives, or natural intrinsic width), then
// capped by max-width. Unlike
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
func (d imageDims) resolveWidth(containerWidth, intrinsicWd, intrinsicHt bag.ScaledPoint) bag.ScaledPoint {
	probe := containerWidth > intrinsicWd*100
	var wd bag.ScaledPoint
	switch {
	case d.widthPct > 0 && probe:
		wd = intrinsicWd
	case d.widthPct > 0:
		wd = bag.ScaledPoint(float64(containerWidth) * d.widthPct / 100.0)
	default:
		wd = d.naturalWidth(intrinsicWd, intrinsicHt)
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

		newWd := dims.resolveWidth(containerWidth, intrinsicWd, intrinsicHt)

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
