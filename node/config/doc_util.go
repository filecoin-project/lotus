package config

import (
	"fmt"
	"strings"
)

func findDoc(root interface{}, section, name string) *DocField {
	rt := fmt.Sprintf("%T", root)[len("*config."):]

	doc := findDocSect(rt, section, name)
	if doc != nil {
		return doc
	}

	return findDocSect("Common", section, name)
}

func findDocSect(root, section, name string) *DocField {
	path := strings.Split(section, ".")

	docSection := Doc[root]
	for _, e := range path {
		if docSection == nil {
			return nil
		}

		for _, field := range docSection {
			if field.Name == e {
				docSection = Doc[field.Type]
				break
			}

		}
	}

	for _, df := range docSection {
		if df.Name == name {
			return &df
		}
	}

	return nil
}
