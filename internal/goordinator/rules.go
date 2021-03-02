package goordinator

import (
	"strings"

	"github.com/simplesurance/goordinator/internal/stringutils"
)

type Rules []*Rule

func (rr Rules) String() string {
	var result strings.Builder

	for i, r := range rr {
		result.WriteString(stringutils.IndentString(r.DetailedString(), "  "))
		if i < len(rr)-1 {
			result.WriteRune('\n')
		}
	}

	return result.String()
}
