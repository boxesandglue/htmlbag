package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// insetValue is one argument of inset(): a length, or a percentage of the
// image's natural width (left, right) or height (top, bottom).
type insetValue struct {
	length bag.ScaledPoint
	pct    float64
	isPct  bool
}

func (v insetValue) resolve(natural bag.ScaledPoint) bag.ScaledPoint {
	if v.isPct {
		return bag.ScaledPoint(float64(natural) * v.pct / 100)
	}
	return v.length
}

// objectViewBox is a parsed object-view-box: inset(), in the order top,
// right, bottom, left.
type objectViewBox struct {
	inset [4]insetValue
}

// parseObjectViewBox reads an object-view-box value. It returns nil for none
// (and an empty value), and nil with a warning for anything it does not
// support.
func parseObjectViewBox(v string, fontsize, rootFontsize bag.ScaledPoint) *objectViewBox {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" || s == "none" {
		return nil
	}
	inner, ok := strings.CutPrefix(s, "inset(")
	if ok {
		inner, ok = strings.CutSuffix(inner, ")")
	}
	if !ok {
		bag.Logger.Warn("object-view-box supports none and inset(), ignoring it", "value", v)
		return nil
	}
	fields := strings.Fields(inner)
	for i, f := range fields {
		if f == "round" {
			bag.Logger.Warn("object-view-box: the round corners of inset() are ignored", "value", v)
			fields = fields[:i]
			break
		}
	}
	if len(fields) < 1 || len(fields) > 4 {
		bag.Logger.Warn("object-view-box: inset() needs one to four lengths or percentages, ignoring it", "value", v)
		return nil
	}
	vals := make([]insetValue, len(fields))
	for i, f := range fields {
		iv, ok := parseInsetValue(f, fontsize, rootFontsize)
		if !ok {
			bag.Logger.Warn("object-view-box: inset() needs one to four lengths or percentages, ignoring it", "value", v)
			return nil
		}
		vals[i] = iv
	}
	ovb := &objectViewBox{}
	switch len(vals) {
	case 1:
		ovb.inset = [4]insetValue{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		ovb.inset = [4]insetValue{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		ovb.inset = [4]insetValue{vals[0], vals[1], vals[2], vals[1]}
	case 4:
		ovb.inset = [4]insetValue{vals[0], vals[1], vals[2], vals[3]}
	}
	return ovb
}

func parseInsetValue(f string, fontsize, rootFontsize bag.ScaledPoint) (insetValue, bool) {
	if p, ok := strings.CutSuffix(f, "%"); ok {
		pct, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return insetValue{}, false
		}
		return insetValue{pct: pct, isPct: true}, true
	}
	if f == "0" {
		return insetValue{}, true
	}
	if strings.HasSuffix(f, "em") {
		if _, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(f, "em"), "r"), 64); err != nil {
			return insetValue{}, false
		}
		return insetValue{length: ParseRelativeSize(f, fontsize, rootFontsize)}, true
	}
	sp, err := bag.SP(f)
	if err != nil {
		return insetValue{}, false
	}
	return insetValue{length: sp}, true
}

// region returns the view box in the image's natural units (points, which for
// a bitmap are its pixels), given its natural size. It reports false when the
// insets leave no area.
func (ovb *objectViewBox) region(naturalWd, naturalHt bag.ScaledPoint) (*node.ImageCrop, bool) {
	top := ovb.inset[0].resolve(naturalHt)
	right := ovb.inset[1].resolve(naturalWd)
	bottom := ovb.inset[2].resolve(naturalHt)
	left := ovb.inset[3].resolve(naturalWd)
	wd := naturalWd - left - right
	ht := naturalHt - top - bottom
	if wd <= 0 || ht <= 0 {
		return nil, false
	}
	return &node.ImageCrop{X: left.ToPT(), Y: top.ToPT(), Width: wd.ToPT(), Height: ht.ToPT()}, true
}

// imageCrop resolves the object-view-box of an image with the given natural
// size. It returns nil when there is none to apply.
func imageCrop(item *HTMLItem, naturalWd, naturalHt, fontsize, rootFontsize bag.ScaledPoint) *node.ImageCrop {
	v := item.Styles.Get("object-view-box")
	ovb := parseObjectViewBox(v, fontsize, rootFontsize)
	if ovb == nil || naturalWd <= 0 || naturalHt <= 0 {
		return nil
	}
	crop, ok := ovb.region(naturalWd, naturalHt)
	if !ok {
		bag.Logger.Warn("object-view-box: inset() leaves no area of the image, ignoring it", "value", v)
		return nil
	}
	return crop
}
