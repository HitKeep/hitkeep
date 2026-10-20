package oauth

import (
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
)

type issuerURLs struct {
	Issuer                    string
	Prefix                    string
	APIResource               string
	MCPResource               string
	AuthorizationMetadataPath string
	APIResourceMetadataPath   string
	MCPResourceMetadataPath   string
}

func parseIssuer(raw, mcpPath string) (issuerURLs, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.ContainsAny(raw, "#\\\r\n\t") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return issuerURLs{}, errors.New("OAuth issuer must be an absolute canonical HTTPS URL without credentials, query or fragment")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return issuerURLs{}, errors.New("invalid OAuth issuer port")
		}
	}
	prefix := strings.TrimSuffix(u.Path, "/")
	if u.EscapedPath() != u.Path || strings.Contains(u.Path, "//") || u.RawPath != "" || (prefix != "" && (!strings.HasPrefix(prefix, "/") || path.Clean(prefix) != prefix)) {
		return issuerURLs{}, errors.New("ambiguous OAuth issuer path")
	}
	if mcpPath == "" || !strings.HasPrefix(mcpPath, "/") || strings.HasPrefix(mcpPath, "//") || path.Clean(mcpPath) != mcpPath || strings.ContainsAny(mcpPath, "?%#\\\r\n\t") || strings.ContainsFunc(mcpPath, unicode.IsSpace) || mcpPath == "/" || mcpPath == "/api" || strings.HasPrefix(mcpPath, "/api/") || mcpPath == "/oauth" || strings.HasPrefix(mcpPath, "/oauth/") {
		return issuerURLs{}, errors.New("invalid or overlapping OAuth MCP mount")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = prefix
	issuer := u.String()
	return issuerURLs{
		Issuer: issuer, Prefix: prefix, APIResource: issuer + "/api", MCPResource: issuer + mcpPath,
		AuthorizationMetadataPath: "/.well-known/oauth-authorization-server" + prefix,
		APIResourceMetadataPath:   "/.well-known/oauth-protected-resource" + prefix + "/api",
		MCPResourceMetadataPath:   "/.well-known/oauth-protected-resource" + prefix + mcpPath,
	}, nil
}
