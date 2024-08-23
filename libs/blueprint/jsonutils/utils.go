package jsonutils

import "strings"

// EscapeJSONString escapes a string to be used in a marshalled JSON string.
func EscapeJSONString(str string) string {
	escapedNewLine := strings.ReplaceAll(str, "\n", "\\n")
	escapedSingleQuote := strings.ReplaceAll(escapedNewLine, "'", "\\'")
	escapedDoubleQuote := strings.ReplaceAll(escapedSingleQuote, "\"", "\\\"")
	escapedAmpersand := strings.ReplaceAll(escapedDoubleQuote, "&", "\\&")
	escapedReturn := strings.ReplaceAll(escapedAmpersand, "\r", "\\r")
	escapedTab := strings.ReplaceAll(escapedReturn, "\t", "\\t")
	escapedBackspace := strings.ReplaceAll(escapedTab, "\b", "\\b")
	return strings.ReplaceAll(escapedBackspace, "\f", "\\f")
}