package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

const runningFooterCSS = `@page {
    size: a4;
    margin: 20mm 20mm 30mm 20mm;
    @bottom-center { content: element(pagefooter); }
}
.pagefooter { position: running(pagefooter); font-size: 8pt; }
.pagefooter table { width: 100%; border-collapse: collapse; }
.pagefooter td { vertical-align: top; }`

const runningFooterHTML = `<footer class="pagefooter">
  <table>
    <tr>
      <td>Muster GmbH<br>Beispielstraße 12</td>
      <td>IBAN: DE12 3456 7890</td>
      <td>USt-ID: DE123456789</td>
    </tr>
  </table>
</footer>`

func fillerParagraphs(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "<p>Absatz %d: Fließtext der über mehrere Seiten laufen soll, damit der wiederkehrende Seitenfuß auf jeder Seite geprüft werden kann.</p>\n", i)
	}
	return sb.String()
}

// footerObjectY returns the page Y coordinate of the object carrying the
// footer text, or -1 when the footer is not on the page.
func footerObjectY(pg *document.Page) bag.ScaledPoint {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectComponents(obj.Vlist.List, &sb)
		if strings.Contains(sb.String(), "IBAN") {
			return obj.Y
		}
	}
	return -1
}

// TestRunningElementFooter: a footer removed from the flow via
// `position: running(pagefooter)` and placed with
// `@page { @bottom-center { content: element(pagefooter) } }` must appear
// on every page of a two-page document, at the same fixed position inside
// the bottom margin band, without repeating in the body flow.
func TestRunningElementFooter(t *testing.T) {
	html := `<html><body>` + runningFooterHTML + fillerParagraphs(60) + `</body></html>`
	pages := renderHTMLPages(t, runningFooterCSS, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	for i, pg := range pages {
		txt := pageText(pg)
		for _, needle := range []string{"MusterGmbH", "IBAN", "USt-ID"} {
			if got := strings.Count(txt, strings.ReplaceAll(needle, " ", "")); got != 1 {
				t.Errorf("page %d: footer text %q found %d times, want exactly once", i+1, needle, got)
			}
		}
	}
	// Fixed position: the footer starts at the top of the bottom margin
	// band (margin-bottom: 30mm) on every page, regardless of how much
	// body content the page carries.
	wantY := bag.MustSP("30mm")
	for i, pg := range pages {
		y := footerObjectY(pg)
		if y == -1 {
			t.Fatalf("page %d: footer object not found", i+1)
		}
		if y != wantY {
			t.Errorf("page %d: footer at y=%s, want %s (top edge of the bottom margin band)", i+1, y, wantY)
		}
	}
}

// paragraphY returns the page Y coordinate of the object carrying the
// given text, or -1 when it is not on the page.
func paragraphY(pg *document.Page, needle string) bag.ScaledPoint {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectComponents(obj.Vlist.List, &sb)
		if strings.Contains(sb.String(), needle) {
			return obj.Y
		}
	}
	return -1
}

// TestRunningElementNotInFlow: the running element must not occupy space
// or render at its source position in the normal flow. The body paragraph
// must land at exactly the Y it gets in a control render without the
// footer element.
func TestRunningElementNotInFlow(t *testing.T) {
	html := `<html><body>` + runningFooterHTML + `<p>EINZIGER ABSATZ</p></body></html>`
	pages := renderHTMLPages(t, runningFooterCSS, html)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	txt := pageText(pages[0])
	if !strings.Contains(txt, "EINZIGERABSATZ") {
		t.Fatal("body paragraph missing")
	}
	if got := strings.Count(txt, "IBAN"); got != 1 {
		t.Errorf("footer text found %d times, want exactly once (margin box only)", got)
	}
	control := renderHTMLPages(t, runningFooterCSS, `<html><body><p>EINZIGER ABSATZ</p></body></html>`)
	wantY := paragraphY(control[0], "EINZIGERABSATZ")
	gotY := paragraphY(pages[0], "EINZIGERABSATZ")
	if wantY == -1 || gotY == -1 {
		t.Fatal("body paragraph object not found")
	}
	if gotY != wantY {
		t.Errorf("body paragraph at y=%s, control without footer has y=%s (footer must not occupy flow space)", gotY, wantY)
	}
}
