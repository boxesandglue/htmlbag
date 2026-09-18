package htmlbag

import (
	"strconv"
	"strings"

	scanner "github.com/speedata/css"
)

// declaration is one CSS declaration that survived the cascade for a single
// element. ApplyCSS collects these per node in cascade order, and the order is
// load-bearing: shorthand expansion in resolveDeclarations runs in sequence, so
//
//	border-left-style: dotted;
//	border-left: thick green;
//
// leaves the left border solid-less ("none") because the shorthand resets the
// style the longhand had set.
type declaration struct {
	property string
	value    tokenstream
}

// StyleValue is the value of one computed CSS declaration. It carries the
// parsed token stream, so consumers that need the structure -- content lists,
// url() references, function calls -- read tokens instead of re-parsing text
// that a serialiser has already flattened.
type StyleValue struct {
	toks tokenstream
	text string
}

// String returns the CSS text of the value. Properties whose value is a plain
// keyword, length or color read this; anything with internal structure should
// use the tokens instead.
func (v StyleValue) String() string { return v.text }

// tokens returns the value's token stream. Unexported on purpose: the token
// type belongs to the CSS scanner and is not part of the package's API.
func (v StyleValue) tokens() tokenstream { return v.toks }

// isEmpty reports whether the value carries nothing at all.
func (v StyleValue) isEmpty() bool { return v.text == "" && len(v.toks) == 0 }

// uri returns the target of a url() value and whether the value is one. The
// token keeps the path the CSS parser already resolved against the declaring
// stylesheet, so no unwrapping and no second file lookup is needed.
func (v StyleValue) uri() (string, bool) {
	for _, tok := range v.toks {
		if tok.Type == scanner.S {
			continue
		}
		if tok.Type == scanner.URI {
			return tok.Value, true
		}
		return "", false
	}
	return "", false
}

// function returns the name and arguments of a value that is a single function
// call, e.g. `running(header)` or `leader(".")`.
func (v StyleValue) function() (name string, args tokenstream, ok bool) {
	toks := trimSpace(v.toks)
	if len(toks) == 0 || toks[0].Type != scanner.Function {
		return "", nil, false
	}
	args = toks[1:]
	if n := len(args); n > 0 && args[n-1].Type == scanner.Delim && args[n-1].Value == ")" {
		args = args[:n-1]
	}
	return toks[0].Value, args, true
}

// tokenValue builds a StyleValue from parsed tokens.
func tokenValue(toks tokenstream) StyleValue {
	return StyleValue{toks: toks, text: stringValue(toks)}
}

// textValue builds a StyleValue from plain CSS text. Used for values the
// package synthesises itself (shorthand defaults such as "none" or "1pt") and
// for callers that only have a string to begin with.
func textValue(s string) StyleValue {
	return StyleValue{toks: tokenizeCSSString(s), text: s}
}

// StyleMap holds the computed CSS declarations of one element, keyed by
// property name. Shorthands are already expanded into their longhands.
// Pseudo-element declarations keep their prefix, e.g. "before::content".
type StyleMap map[string]StyleValue

// Get returns the CSS text of a property, or the empty string when the
// property is not set.
func (sm StyleMap) Get(key string) string { return sm[key].text }

// Strings returns the map in its flattened, text-only form. For callers
// outside the package that only need the computed values as strings.
func (sm StyleMap) Strings() map[string]string {
	out := make(map[string]string, len(sm))
	for k, v := range sm {
		out[k] = v.text
	}
	return out
}

// declarationsFromText parses a CSS declaration block -- the body of a rule or
// the contents of a style="..." attribute -- into declarations in source
// order.
func declarationsFromText(css string) []declaration {
	var tokens tokenstream
	s := scanner.New(css)
	for {
		token := s.Next()
		if token.Type == scanner.EOF || token.Type == scanner.Error {
			break
		}
		if token.Type == scanner.Comment {
			continue
		}
		tokens = append(tokens, token)
	}
	block := consumeBlock(tokens, true)
	decls := make([]declaration, 0, len(block.rules))
	for _, rule := range block.rules {
		decls = append(decls, declaration{property: stringValue(rule.key), value: rule.value})
	}
	return decls
}

// splitComponents splits a value token stream into its top-level,
// whitespace-separated components. A function call is one component including
// its arguments, so `cmyk(1,0,0,0) red` yields two components -- a split on
// the serialised text could never get that right, which is why the old string
// path needed a colorMatcher special case for `background`.
func splitComponents(toks tokenstream) []tokenstream {
	var components []tokenstream
	var current tokenstream
	depth := 0
	flush := func() {
		if len(current) > 0 {
			components = append(components, current)
			current = nil
		}
	}
	for _, tok := range toks {
		switch {
		case tok.Type == scanner.S && depth == 0:
			flush()
			continue
		case tok.Type == scanner.Function:
			depth++
		case tok.Type == scanner.Delim && tok.Value == ")" && depth > 0:
			depth--
		}
		current = append(current, tok)
	}
	flush()
	return components
}

// componentValues splits a value into its top-level components as StyleValues.
func componentValues(toks tokenstream) []StyleValue {
	parts := splitComponents(toks)
	values := make([]StyleValue, 0, len(parts))
	for _, p := range parts {
		values = append(values, tokenValue(p))
	}
	return values
}

// fourValues fills top, right, bottom and left from the one to four components
// of a margin/padding/border-width shorthand.
func fourValues(toks tokenstream) map[string]StyleValue {
	parts := componentValues(toks)
	// All four sides are always present, even for a shorthand with no usable
	// components, so an expansion never leaves a longhand simply unset.
	out := map[string]StyleValue{"top": {}, "right": {}, "bottom": {}, "left": {}}
	switch len(parts) {
	case 1:
		out["top"], out["right"], out["bottom"], out["left"] = parts[0], parts[0], parts[0], parts[0]
	case 2:
		out["top"], out["bottom"] = parts[0], parts[0]
		out["right"], out["left"] = parts[1], parts[1]
	case 3:
		out["top"] = parts[0]
		out["right"], out["left"] = parts[1], parts[1]
		out["bottom"] = parts[2]
	case 4:
		out["top"], out["right"], out["bottom"], out["left"] = parts[0], parts[1], parts[2], parts[3]
	}
	return out
}

// borderShorthand splits `1pt solid black` into its three longhands. Unlike the
// old word scanner it classifies whole components, so a function-valued color
// (`cmyk(0,0,0,1)`) arrives in one piece instead of being reassembled from the
// remaining words.
func borderShorthand(toks tokenstream) (width, style, color StyleValue) {
	width, style, color = textValue("1pt"), textValue("none"), textValue("currentcolor")
	for _, part := range componentValues(toks) {
		t := part.String()
		if ok, wd := isDimension(t); ok {
			width = textValue(wd)
			continue
		}
		if ok, sty := isBorderStyle(t); ok {
			style = textValue(sty)
			continue
		}
		color = part
	}
	return width, style, color
}

// resolveDeclarations turns the cascaded declarations of one element into the
// computed style map, expanding every shorthand into its longhands. This
// replaces the former attribute round trip: the declarations arrive as tokens
// and stay tokens, so nothing is stringified and re-parsed on the way.
func resolveDeclarations(decls []declaration) StyleMap {
	resolved := make(StyleMap, len(decls))
	set := func(key string, value StyleValue) { resolved[key] = value }

	for _, decl := range decls {
		key, value := decl.property, tokenValue(decl.value)
		switch key {
		case "margin", "padding":
			for loc, v := range fourValues(decl.value) {
				set(key+"-"+loc, v)
			}
		case "list-style":
			for _, part := range componentValues(decl.value) {
				switch txt := part.String(); txt {
				case "inside", "outside":
					set("list-style-position", part)
				default:
					if _, isURL := part.uri(); isURL {
						set("list-style-image", part)
					} else {
						set("list-style-type", part)
					}
				}
			}
		case "border":
			wd, sty, col := borderShorthand(decl.value)
			for _, loc := range toprightbottomleft {
				set("border-"+loc+"-style", sty)
				set("border-"+loc+"-width", wd)
				set("border-"+loc+"-color", col)
			}
		case "border-radius":
			for _, lr := range []string{"left", "right"} {
				for _, tb := range []string{"top", "bottom"} {
					set("border-"+tb+"-"+lr+"-radius", value)
				}
			}
		case "border-top", "border-right", "border-bottom", "border-left":
			wd, sty, col := borderShorthand(decl.value)
			set(key+"-width", wd)
			set(key+"-style", sty)
			set(key+"-color", col)
		case "border-color", "border-style":
			longhand := strings.TrimPrefix(key, "border-")
			for loc, v := range fourValues(decl.value) {
				set("border-"+loc+"-"+longhand, v)
			}
		case "border-width":
			for loc, v := range fourValues(decl.value) {
				set("border-"+loc+"-width", v)
			}
			set(key, value)
		case "font":
			// The shorthand must carry <font-size> and <font-family>; style,
			// variant, weight and stretch precede the size, line-height
			// follows it after a slash, and the family is last. The size is
			// the anchor: everything before it is a keyword, everything after
			// it is the family. Style, weight and line-height reset to their
			// initial value when omitted, as the shorthand demands. CSS allows
			// whitespace around the slash, so `10pt / 12pt` is glued into one
			// component first.
			parts := componentValues(glueSlash(decl.value))
			sizeIdx := -1
			for idx, part := range parts {
				if isFontSizeValue(part) {
					sizeIdx = idx
					break
				}
			}
			if sizeIdx < 0 || sizeIdx == len(parts)-1 {
				// Without a size or a family the shorthand is invalid and
				// the declaration is dropped, as CSS demands.
				break
			}
			set("font-style", textValue("normal"))
			set("font-weight", textValue("normal"))
			set("line-height", textValue("normal"))
			for _, part := range parts[:sizeIdx] {
				switch txt := part.String(); txt {
				case "italic", "oblique":
					set("font-style", part)
				case "bold", "bolder", "lighter":
					set("font-weight", part)
				default:
					if _, err := strconv.Atoi(txt); err == nil {
						set("font-weight", part)
					}
				}
			}
			size, lh, hasLineHeight := splitSlash(parts[sizeIdx])
			set("font-size", size)
			if hasLineHeight {
				set("line-height", lh)
			}
			family := make([]string, 0, len(parts)-sizeIdx-1)
			for _, part := range parts[sizeIdx+1:] {
				family = append(family, part.String())
			}
			set("font-family", textValue(strings.Join(family, " ")))
		case "text-decoration":
			for _, part := range componentValues(decl.value) {
				switch part.String() {
				case "none", "underline", "overline", "line-through":
					set("text-decoration-line", part)
				case "solid", "double", "dotted", "dashed", "wavy":
					set("text-decoration-style", part)
				default:
					set("text-decoration-color", part)
				}
			}
		case "background":
			// background-clip, background-color, background-image,
			// background-origin, background-position, background-repeat,
			// background-size and background-attachment all live here; only
			// the color is read so far. The last component wins, matching the
			// previous behaviour.
			for _, part := range componentValues(decl.value) {
				set("background-color", part)
			}
		default:
			set(key, value)
		}
	}

	// Default the style only when nothing has set it. This used to run
	// unconditionally, overwriting a style parsed from the shorthand above or
	// declared through the text-decoration-style longhand.
	if line, ok := resolved["text-decoration-line"]; ok && line.String() != "none" {
		if _, set := resolved["text-decoration-style"]; !set {
			resolved["text-decoration-style"] = textValue("solid")
		}
	}
	return resolved
}

// isFontSizeValue reports whether v can stand in the size slot of the `font`
// shorthand: a length, a percentage or one of the absolute or relative size
// keywords, each optionally followed by a slash and the line-height. A bare
// number is a weight, not a size.
func isFontSizeValue(v StyleValue) bool {
	size, _, _ := splitSlash(v)
	s := size.String()
	switch s {
	case "xx-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large", "smaller", "larger":
		return true
	}
	if _, err := strconv.Atoi(s); err == nil {
		return false
	}
	return dimen.MatchString(s) || strings.HasSuffix(s, "%")
}

// splitSlash splits a `size/line-height` component at the slash delimiter. The
// third result is false when the component has no slash, size is then v itself.
func splitSlash(v StyleValue) (size, lineHeight StyleValue, ok bool) {
	for i, tok := range v.toks {
		if tok.Type == scanner.Delim && tok.Value == "/" {
			return tokenValue(v.toks[:i]), tokenValue(v.toks[i+1:]), true
		}
	}
	return v, StyleValue{}, false
}

// glueSlash drops the whitespace tokens next to a "/" delimiter, so that
// `10pt / 12pt` forms one component like `10pt/12pt` does.
func glueSlash(toks tokenstream) tokenstream {
	out := make(tokenstream, 0, len(toks))
	for i, tok := range toks {
		if tok.Type == scanner.S {
			if i > 0 && isSlash(toks[i-1]) || i+1 < len(toks) && isSlash(toks[i+1]) {
				continue
			}
		}
		out = append(out, tok)
	}
	return out
}

func isSlash(tok *scanner.Token) bool {
	return tok.Type == scanner.Delim && tok.Value == "/"
}
