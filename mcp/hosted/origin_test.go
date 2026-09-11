package hosted

import "testing"

func TestHTTPSOriginOf(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"https origin", "https://pinner.xyz", "https://pinner.xyz"},
		{"path stripped", "https://mcp.example.com/mcp", "https://mcp.example.com"},
		{"port preserved", "https://pinner.xyz:8443/mcp", "https://pinner.xyz:8443"},
		{"http scheme", "http://localhost:8080/mcp", "http://localhost:8080"},
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"malformed", "not a url", ""},
		{"scheme only", "https://", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HTTPSOriginOf(tc.in); got != tc.want {
				t.Fatalf("HTTPSOriginOf(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
