package htmlbag

import (
	"strings"

	scanner "github.com/speedata/css"
	"golang.org/x/net/html"
)

// CSS custom properties (CSS Variables Level 1). A declaration whose name
// starts with "--" defines a custom property; var(--name) or
// var(--name, fallback) in any other declaration is replaced by its value.
// Custom properties inherit, so the cascade is resolved top down: an element
// sees the values of its ancestors unless it sets its own. Pseudo-elements
// inherit from their originating element.
//
// resolveCustomProperties runs once at the end of ApplyCSS. Afterwards the
// computed declarations contain no custom properties and no var() anymore,
// so nothing downstream has to know about them.

// customProps holds the computed values of the custom properties of one
// element. A nil map means "none set". Maps are shared between elements and
// never modified after they are built.
type customProps map[string]tokenstream

func isCustomProperty(name string) bool {
	return strings.HasPrefix(name, "--")
}

// resolveCustomProperties walks the DOM from n and substitutes var()
// references in every computed declaration.
func (c *CSS) resolveCustomProperties(n *html.Node, inherited customProps) {
	if n.Type == html.ElementNode || n.Type == html.DocumentNode {
		if decls, ok := c.computed[n]; ok {
			inherited, c.computed[n] = resolveVarDeclarations(decls, inherited)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.resolveCustomProperties(child, inherited)
	}
}

// resolveVarDeclarations computes the custom properties of one element from
// its own declarations and the inherited values, and returns them together
// with the remaining declarations, var() replaced. A declaration whose var()
// cannot be resolved is invalid at computed-value time and dropped, so the
// property falls back to its inherited or initial value.
func resolveVarDeclarations(decls []declaration, inherited customProps) (customProps, []declaration) {
	own := map[string]tokenstream{}
	pseudoOwn := map[string]map[string]tokenstream{}
	hasVar := false
	for _, d := range decls {
		pseudo, name := splitPseudoPrefix(d.property)
		if isCustomProperty(name) {
			if pseudo == "" {
				own[name] = d.value
			} else {
				if pseudoOwn[pseudo] == nil {
					pseudoOwn[pseudo] = map[string]tokenstream{}
				}
				pseudoOwn[pseudo][name] = d.value
			}
		} else if containsVar(d.value) {
			hasVar = true
		}
	}
	if len(own) == 0 && len(pseudoOwn) == 0 && !hasVar {
		return inherited, decls
	}

	props := computeCustomProps(own, inherited)
	pseudoProps := map[string]customProps{}
	for pseudo, defs := range pseudoOwn {
		pseudoProps[pseudo] = computeCustomProps(defs, props)
	}

	out := decls[:0:0]
	for _, d := range decls {
		pseudo, name := splitPseudoPrefix(d.property)
		if isCustomProperty(name) {
			continue
		}
		if !containsVar(d.value) {
			out = append(out, d)
			continue
		}
		env := props
		if pp, ok := pseudoProps[pseudo]; ok {
			env = pp
		}
		value, ok := substituteVars(d.value, func(name string) (tokenstream, bool) {
			v, ok := env[name]
			return v, ok
		}, 0)
		if !ok {
			continue
		}
		out = append(out, declaration{property: d.property, value: trimSpace(value)})
	}
	return props, out
}

// splitPseudoPrefix splits "before::color" into "before" and "color".
func splitPseudoPrefix(property string) (pseudo, name string) {
	if p, n, ok := strings.Cut(property, "::"); ok {
		return p, n
	}
	return "", property
}

// computeCustomProps resolves the custom property definitions of one element
// against each other and against the inherited values.
func computeCustomProps(defs map[string]tokenstream, inherited customProps) customProps {
	if len(defs) == 0 {
		return inherited
	}
	props := make(customProps, len(inherited)+len(defs))
	for k, v := range inherited {
		props[k] = v
	}
	const (
		pending = iota + 1
		active
		done
	)
	state := make(map[string]int, len(defs))
	for name := range defs {
		state[name] = pending
	}
	var resolve func(name string) (tokenstream, bool)
	resolve = func(name string) (tokenstream, bool) {
		switch state[name] {
		case 0:
			// Not defined here: the inherited value.
			v, ok := inherited[name]
			return v, ok
		case active:
			// A reference cycle: the reference is invalid.
			return nil, false
		case done:
			v, ok := props[name]
			return v, ok
		}
		state[name] = active
		raw := trimSpace(defs[name])
		var value tokenstream
		ok := true
		switch cssWideKeyword(raw) {
		case "initial":
			ok = false
		case "inherit", "unset", "revert", "revert-layer":
			value, ok = inherited[name]
		default:
			value, ok = substituteVars(raw, resolve, 0)
			if !ok {
				// Invalid at computed-value time: custom properties
				// inherit, so the inherited value is used.
				value, ok = inherited[name]
			}
		}
		if state[name] == active {
			state[name] = done
		}
		if ok {
			props[name] = value
		} else {
			delete(props, name)
		}
		return value, ok
	}
	for name := range defs {
		resolve(name)
	}
	return props
}

// cssWideKeyword returns the keyword if the value is a single CSS-wide
// keyword.
func cssWideKeyword(toks tokenstream) string {
	if len(toks) != 1 || toks[0].Type != scanner.Ident {
		return ""
	}
	switch kw := strings.ToLower(toks[0].Value); kw {
	case "initial", "inherit", "unset", "revert", "revert-layer":
		return kw
	}
	return ""
}

// containsVar reports whether the value has a var() reference.
func containsVar(toks tokenstream) bool {
	for _, tok := range toks {
		if tok.Type == scanner.Function && strings.EqualFold(tok.Value, "var") {
			return true
		}
	}
	return false
}

// maxVarDepth bounds the nesting of var() fallbacks, which an author can
// make arbitrarily deep.
const maxVarDepth = 32

// substituteVars replaces every var() in toks. lookup returns the value of a
// custom property. The result is false when a reference has neither a value
// nor a fallback.
func substituteVars(toks tokenstream, lookup func(string) (tokenstream, bool), depth int) (tokenstream, bool) {
	if !containsVar(toks) {
		return toks, true
	}
	if depth > maxVarDepth {
		return nil, false
	}
	var out tokenstream
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if tok.Type != scanner.Function || !strings.EqualFold(tok.Value, "var") {
			out = append(out, tok)
			continue
		}
		end := matchingParen(toks, i)
		if end < 0 {
			return nil, false
		}
		args := toks[i+1 : end]
		i = end

		j := 0
		for j < len(args) && args[j].Type == scanner.S {
			j++
		}
		if j == len(args) || args[j].Type != scanner.Ident || !isCustomProperty(args[j].Value) {
			return nil, false
		}
		name := args[j].Value
		j++
		for j < len(args) && args[j].Type == scanner.S {
			j++
		}
		var fallback tokenstream
		hasFallback := false
		if j < len(args) {
			if args[j].Type != scanner.Delim || args[j].Value != "," {
				return nil, false
			}
			hasFallback = true
			fallback = trimSpace(args[j+1:])
		}
		if value, ok := lookup(name); ok {
			out = append(out, value...)
			continue
		}
		if !hasFallback {
			return nil, false
		}
		value, ok := substituteVars(fallback, lookup, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, value...)
	}
	return out, true
}

// matchingParen returns the index of the ")" that closes the function token
// at toks[start], or -1. The scanner emits the closing parenthesis as a
// delimiter; nested functions and plain parentheses each open one level.
func matchingParen(toks tokenstream, start int) int {
	depth := 0
	for i := start; i < len(toks); i++ {
		tok := toks[i]
		switch {
		case tok.Type == scanner.Function:
			depth++
		case tok.Type == scanner.Delim && tok.Value == "(":
			depth++
		case tok.Type == scanner.Delim && tok.Value == ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
