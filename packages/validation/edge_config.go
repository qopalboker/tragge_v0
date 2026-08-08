package validation

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// EdgeEnvironment is the centralized startup contract for SEC-006 controls.
type EdgeEnvironment struct {
	Production        bool
	TrustedProxyCIDRs string
	UserOrigins       []string
	AdminOrigins      []string
	TradeOrigins      []string
	PaymentOrigins    []string
	DefaultBodyBytes  int64
	UploadBodyBytes   int64
	MaxHeaderBytes    int
}

func parsePositiveBounded(raw string, fallback, minimum, maximum int64) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("edge size setting is outside approved bounds")
	}
	return value, nil
}

func exactOrigins(raw string) ([]string, error) {
	origins := parseCommaSeparated(raw)
	for _, origin := range origins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Hostname() == "" || parsed.Path != "" || parsed.RawQuery != "" ||
			parsed.Fragment != "" || parsed.User != nil || parsed.Opaque != "" ||
			strings.Contains(origin, "*") {
			return nil, fmt.Errorf("edge origin must be an exact scheme and host")
		}
	}
	return origins, nil
}

// LoadAndValidateEdgeEnvironment validates production controls without logging
// configuration values. Local defaults are explicit and never accepted in
// production.
func LoadAndValidateEdgeEnvironment(getenv func(string) string) (EdgeEnvironment, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("ENVIRONMENT")))
	production := env == "" || env == environmentProduction || env == "staging"
	defaultBytes, err := parsePositiveBounded(getenv("EDGE_MAX_BODY_BYTES"), 1024*1024, 1024, 8*1024*1024)
	if err != nil {
		return EdgeEnvironment{}, err
	}
	uploadBytes, err := parsePositiveBounded(getenv("EDGE_MAX_UPLOAD_BYTES"), 35*1024*1024, defaultBytes, 64*1024*1024)
	if err != nil {
		return EdgeEnvironment{}, err
	}
	headerBytes, err := parsePositiveBounded(getenv("EDGE_MAX_HEADER_BYTES"), 16*1024, 8*1024, 64*1024)
	if err != nil {
		return EdgeEnvironment{}, err
	}
	proxyRaw := strings.TrimSpace(getenv("TRUSTED_PROXY_CIDRS"))
	if _, err := ParseTrustedProxyCIDRs(proxyRaw); err != nil {
		return EdgeEnvironment{}, err
	}
	readOrigins := func(context, legacy string) ([]string, error) {
		raw := strings.TrimSpace(getenv(strings.ToUpper(context) + "_CORS_ALLOWED_ORIGINS"))
		if raw == "" {
			raw = strings.TrimSpace(getenv(legacy))
		}
		if raw == "" && !production {
			switch context {
			case edgeContextAdmin:
				raw = "http://127.0.0.1:8081"
			default:
				raw = "http://127.0.0.1:8080"
			}
		}
		origins, parseErr := exactOrigins(raw)
		if production && len(origins) == 0 {
			return nil, fmt.Errorf("%s production origin is required", context)
		}
		return origins, parseErr
	}
	user, err := readOrigins(edgeContextUser, "USER_FRONTEND_ORIGIN")
	if err != nil {
		return EdgeEnvironment{}, err
	}
	admin, err := readOrigins(edgeContextAdmin, "ADMIN_FRONTEND_ORIGIN")
	if err != nil {
		return EdgeEnvironment{}, err
	}
	if production {
		userSet := make(map[string]struct{}, len(user))
		for _, origin := range user {
			userSet[origin] = struct{}{}
		}
		for _, origin := range admin {
			if _, collision := userSet[origin]; collision {
				return EdgeEnvironment{}, fmt.Errorf("production User and Admin origins must be distinct")
			}
		}
	}
	trade, err := readOrigins(edgeContextTrade, "USER_FRONTEND_ORIGIN")
	if err != nil {
		return EdgeEnvironment{}, err
	}
	payment, err := readOrigins(edgeContextPayment, "USER_FRONTEND_ORIGIN")
	if err != nil {
		return EdgeEnvironment{}, err
	}
	if production && proxyRaw == "" {
		return EdgeEnvironment{}, fmt.Errorf("production TRUSTED_PROXY_CIDRS is required")
	}
	return EdgeEnvironment{
		Production: production, TrustedProxyCIDRs: proxyRaw,
		UserOrigins: user, AdminOrigins: admin, TradeOrigins: trade, PaymentOrigins: payment,
		DefaultBodyBytes: defaultBytes, UploadBodyBytes: uploadBytes, MaxHeaderBytes: int(headerBytes),
	}, nil
}
