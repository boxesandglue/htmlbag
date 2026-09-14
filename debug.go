package htmlbag

import (
	"fmt"
	"strings"

	scanner "github.com/speedata/css"
)

func indent(s string) string {
	ret := []string{}
	for _, line := range strings.Split(s, "\n") {
		ret = append(ret, "    "+line)
	}
	return strings.Join(ret, "\n")
}

func (b sBlock) String() string {
	ret := []string{}
	var firstline string
	if b.name != "" {
		firstline = fmt.Sprintf("@%s ", b.name)
	}
	firstline = firstline + b.componentValues.String() + " {"
	ret = append(ret, firstline)
	for _, v := range b.rules {
		ret = append(ret, "    "+v.key.String()+":"+v.value.String()+";")
	}
	for _, v := range b.childAtRules {
		ret = append(ret, indent(v.String()))
	}
	for _, v := range b.blocks {
		ret = append(ret, indent(v.String()))
	}
	ret = append(ret, "}")
	return strings.Join(ret, "\n")
}

func (t tokenstream) String() string {
	ret := []string{}
	for _, tok := range t {
		if tok.Type == scanner.Function {
			ret = append(ret, tok.Value+"(")
		} else {
			ret = append(ret, tok.Value)
		}
	}
	return strings.Join(ret, "")
}
