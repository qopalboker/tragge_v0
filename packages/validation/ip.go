package validation

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// defaultTrustedProxies are private/loopback CIDRs trusted by default.
var defaultTrustedCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

var (
	trustedProxiesOnce sync.Once
	trustedProxies     []*net.IPNet
)

// loadTrustedProxies parses TRUSTED_PROXY_CIDRS (comma-separated) or uses defaults.
func loadTrustedProxies() []*net.IPNet {
	trustedProxiesOnce.Do(func() {
		cidrs := defaultTrustedCIDRs
		if env := os.Getenv("TRUSTED_PROXY_CIDRS"); env != "" {
			cidrs = strings.Split(env, ",")
		}
		for _, cidr := range cidrs {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			trustedProxies = append(trustedProxies, network)
		}
	})
	return trustedProxies
}

// ExtractClientIP extracts the real client IP from an HTTP request.
// If RemoteAddr is within a trusted proxy CIDR, it reads X-Real-IP first,
// then the first entry of X-Forwarded-For. Otherwise it returns RemoteAddr.
//
// Trusted proxies are loaded from TRUSTED_PROXY_CIDRS env var (comma-separated CIDRs),
// defaulting to 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16.
func ExtractClientIP(r *http.Request) string {
	return ExtractClientIPWithProxies(r, loadTrustedProxies())
}

// ExtractClientIPWithProxies extracts the real client IP using an explicit trusted proxy list.
func ExtractClientIPWithProxies(r *http.Request, proxies []*net.IPNet) string {
	// Parse RemoteAddr (host:port)
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil {
		return remoteHost
	}

	// Only trust proxy headers if RemoteAddr is from a trusted proxy
	isTrusted := false
	for _, cidr := range proxies {
		if cidr.Contains(remoteIP) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return remoteHost
	}

	// X-Real-IP is typically set by nginx proxy_set_header
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		trimmed := strings.TrimSpace(ip)
		if parsed := net.ParseIP(trimmed); parsed != nil {
			return trimmed
		}
	}

	// X-Forwarded-For may contain: client, proxy1, proxy2
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		trimmed := strings.TrimSpace(parts[0])
		if parsed := net.ParseIP(trimmed); parsed != nil {
			return trimmed
		}
	}

	return remoteHost
}
