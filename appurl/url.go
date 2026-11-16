package appurl

import (
	"net/url"
	"strings"
)

// Path joins the configured public URL with an app-owned absolute path.
func Path(publicURL, appPath string) string {
	reference, err := url.Parse("/" + strings.TrimLeft(strings.TrimSpace(appPath), "/"))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(publicURL) == "" {
		return reference.String()
	}
	base, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil {
		return ""
	}
	decoded, escaped, ok := Prefix(base.EscapedPath())
	if !ok {
		return ""
	}
	base.Path = decoded + reference.Path
	base.RawPath = escaped + reference.EscapedPath()
	base.RawQuery = reference.RawQuery
	base.ForceQuery = reference.ForceQuery
	base.Fragment = reference.Fragment
	base.RawFragment = reference.RawFragment
	return base.String()
}
