package validation

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// ParseTrustedProxyCIDRs parses an explicit immediate-proxy allowlist. Empty
// input trusts no proxy. Production never receives implicit private-network
// trust: an operator must identify every ingress hop deliberately.
func ParseTrustedProxyCIDRs(raw string) ([]*net.IPNet, error) {
	var proxies []*net.IPNet
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			proxies = append(proxies, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR")
		}
		proxies = append(proxies, network)
	}
	return proxies, nil
}

// TrustedProxiesFromEnv returns the configured trusted ingress hops. There are
// no permissive private-network defaults.
func TrustedProxiesFromEnv() ([]*net.IPNet, error) {
	return ParseTrustedProxyCIDRs(os.Getenv("TRUSTED_PROXY_CIDRS"))
}

func containsIP(networks []*net.IPNet, ip net.IP) bool {
	for _, network := range networks {
		if network != nil && ip != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(remoteAddr), "[]")
	}
	return net.ParseIP(host)
}

// ExtractClientIP uses the socket peer unless that immediate peer is in the
// explicit trusted-proxy set. Invalid configuration or headers fail safely by
// returning the socket peer.
func ExtractClientIP(r *http.Request) string {
	proxies, err := TrustedProxiesFromEnv()
	if err != nil {
		proxies = nil
	}
	return ExtractClientIPWithProxies(r, proxies)
}

// ExtractClientIPWithProxies walks X-Forwarded-For from the immediate peer
// toward the client and stops at the first untrusted hop. This prevents a
// caller from selecting an arbitrary left-most address. X-Real-IP is used only
// when X-Forwarded-For is absent and contains one valid address.
func ExtractClientIPWithProxies(r *http.Request, proxies []*net.IPNet) string {
	peer := remoteIP(r.RemoteAddr)
	if peer == nil {
		return "unknown"
	}
	peerText := peer.String()
	if !containsIP(proxies, peer) {
		return peerText
	}

	if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		var chain []net.IP
		for _, header := range forwarded {
			for _, raw := range strings.Split(header, ",") {
				ip := net.ParseIP(strings.Trim(strings.TrimSpace(raw), "[]"))
				if ip == nil {
					return peerText
				}
				chain = append(chain, ip)
			}
		}
		current := peer
		for i := len(chain) - 1; i >= 0; i-- {
			if !containsIP(proxies, current) {
				break
			}
			current = chain[i]
		}
		return current.String()
	}

	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if strings.Contains(real, ",") {
			return peerText
		}
		if ip := net.ParseIP(strings.Trim(real, "[]")); ip != nil {
			return ip.String()
		}
	}
	return peerText
}

// IsSecureRequest recognizes a direct TLS request or a trusted proxy's exact
// https forwarding signal. Untrusted callers cannot enable HSTS/cookie policy
// by spoofing X-Forwarded-Proto.
func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proxies, err := TrustedProxiesFromEnv()
	if err != nil || !containsIP(proxies, remoteIP(r.RemoteAddr)) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
