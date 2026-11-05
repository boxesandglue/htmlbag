package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// defaultBleed is bleed: auto beside crop marks (CSS Paged Media 3 §7.2.2).
const defaultBleed = 6 * bag.Factor

// PageMarks holds the print production side of an @page rule. The zero value
// declares nothing, so a document setting made elsewhere stays.
type PageMarks struct {
	// Bleed is how far the content may run past the trimmed sheet.
	Bleed bag.ScaledPoint
	// Crop asks for crop marks.
	Crop bool
}

// Marks reads the bleed and marks descriptors of the rule. The backend knows
// one bleed and one kind of mark per document, so the caller takes them from
// the rule of the first page, before that page exists.
func (p *Page) Marks() PageMarks {
	var m PageMarks
	res := resolveDeclarations(p.Attributes)
	if v, ok := res["marks"]; ok {
		for mark := range strings.FieldsSeq(v.String()) {
			switch mark {
			case "none":
			case "crop":
				m.Crop = true
			default:
				bag.Logger.Warn("@page marks: only crop marks are supported, ignoring it", "value", mark)
			}
		}
	}
	bleed := "auto"
	if v, ok := res["bleed"]; ok {
		bleed = strings.TrimSpace(v.String())
	}
	if bleed == "auto" {
		if m.Crop {
			m.Bleed = defaultBleed
		}
		return m
	}
	wd, err := bag.SP(bleed)
	if err != nil || wd < 0 {
		bag.Logger.Warn("@page bleed: not a length of zero or more, ignoring it", "value", bleed)
		return m
	}
	m.Bleed = wd
	return m
}
