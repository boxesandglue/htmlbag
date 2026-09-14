package htmlbag

import (
	"strings"

	scanner "github.com/speedata/css"
)

// flattenNestedBlocks recursively flattens CSS nesting. Nested selector blocks
// are converted into top-level blocks by combining the parent selector with
// the child selector. The & token is replaced with the parent selector; if no
// & is present, the parent selector is prepended as an ancestor (descendant
// combinator).
//
// Examples:
//
//	.card { & > h2 { color: red } }         → .card > h2 { color: red }
//	.card { h2 { color: red } }             → .card h2 { color: red }
//	h2 { .card & { color: red } }           → .card h2 { color: red }
//	.a { &.b { color: red } }               → .a.b { color: red }
func flattenNestedBlocks(blocks []*sBlock) []*sBlock {
	var result []*sBlock
	for _, block := range blocks {
		result = append(result, flattenBlock(block, nil)...)
	}
	return result
}

// flattenBlock flattens a single block. parentSelector is nil for top-level blocks.
func flattenBlock(block *sBlock, parentSelector tokenstream) []*sBlock {
	var result []*sBlock

	// Build the effective selector for this block.
	var effectiveSelector tokenstream
	if parentSelector == nil {
		effectiveSelector = block.componentValues
	} else {
		effectiveSelector = combineSelectors(parentSelector, block.componentValues)
	}

	// This block itself becomes a flat block (with only rules, no nested blocks).
	if len(block.rules) > 0 {
		flat := &sBlock{
			componentValues: effectiveSelector,
			rules:           block.rules,
		}
		result = append(result, flat)
	}

	// Recursively flatten nested blocks.
	for _, child := range block.blocks {
		result = append(result, flattenBlock(child, effectiveSelector)...)
	}

	return result
}

// combineSelectors combines a parent selector with a child selector.
// If the child contains &, it is replaced with the parent selector.
// If no & is present, the parent is prepended with a space (descendant combinator).
func combineSelectors(parent, child tokenstream) tokenstream {
	parent = trimTrailingSpace(parent)
	child = trimTrailingSpace(child)
	if hasAmpersand(child) {
		return substituteAmpersand(parent, child)
	}
	// No &: prepend parent with space (descendant combinator).
	combined := make(tokenstream, 0, len(parent)+1+len(child))
	combined = append(combined, parent...)
	combined = append(combined, &scanner.Token{Type: scanner.S, Value: " "})
	combined = append(combined, child...)
	return combined
}

// trimTrailingSpace removes trailing whitespace tokens.
func trimTrailingSpace(toks tokenstream) tokenstream {
	for len(toks) > 0 && toks[len(toks)-1].Type == scanner.S {
		toks = toks[:len(toks)-1]
	}
	return toks
}

// hasAmpersand returns true if the token stream contains an & delimiter.
func hasAmpersand(toks tokenstream) bool {
	for _, tok := range toks {
		if tok.Type == scanner.Delim && tok.Value == "&" {
			return true
		}
	}
	return false
}

// substituteAmpersand replaces each & in child with the parent selector.
// Handles cases like:
//
//	& > h2    → parent > h2
//	&.active  → parent.active  (no extra space)
//	.wrap &   → .wrap parent
func substituteAmpersand(parent, child tokenstream) tokenstream {
	var result tokenstream
	for _, tok := range child {
		if tok.Type == scanner.Delim && tok.Value == "&" {
			result = append(result, parent...)
		} else {
			result = append(result, tok)
		}
	}
	return result
}

// selectorString converts a tokenstream to its string representation,
// suitable for passing to cascadia. It produces correct CSS selector syntax:
// no space before . # : (they attach to the preceding simple selector),
// but preserves whitespace tokens as descendant combinators.
func selectorString(toks tokenstream) string {
	var sb strings.Builder
	for _, tok := range toks {
		switch tok.Type {
		case scanner.S:
			sb.WriteByte(' ')
		case scanner.Function:
			sb.WriteString(tok.Value)
			sb.WriteByte('(')
		case scanner.Hash:
			// Value already contains "#" prefix from fixupComponentValues
			sb.WriteString(tok.Value)
		case scanner.String:
			// The scanner strips the quotes; without re-quoting,
			// [href="#x"] would reach cascadia as [href=#x].
			sb.WriteString(cssQuoteString(tok.Value))
		// The attribute match operators carry their meaning in the token
		// type; the scanner sets Value to "" for all of them.
		case scanner.Includes:
			sb.WriteString("~=")
		case scanner.DashMatch:
			sb.WriteString("|=")
		case scanner.PrefixMatch:
			sb.WriteString("^=")
		case scanner.SuffixMatch:
			sb.WriteString("$=")
		case scanner.SubstringMatch:
			sb.WriteString("*=")
		default:
			sb.WriteString(tok.Value)
		}
	}
	return strings.TrimSpace(sb.String())
}
