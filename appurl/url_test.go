package appurl

import "testing"

func TestPathJoinsPublicURLAndAppPath(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		appPath   string
		want      string
	}{
		{name: "origin", publicURL: "https://analytics.example.com", appPath: "/dashboard", want: "https://analytics.example.com/dashboard"},
		{name: "origin slash", publicURL: "https://analytics.example.com/", appPath: "/dashboard", want: "https://analytics.example.com/dashboard"},
		{name: "subdirectory", publicURL: "https://www.example.net/hitkeep/", appPath: "/reset-password?token=abc", want: "https://www.example.net/hitkeep/reset-password?token=abc"},
		{name: "nested subdirectory", publicURL: "https://www.example.net/tools/hitkeep", appPath: "api/status", want: "https://www.example.net/tools/hitkeep/api/status"},
		{name: "literal unicode prefix", publicURL: "https://example.test/café/", appPath: "/login?returnUrl=%2Fsettings#factor", want: "https://example.test/caf%C3%A9/login?returnUrl=%2Fsettings#factor"},
		{name: "equivalent encoded prefix", publicURL: "https://example.test/caf%c3%a9", appPath: "/api/status", want: "https://example.test/caf%C3%A9/api/status"},
		{name: "unreserved encoded prefix", publicURL: "https://example.test/%70refix", appPath: "/api/status", want: "https://example.test/prefix/api/status"},
		{name: "empty public URL", publicURL: "", appPath: "/dashboard", want: "/dashboard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Path(tt.publicURL, tt.appPath); got != tt.want {
				t.Fatalf("Path(%q, %q) = %q, want %q", tt.publicURL, tt.appPath, got, tt.want)
			}
		})
	}
}
