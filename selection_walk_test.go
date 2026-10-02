package htmlbag

import (
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// walkText walks the body of htmlStr and returns the text it collected.
func walkText(t *testing.T, htmlStr string) string {
	t.Helper()
	c := NewCSSParser()
	doc, err := c.ProcessHTMLChunk(htmlStr)
	if err != nil {
		t.Error(err)
		return ""
	}
	if _, err = c.ApplyCSS(doc); err != nil {
		t.Error(err)
		return ""
	}
	h := &HTMLItem{Dir: ModeVertical}
	if err = c.GetHTMLItemFromHTMLNode(doc.Find("body").Nodes[0], ModeVertical, h); err != nil {
		t.Error(err)
		return ""
	}
	var sb strings.Builder
	var collect func(*HTMLItem)
	collect = func(itm *HTMLItem) {
		if itm.Typ == html.TextNode {
			sb.WriteString(itm.Data)
		}
		for _, c := range itm.Children {
			collect(c)
		}
	}
	collect(h)
	return sb.String()
}

// Walks running at the same time keep their own white-space and tab stops:
// the state in force belongs to the walk, not to the package.
func TestWalksKeepTheirOwnWhiteSpace(t *testing.T) {
	const pre = `<html><body><p style="white-space: pre; -bag-tab-stops: 1cm">a  b	c</p></body></html>`
	const normal = `<html><body><p>a  b	c</p></body></html>`
	wantPre, wantNormal := walkText(t, pre), walkText(t, normal)
	if wantPre != "a  b\tc" || wantNormal == wantPre {
		t.Fatalf("one walk at a time: pre %q, normal %q", wantPre, wantNormal)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 50 {
				if got := walkText(t, pre); got != wantPre {
					t.Errorf("pre: got %q, want %q", got, wantPre)
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 50 {
				if got := walkText(t, normal); got != wantNormal {
					t.Errorf("normal: got %q, want %q", got, wantNormal)
					return
				}
			}
		}()
	}
	wg.Wait()
}
