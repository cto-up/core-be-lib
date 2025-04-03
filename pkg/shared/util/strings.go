package util

import (
	"os"
	"regexp"
	"strings"
)

func Interpolate(template string, variables map[string]string) string {
	f := func(ph string) string {
		return variables[ph]
	}
	return os.Expand(template, f)
}

func Spaces(size int) string {
	if size <= 0 {
		return ""
	}
	return strings.Repeat(" ", size)
}
func SplitCamelCase(input string) string {
	// Use a regular expression to find capital letters and insert a space before them
	re := regexp.MustCompile(`([a-z])([A-Z])`)
	result := re.ReplaceAllString(input, `$1 $2`)
	return strings.ToLower(result)
}
