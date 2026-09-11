package hosted

import (
	"net/url"
	"strings"
)

// HTTPSOriginOf returns the exact origin (scheme://host[:port], no path) of a
// public base URL, or empty when baseURL is empty or malformed. It is the
// reusable helper a hosted assembly uses to attribute app views and byte-route
// coordinator connect-origins to THIS deployment's own origin from the public
// base URL — so a hosted deployment without a public origin never advertises a
// foreign domain.
func HTTPSOriginOf(baseURL string) string {
	if strings.TrimSpace(baseURL) == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
