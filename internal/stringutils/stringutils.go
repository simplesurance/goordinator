package stringutils

import "strings"

// IndentString prefixes each line of the string with indent.
func IndentString(str, indent string) string {
	spl := strings.SplitAfter(str, "\n")
	return strings.Join(append([]string{""}, spl...), indent)
}
