package htmlbag

import (
	"testing"

	"github.com/boxesandglue/htmlbag/internal/cascadia"
)

// computedFor applies css to the HTML fragment and returns the computed
// styles of the element sel selects.
func computedFor(t *testing.T, css, htmlStr, sel string) StyleMap {
	t.Helper()
	c := NewCSSParser()
	if err := c.AddCSSText(css); err != nil {
		t.Fatal(err)
	}
	doc, err := c.ProcessHTMLChunk(htmlStr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyCSS(doc); err != nil {
		t.Fatal(err)
	}
	nodes := doc.Find(sel).Nodes
	if len(nodes) == 0 {
		t.Fatalf("%s matches nothing", sel)
	}
	return c.ComputedStyles(nodes[0])
}

func TestIsSelector(t *testing.T) {
	css := `
	:is(.a, .b) span { color: red; }
	p:is(.c)::before { content: "x"; }
	:is(.d, #e) { color: blue; }
	:root, :host { --c: olive; }
	#e span { color: var(--c); }
	.f { color: green; }
	:where(#e) span, :where(.g) { color: maroon; }
	.g { color: navy; }
	li:is(:nth-child(2), .x) { color: lime; }
	:is(.forgiving, ::-moz-nonsense) { color: teal; }`
	html := `<html><body>
	<div class="b"><span id="s">x</span></div>
	<p class="c" id="p">y</p>
	<div class="d f" id="d">z</div>
	<div class="f" id="e">w<span id="t">t</span></div>
	<div class="g" id="g">g</div>
	<ul><li>1</li><li id="li2">2</li></ul>
	<p class="forgiving" id="fg">f</p>
	</body></html>`
	if got := computedFor(t, css, html, "#s").Get("color"); got != "red" {
		t.Errorf("descendant of :is(): color = %q, want red", got)
	}
	if got := computedFor(t, css, html, "#p").Get("before::content"); got != `"x"` {
		t.Errorf(":is() with pseudo-element: content = %q", got)
	}
	// :is() takes the specificity of its most specific argument: the id in
	// :is(.d, #e) beats .f even for the element matched through .d.
	if got := computedFor(t, css, html, "#d").Get("color"); got != "blue" {
		t.Errorf("specificity of :is(): color = %q, want blue", got)
	}
	if got := computedFor(t, css, html, "#e").Get("color"); got != "blue" {
		t.Errorf("specificity of :is(): color = %q, want blue", got)
	}
	// `#e span` (1,0,1) beats `:where(#e) span` (0,0,1): the id inside
	// :where() does not count.
	if got := computedFor(t, css, html, "#t").Get("color"); got != "olive" {
		t.Errorf(":root, :host: color = %q, want olive", got)
	}
	// :where() has specificity 0: .g wins although it comes first.
	if got := computedFor(t, css, html, "#g").Get("color"); got != "navy" {
		t.Errorf(":where() specificity: color = %q, want navy", got)
	}
	if got := computedFor(t, css, html, "#li2").Get("color"); got != "lime" {
		t.Errorf(":is(:nth-child(2)): color = %q, want lime", got)
	}
	// :is() takes a forgiving list: the unknown pseudo-element drops out.
	if got := computedFor(t, css, html, "#fg").Get("color"); got != "teal" {
		t.Errorf("forgiving :is(): color = %q, want teal", got)
	}
}

func TestHostSelectorString(t *testing.T) {
	for _, tc := range []struct{ css, want string }{
		{`:root, :host { color: red }`, `:root, :not(*)`},
		{`:host(.x) p, :host-context(.y) { color: red }`, `:not(*) p, :not(*)`},
	} {
		bl := consumeBlock(tokenizeCSSString(tc.css), false)
		sel := selectorString(bl.blocks[0].componentValues)
		if sel != tc.want {
			t.Errorf("%s: selector = %q, want %q", tc.css, sel, tc.want)
		}
		if _, err := cascadia.ParseGroupWithPseudoElements(sel); err != nil {
			t.Errorf("cascadia rejects %q: %v", sel, err)
		}
	}
}

func TestCustomProperties(t *testing.T) {
	css := `
	:root { --main: green; --size: 12pt; --pad: 1pt 2pt; }
	.box { --main: blue; --derived: var(--main); }
	.inner { color: var(--main); font-size: var(--size); padding: var(--pad); }
	.fallback { color: var(--nope, var(--also-nope, purple)); }
	.missing { color: var(--nope); }
	.derived { color: var(--derived); }
	.cycle { --x: var(--y); --y: var(--x); color: var(--x, orange); }
	.shorthand { margin: 0 var(--w, 2em); }
	.icon { --fa: "\f015"; }
	.icon::before { content: var(--fa) / ""; }
	.pseudo::before { --fa: "\f00c"; content: var(--fa); }
	.empty { --e:; margin-left: var(--e) 3pt; }
	`
	html := `<html><body>
	<div class="box"><p class="inner derived" id="a">a</p></div>
	<p class="inner" id="b">b</p>
	<p class="fallback" id="c">c</p>
	<p class="missing" id="d">d</p>
	<p class="cycle" id="e">e</p>
	<p class="shorthand" id="f">f</p>
	<p class="icon" id="g">g</p>
	<p class="icon pseudo" id="h">h</p>
	<p class="inner" id="i" style="--main: red; --pad: 5pt">i</p>
	<p class="empty" id="j">j</p>
	</body></html>`
	for _, tc := range []struct{ sel, prop, want string }{
		{"#a", "color", "blue"},
		{"#a", "font-size", "12pt"},
		{"#b", "color", "green"},
		{"#b", "padding-left", "2pt"},
		{"#b", "padding-top", "1pt"},
		{"#c", "color", "purple"},
		{"#d", "color", ""},
		{"#e", "color", "orange"},
		{"#f", "margin-right", "2em"},
		{"#f", "margin-top", "0"},
		{"#g", "before::content", "\"\" / \"\""},
		{"#h", "before::content", "\"\""},
		{"#i", "color", "red"},
		{"#i", "padding-left", "5pt"},
		{"#j", "margin-left", "3pt"},
	} {
		got := computedFor(t, css, html, tc.sel)
		if v := got.Get(tc.prop); v != tc.want {
			t.Errorf("%s %s = %q, want %q", tc.sel, tc.prop, v, tc.want)
		}
		for k := range got {
			if _, name := splitPseudoPrefix(k); isCustomProperty(name) {
				t.Errorf("%s: custom property %s left in the computed styles", tc.sel, k)
			}
		}
	}
	// .derived resolves var(--main) on .box, where it is blue, and inherits
	// the result.
	if v := computedFor(t, css, `<div class="box"><p class="derived" id="x">x</p></div>`, "#x").Get("color"); v != "blue" {
		t.Errorf("derived color = %q, want blue", v)
	}
}
