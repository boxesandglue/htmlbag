package htmlbag

import (
	"fmt"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// CreateVlist builts a vlist (a vertical list) from the Text object.
func (cb *CSSBuilder) CreateVlist(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	vl, err := cb.buildVlistInternal(te, wd)
	if err != nil {
		return nil, err
	}
	return vl, nil
}

func (cb *CSSBuilder) buildVlistInternal(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	var prependvl *node.VList
	if prepend, ok := te.Settings[frontend.SettingPrepend]; ok {
		if p, ok := prepend.(node.Node); ok {
			hl := node.Hpack(p)
			prependvl = node.Vpack(hl)
			prependvl.Attributes = node.H{"origin": "v prepend in HTML mode"}
		}
	}

	if isBox, ok := settings[frontend.SettingBox]; ok && isBox.(bool) {
		vls := node.NewVList()
		vls.Attributes = node.H{"origin": "buildVListInternal"}
		for _, itm := range te.Items {
			switch t := itm.(type) {
			case *frontend.Text:
				if dbg, ok := t.Settings[frontend.SettingDebug].(string); ok && dbg == "table" {
					return cb.buildTable(t, wd)
				}
				vl, err := cb.buildVlistInternal(t, wd)
				if err != nil {
					return nil, err
				}
				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), vl)
				if vl.Width > vls.Width {
					vls.Width = vl.Width
				}
				vls.Height += vl.Height
				vls.Depth = vl.Depth
			case string:
				fmt.Println("~~> string")
			default:
				fmt.Println("~~> bvi unknown", t)
			}
		}
		return vls, nil
	}
	vl, _, err := cb.frontend.FormatParagraph(te, wd)
	if err != nil {
		return nil, err
	}
	if prependvl != nil {
		vl = node.InsertBefore(vl, vl, prependvl).(*node.VList)
		hl := node.Hpack(vl)
		vl = node.Vpack(hl)
	}
	return vl, nil
}
