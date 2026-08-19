package htmlbag

import (
	"strings"
	"unicode"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// applyInitialLetter implements CSS initial-letter (dropcaps): the block's
// first letter — including any leading punctuation, per css-inline-3 —
// is removed from the paragraph text and re-set as a large glyph whose
// cap height spans from the first line's cap line down to the baseline of
// line N. The paragraph indents its first N rows to make room
// (SettingIndentLeft / SettingIndentLeftRows) and the initial rides along
// as a zero-width prepend box painted into the carved-out corner, reusing
// the outside-marker anchoring of the list markers.
func applyInitialLetter(blockte *frontend.Text, styles *FormattingStyles, df *frontend.Document) error {
	n := styles.initialLetterLines

	// The paragraph is the first inner Text with a leading string item.
	var para *frontend.Text
	for _, itm := range blockte.Items {
		if t, ok := itm.(*frontend.Text); ok {
			para = t
			break
		}
	}
	if para == nil {
		// A pure inline paragraph keeps its items directly on blockte.
		para = blockte
	}
	if len(para.Items) == 0 {
		return nil
	}
	first, ok := para.Items[0].(string)
	if !ok || first == "" {
		return nil
	}

	// Split off leading punctuation (opening quotes and the like) plus
	// exactly one letter.
	runes := []rune(first)
	cut := 0
	for cut < len(runes) && (unicode.IsPunct(runes[cut]) || unicode.IsSymbol(runes[cut])) {
		cut++
	}
	if cut < len(runes) {
		cut++
	}
	initial := string(runes[:cut])
	para.Items[0] = strings.TrimLeft(string(runes[cut:]), " \t")

	// Geometry: the initial's cap height covers N-1 full line advances
	// plus the body's cap height, so its top aligns with the first
	// line's cap line and its baseline with line N's baseline.
	if styles.fontfamily == nil {
		return nil
	}
	fs, err := styles.fontfamily.GetFontSource(styles.Fontweight, styles.fontstyle)
	if err != nil {
		return err
	}
	face, err := df.LoadFace(fs)
	if err != nil {
		return err
	}
	upem := int64(face.UnitsPerEM)
	var capFU int64
	if face.Shaper != nil && face.Shaper.Face() != nil {
		capFU = int64(face.Shaper.Face().CapHeight())
	}
	if capFU <= 0 || upem <= 0 {
		// No cap height in the font: assume the typical 0.7 em.
		capFU = upem * 7 / 10
	}
	leading := styles.lineheight
	if styles.lineheightFactor != 0 {
		leading = bag.ScaledPoint(float64(styles.Fontsize) * styles.lineheightFactor)
	}
	if leading == 0 {
		leading = styles.Fontsize * 12 / 10
	}
	capBody := bag.ScaledPoint(int64(styles.Fontsize) * capFU / upem)
	target := bag.ScaledPoint(n-1)*leading + capBody
	initialSize := bag.ScaledPoint(int64(target) * upem / capFU)

	set := frontend.TypesettingSettings{
		frontend.SettingFontFamily: styles.fontfamily,
		frontend.SettingSize:       initialSize,
	}
	if styles.color != nil {
		set[frontend.SettingColor] = styles.color
	}
	nl, err := df.BuildNodelistFromString(set, initial)
	if err != nil {
		return err
	}
	content := node.Hpack(nl)
	gap := styles.Fontsize / 3
	indent := content.Width + gap

	// Zero-width box: a negative glue walks from the anchor (the
	// paragraph's indented content origin, stamped by FormatParagraph
	// via the outside-marker attribute) back to the block's left edge,
	// the initial paints there, a fil glue absorbs the rest. Height and
	// depth are zeroed so line 1 keeps its normal leading; the shift
	// drops the initial's baseline onto line N.
	leftShift := node.NewGlue()
	leftShift.Width = -indent
	fill := node.NewGlue()
	fill.Stretch = 1 * bag.Factor
	fill.StretchOrder = node.StretchFil
	node.InsertAfter(leftShift, leftShift, content)
	node.InsertAfter(leftShift, content, fill)
	hbox := node.HpackTo(leftShift, 0)
	hbox.Height = 0
	hbox.Depth = 0
	hbox.Shift = -bag.ScaledPoint(n-1) * leading
	if hbox.Attributes == nil {
		hbox.Attributes = node.H{}
	}
	hbox.Attributes["outside-marker"] = true

	// The settings go on the block Text: a pure-inline paragraph is
	// handed to FormatParagraph as-is, while for a box container the
	// vlist builder relocates prepend and indents onto the first child
	// paragraph.
	blockte.Settings[frontend.SettingPrepend] = hbox
	blockte.Settings[frontend.SettingIndentLeft] = indent
	blockte.Settings[frontend.SettingIndentLeftRows] = n
	return nil
}
