package server

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateScanURL checks that rawURL is safe to use as a scan target.
// It enforces HTTPS and blocks requests to localhost and private IP ranges
// to prevent SSRF attacks.
func ValidateScanURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("URL must not be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "https" {
		return fmt.Errorf("URL must use HTTPS, got %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a non-empty host")
	}

	if isPrivateHost(host) {
		return fmt.Errorf("URL host %q is not allowed (localhost or private network)", host)
	}

	return nil
}

// isPrivateHost reports whether the given hostname is localhost or resolves
// to a private/loopback IP range that should not be reachable from the server.
func isPrivateHost(host string) bool {
	// Named localhost variants.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Not a raw IP — pass through (DNS-based SSRF is out of scope here).
		return false
	}

	// Loopback: 127.0.0.0/8
	if ip.IsLoopback() {
		return true
	}

	// Private ranges per RFC 1918 and RFC 4193.
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",  // Shared address space (RFC 6598)
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
		"::1/128",        // IPv6 loopback
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}
