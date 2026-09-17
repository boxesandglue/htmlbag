package htmlbag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/frontend"
	scanner "github.com/speedata/css"
)

// ColorDefinition is a named color from a @-bag-color rule. The descriptors
// mirror the attributes of the DefineColor command in xts and the speedata
// Publisher:
//
//	@-bag-color muted { value: #6A6A6A; }
//	@-bag-color brand { model: cmyk; c: 0; m: 80; y: 90; k: 10; }
//	@-bag-color spot  { model: spotcolor; colorname: "PANTONE 300 C"; c: 100; m: 44; y: 0; k: 0; }
//
// Without a model the value is any color that CSS accepts, including a
// previously defined name. The models cmyk, rgb and gray take components
// from 0 to 100 (percentages are fine too), RGB and GRAY from 0 to 255. A spot
// color takes the ink name and optional cmyk components for the fallback
// tint; the ink name defaults to the color name.
type ColorDefinition struct {
	Name      string
	Model     string
	Value     string
	Colorname string
	// Components holds the r, g, b, c, m, y, k descriptors as written.
	Components map[string]string
	// resolved caches the color so repeated registrations reuse the same
	// object (a spot color must be registered only once).
	resolved *color.Color
}

// colorDescriptorValue returns the plain text of a descriptor value: a quoted
// string loses its quotes, everything else is the serialized value.
func colorDescriptorValue(toks tokenstream) string {
	toks = trimSpace(toks)
	if len(toks) == 1 && toks[0].Type == scanner.String {
		return toks[0].Value
	}
	return strings.TrimSpace(stringValue(toks))
}

// doColor parses a @-bag-color rule and appends it to the color definitions.
// The color itself is created when the definitions are registered with a
// document, because a value may refer to other named colors.
func (c *CSS) doColor(atrule *sBlock) error {
	cd := ColorDefinition{
		Name:       strings.TrimSpace(stringValue(atrule.componentValues)),
		Components: make(map[string]string),
	}
	if cd.Name == "" {
		return fmt.Errorf("@-bag-color: missing color name")
	}
	for _, rule := range atrule.rules {
		key := strings.TrimSpace(rule.key.String())
		value := colorDescriptorValue(rule.value)
		switch key {
		case "model":
			cd.Model = value
		case "value":
			cd.Value = value
		case "colorname":
			cd.Colorname = value
		case "r", "g", "b", "c", "m", "y", "k":
			cd.Components[key] = value
		default:
			return fmt.Errorf("@-bag-color %s: unknown descriptor %q", cd.Name, key)
		}
	}
	switch cd.Model {
	case "cmyk", "rgb", "RGB", "gray", "GRAY", "spotcolor":
	case "":
		if cd.Value == "" {
			return fmt.Errorf("@-bag-color %s: a model or a value is required", cd.Name)
		}
	default:
		return fmt.Errorf("@-bag-color %s: model %q not recognized", cd.Name, cd.Model)
	}
	c.Colors = append(c.Colors, cd)
	return nil
}

// colorComponent converts a descriptor such as "80", "80%" or "0.5" to the
// 0..1 range. A percentage is always relative to 100, a plain number to max.
func (cd *ColorDefinition) colorComponent(name string, max float64) (float64, error) {
	s, ok := cd.Components[name]
	if !ok {
		return 0, fmt.Errorf("@-bag-color %s: model %s needs the descriptor %s", cd.Name, cd.Model, name)
	}
	if pct, isPct := strings.CutSuffix(s, "%"); isPct {
		s = pct
		max = 100
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("@-bag-color %s: cannot parse %s: %q", cd.Name, name, s)
	}
	return f / max, nil
}

// optionalComponent is like colorComponent but returns 0 for a missing
// descriptor.
func (cd *ColorDefinition) optionalComponent(name string, max float64) (float64, error) {
	if _, ok := cd.Components[name]; !ok {
		return 0, nil
	}
	return cd.colorComponent(name, max)
}

// Color creates the color for the definition. Colors given by value are
// resolved against the document, so they can refer to other named colors.
func (cd *ColorDefinition) Color(fe *frontend.Document) (*color.Color, error) {
	if cd.resolved != nil {
		return cd.resolved, nil
	}
	col := &color.Color{}
	var err error
	switch cd.Model {
	case "cmyk":
		col.Space = color.ColorCMYK
		if col.C, err = cd.colorComponent("c", 100); err != nil {
			return nil, err
		}
		if col.M, err = cd.colorComponent("m", 100); err != nil {
			return nil, err
		}
		if col.Y, err = cd.colorComponent("y", 100); err != nil {
			return nil, err
		}
		if col.K, err = cd.colorComponent("k", 100); err != nil {
			return nil, err
		}
	case "rgb", "RGB":
		col.Space = color.ColorRGB
		max := 100.0
		if cd.Model == "RGB" {
			max = 255
		}
		if col.R, err = cd.colorComponent("r", max); err != nil {
			return nil, err
		}
		if col.G, err = cd.colorComponent("g", max); err != nil {
			return nil, err
		}
		if col.B, err = cd.colorComponent("b", max); err != nil {
			return nil, err
		}
	case "gray", "GRAY":
		col.Space = color.ColorGray
		max := 100.0
		if cd.Model == "GRAY" {
			max = 255
		}
		if col.G, err = cd.colorComponent("g", max); err != nil {
			return nil, err
		}
	case "spotcolor":
		col.Space = color.ColorSpotcolor
		col.Basecolor = cd.Colorname
		if col.C, err = cd.optionalComponent("c", 100); err != nil {
			return nil, err
		}
		if col.M, err = cd.optionalComponent("m", 100); err != nil {
			return nil, err
		}
		if col.Y, err = cd.optionalComponent("y", 100); err != nil {
			return nil, err
		}
		if col.K, err = cd.optionalComponent("k", 100); err != nil {
			return nil, err
		}
	default:
		c := fe.GetColor(cd.Value)
		if c == nil {
			return nil, fmt.Errorf("@-bag-color %s: cannot parse color value %q", cd.Name, cd.Value)
		}
		col = c
	}
	cd.resolved = col
	return col, nil
}

// AddColorsFromCSS defines the colors from the @-bag-color rules in the
// frontend document, so that the names can be used in any CSS color property.
// Registering the same CSS object again is harmless.
func AddColorsFromCSS(cs *CSS, fe *frontend.Document) error {
	for i := range cs.Colors {
		cd := &cs.Colors[i]
		col, err := cd.Color(fe)
		if err != nil {
			return err
		}
		fe.DefineColor(cd.Name, col)
	}
	return nil
}
