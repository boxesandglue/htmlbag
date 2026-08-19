package htmlbag

import (
	"reflect"
	"testing"
)

func TestCSSFontFeatureSettings(t *testing.T) {
	testdata := []struct {
		input string
		want  []string
	}{
		{`"sups" 1`, []string{"sups=1"}},
		{`"sups"`, []string{"sups=1"}},
		{`"liga" off`, []string{"liga=0"}},
		{`"liga" on`, []string{"liga=1"}},
		{`"sups" 1, "liga" 0`, []string{"sups=1", "liga=0"}},
		{`'salt' 2`, []string{"salt=2"}},
		{``, nil},
	}
	for _, td := range testdata {
		if got := cssFontFeatureSettings(td.input); !reflect.DeepEqual(got, td.want) {
			t.Errorf("cssFontFeatureSettings(%q) = %v, want %v", td.input, got, td.want)
		}
	}
}
