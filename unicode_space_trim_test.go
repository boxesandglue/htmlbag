package htmlbag

import (
	"strings"
	"testing"
)

// At the start of a block only document white space goes (CSS Text 3
// §4.1.1). A thin space or a no-break space is text and stays.
func TestBlockStartKeepsUnicodeSpaces(t *testing.T) {
	for _, sp := range []string{" ", " ", " "} {
		te := renderToText(t, "<!DOCTYPE html><html><body><p>\n  "+sp+"x</p></body></html>")
		got := strings.Join(collectStrings(te.Items), "|")
		if !strings.Contains(got, sp+"x") || strings.Contains(got, "\n") {
			t.Errorf("%U: segments %q, want the space before x and no newline", []rune(sp)[0], got)
		}
	}
}
