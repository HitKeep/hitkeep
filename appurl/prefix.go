package appurl

import (
	"net/url"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

var rawPrefixCharacters = strings.NewReplacer("%21", "!", "%27", "'", "%28", "(", "%29", ")", "%2A", "*", ";", "%3B")

// Prefix validates one escaped deployment prefix and returns its decoded routing
// path and canonical URL/cookie path, without changing Unicode or path case.
func Prefix(encoded string) (decoded, escaped string, ok bool) {
	for segment := range strings.SplitSeq(encoded, "/") {
		part, err := url.PathUnescape(segment)
		if err != nil || strings.ContainsAny(part, "/\\") || strings.ContainsFunc(part, unicode.IsControl) {
			return "", "", false
		}
		for i := range part {
			if part[i] == '%' && i+3 <= len(part) {
				if _, err := url.PathUnescape(part[i : i+3]); err == nil {
					return "", "", false
				}
			}
		}
	}
	decoded, err := url.PathUnescape(encoded)
	if err != nil || !utf8.ValidString(decoded) || strings.Contains(decoded, "//") {
		return "", "", false
	}
	decoded = strings.TrimSuffix(decoded, "/")
	if decoded != "" && (!strings.HasPrefix(decoded, "/") || path.Clean(decoded) != decoded) {
		return "", "", false
	}
	value := &url.URL{Path: decoded}
	// Semicolons delimit Set-Cookie attributes, so URL and cookie paths encode them.
	escaped = rawPrefixCharacters.Replace(value.EscapedPath())
	return decoded, escaped, true
}
