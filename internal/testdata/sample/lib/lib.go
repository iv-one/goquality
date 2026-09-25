package lib

import "strings"

// Add adds.
func Add(a, b int) int {
	return a + b
}

// Same compares strings case-insensitively.
func Same(a, b string) bool {
	return strings.ToLower(a) == strings.ToLower(b)
}

// Classify has a cyclomatic complexity of 5.
func Classify(n int) string {
	if n < 0 {
		return "negative"
	}
	if n == 0 {
		return "zero"
	}
	if n < 10 && n%2 == 0 {
		return "small even"
	}
	return "other"
}
