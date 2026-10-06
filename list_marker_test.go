package htmlbag

import "testing"

// The marker of a list item is set from the item's settings, less htmlbag's
// own sentinels: lang, -bag-bookmark or page-break-inside on an <li> carried
// one into BuildNodelistFromString, which stopped the document with
// "Unknown setting".
func TestListItemSentinelsStayOffTheMarker(t *testing.T) {
	for name, li := range map[string]string{
		"lang":              `<li lang="de">Eintrag</li>`,
		"-bag-bookmark":     `<li style="-bag-bookmark: 1">Item</li>`,
		"page-break-inside": `<li style="page-break-inside: avoid">Item</li>`,
	} {
		t.Run(name, func(t *testing.T) {
			pages, _ := renderHTMLPagesCB(t, "", `<!DOCTYPE html><html><body><ul>`+li+`</ul></body></html>`)
			if len(pages) != 1 {
				t.Errorf("%d pages, want 1", len(pages))
			}
		})
	}
}
