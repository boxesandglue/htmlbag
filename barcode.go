package htmlbag

import (
	"fmt"
	"image/color"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
	"github.com/speedata/barcode"
	"github.com/speedata/barcode/code128"
	"github.com/speedata/barcode/ean"
	"github.com/speedata/barcode/qr"
)

// BarcodeType represents the type of barcode to create.
type BarcodeType int

const (
	BarcodeEAN13 BarcodeType = iota
	BarcodeCode128
	BarcodeQR
)

// parseBarcodeType converts a string to a BarcodeType.
func parseBarcodeType(s string) (BarcodeType, error) {
	switch s {
	case "ean13", "EAN13":
		return BarcodeEAN13, nil
	case "code128", "Code128":
		return BarcodeCode128, nil
	case "qrcode", "QRCode", "qr", "QR":
		return BarcodeQR, nil
	default:
		return 0, fmt.Errorf("unknown barcode type %q", s)
	}
}

// parseQRECLevel converts a string to a qr.ErrorCorrectionLevel.
func parseQRECLevel(s string) qr.ErrorCorrectionLevel {
	switch s {
	case "L":
		return qr.L
	case "Q":
		return qr.Q
	case "H":
		return qr.H
	default:
		return qr.M
	}
}

// createBarcode creates a barcode node from the given parameters.
func createBarcode(typ BarcodeType, value string, width, height bag.ScaledPoint, doc *frontend.Document, eclevel qr.ErrorCorrectionLevel) (*node.VList, error) {
	if height == 0 {
		height = width
	}

	var n node.Node
	switch typ {
	case BarcodeEAN13:
		bc, err := ean.Encode(value)
		if err != nil {
			return nil, err
		}
		if n, err = barcode1d(bc, width, height, doc); err != nil {
			return nil, err
		}
	case BarcodeCode128:
		bc, err := code128.Encode(value)
		if err != nil {
			return nil, err
		}
		if n, err = barcode1d(bc, width, height, doc); err != nil {
			return nil, err
		}
	case BarcodeQR:
		bc, err := qr.Encode(value, eclevel, qr.Auto)
		if err != nil {
			return nil, err
		}
		return barcode2d(bc, width, doc)
	}

	vl := node.Vpack(n)
	vl.Attributes = node.H{"origin": "barcode"}
	return vl, nil
}

// barcode1d renders a 1D barcode (EAN13, Code128) as a PDF rule node.
func barcode1d(bc barcode.Barcode, width, height bag.ScaledPoint, doc *frontend.Document) (node.Node, error) {
	dx := bc.Bounds().Dx()
	wdBar := width / bag.ScaledPoint(dx)
	bgcolor := doc.GetColor("black")

	d := pdfdraw.New()
	d.Save()
	d.Color(*bgcolor)
	curX := bag.ScaledPoint(0)
	for i := range dx {
		at := bc.At(i, 0)
		col, _, _, _ := at.RGBA()
		if col == 0 {
			d.Rect(curX, 0, wdBar, -height)
			d.Fill()
		}
		curX += wdBar
	}

	rule := node.NewRule()
	rule.Pre = d.String()
	rule.Height = height
	rule.Width = width
	rule.Hide = true
	rule.Post = pdfdraw.New().Restore().String()
	return rule, nil
}

// barcode2d renders a 2D barcode (QR code) as a PDF rule node.
func barcode2d(bc barcode.Barcode, width bag.ScaledPoint, doc *frontend.Document) (*node.VList, error) {
	dx := bc.Bounds().Dx()
	dy := bc.Bounds().Dy()

	wdRect := width / bag.ScaledPoint(dx)
	bgcolor := doc.GetColor("black")

	curX := bag.ScaledPoint(0)
	curY := bag.ScaledPoint(-wdRect)

	d := pdfdraw.New()
	d.Save()
	d.Color(*bgcolor)
	delta := bag.Factor / 100

	for y := range dy {
		curX = 0
		for x := range dx {
			col := bc.At(x, y).(color.Gray16)
			if col.Y == 0 {
				d.Rect(curX-delta, curY-delta, wdRect+2*delta, wdRect+2*delta).Fill()
			}
			curX += wdRect
		}
		curY -= wdRect
	}

	rule := node.NewRule()
	rule.Pre = d.String()
	rule.Hide = true
	rule.Post = pdfdraw.New().Restore().String()

	vl := node.Vpack(rule)
	vl.Attributes = node.H{"origin": "barcode"}
	vl.Width = width
	vl.Height = width
	return vl, nil
}
