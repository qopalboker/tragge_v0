package validation

import (
	"html"
	"net/url"
	pathpkg "path"
	"regexp"
	"strings"
	"unicode"
)

// ===========================================
// Input Sanitization
// ===========================================

// controlCharRegex matches control characters except newline, tab, carriage return.
var controlCharRegex = regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)

// multipleSpacesRegex matches multiple consecutive spaces.
var multipleSpacesRegex = regexp.MustCompile(`\s+`)

// SanitizeString removes control characters and trims whitespace.
func SanitizeString(s string) string {
	// Remove control characters
	s = controlCharRegex.ReplaceAllString(s, "")

	// Trim whitespace
	s = strings.TrimSpace(s)

	return s
}

// SanitizeStringStrict performs stricter sanitization:
// - Removes control characters
// - Collapses multiple spaces into one
// - Trims whitespace
func SanitizeStringStrict(s string) string {
	// Remove control characters
	s = controlCharRegex.ReplaceAllString(s, "")

	// Collapse multiple spaces
	s = multipleSpacesRegex.ReplaceAllString(s, " ")

	// Trim whitespace
	s = strings.TrimSpace(s)

	return s
}

// SanitizeStringForDisplay sanitizes a string and escapes HTML for safe display.
// Use this for user-supplied strings that will be rendered in HTML contexts.
func SanitizeStringForDisplay(s string) string {
	s = SanitizeString(s)
	return html.EscapeString(s)
}

// SanitizeHTML escapes HTML special characters to prevent XSS.
func SanitizeHTML(s string) string {
	return html.EscapeString(s)
}

// SanitizeForSQL removes or escapes characters that could be used in SQL injection.
// IMPORTANT: This is a defense-in-depth measure only. Always use parameterized queries
// as the primary protection against SQL injection.
func SanitizeForSQL(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Escape backslashes first (before other escapes that might introduce them)
	s = strings.ReplaceAll(s, "\\", "\\\\")

	// Escape single quotes
	s = strings.ReplaceAll(s, "'", "''")

	// Remove SQL comment markers
	s = strings.ReplaceAll(s, "--", "")
	s = strings.ReplaceAll(s, "/*", "")
	s = strings.ReplaceAll(s, "*/", "")

	// Remove semicolons (statement terminators)
	s = strings.ReplaceAll(s, ";", "")

	return s
}

// SanitizeEmail normalizes and sanitizes an email address.
func SanitizeEmail(email string) string {
	email = SanitizeString(email)
	email = strings.ToLower(email)
	return email
}

// SanitizeSymbol normalizes and sanitizes a trading symbol.
func SanitizeSymbol(symbol string) string {
	symbol = SanitizeString(symbol)
	symbol = strings.ToUpper(symbol)

	// Only keep alphanumeric characters and forward slash (for forex/crypto pairs like EUR/USD)
	var result strings.Builder
	for _, r := range symbol {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// SanitizeUUID removes whitespace and normalizes a UUID.
func SanitizeUUID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.ToLower(id)
	return id
}

// SanitizeName sanitizes a name field (person name, contest name, etc.).
func SanitizeName(name string) string {
	name = SanitizeStringStrict(name)
	// Remove leading/trailing special characters but keep internal ones
	name = strings.Trim(name, "!@#$%^&*()_+-=[]{}|;':\",./<>?")
	return name
}

// ===========================================
// Header Sanitization
// ===========================================

// SanitizeHeaderValue removes characters that could cause HTTP header injection.
func SanitizeHeaderValue(value string) string {
	// Remove newlines and carriage returns
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")

	// Remove null bytes
	value = strings.ReplaceAll(value, "\x00", "")

	return strings.TrimSpace(value)
}

// ===========================================
// URL Sanitization
// ===========================================

// SanitizeURLPath removes path traversal sequences.
func SanitizeURLPath(p string) string {
	// URL-decode first to catch encoded traversal sequences (%2e%2e)
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}

	// Remove null bytes
	p = strings.ReplaceAll(p, "\x00", "")

	// Loop removal of ".." until stable
	for {
		cleaned := strings.ReplaceAll(p, "..", "")
		if cleaned == p {
			break
		}
		p = cleaned
	}

	// Collapse multiple slashes
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}

	// Normalize with path.Clean
	p = pathpkg.Clean(p)

	return p
}

// ===========================================
// Numeric Sanitization
// ===========================================

// ClampInt64 clamps an int64 value between min and max.
func ClampInt64(value, min, max int64) int64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampFloat64 clamps a float64 value between min and max.
func ClampFloat64(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ===========================================
// JSON Sanitization
// ===========================================

// ===========================================
// Rich HTML Sanitization
// ===========================================

// Dangerous HTML patterns compiled at package init.
var (
	scriptTagRegex      = regexp.MustCompile(`(?i)<script[\s>][\s\S]*?</script>`)
	scriptOpenRegex     = regexp.MustCompile(`(?i)<script[\s>/]`)
	onEventAttrRegex    = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]*)`)
	javascriptURIRegex  = regexp.MustCompile(`(?i)javascript\s*:`)
	iframeTagRegex      = regexp.MustCompile(`(?i)</?iframe[\s>]`)
	objectTagRegex      = regexp.MustCompile(`(?i)</?object[\s>]`)
	embedTagRegex       = regexp.MustCompile(`(?i)</?embed[\s>]`)
	imgTagRegex         = regexp.MustCompile(`(?i)<img[\s>/]`)
	svgTagRegex         = regexp.MustCompile(`(?i)</?svg[\s>/]`)
	baseTagRegex        = regexp.MustCompile(`(?i)<base[\s>/]`)
	formTagRegex        = regexp.MustCompile(`(?i)</?form[\s>/]`)
	metaTagRegex        = regexp.MustCompile(`(?i)<meta[\s>/]`)
)

// SanitizeRichHTML strips known dangerous HTML patterns while preserving
// legitimate markup. This is defense-in-depth for admin account compromise.
// WARNING: regex-based, replace with bluemonday — regex HTML sanitization is
// fundamentally bypassable (nested tags, data: URIs, <style>/<link> injection).
// Used in production: admin-bff handlers_helpers.go (email template HTML content).
func SanitizeRichHTML(s string) string {
	// Decode HTML entities first to catch encoded payloads like &#60;script&#62;
	s = html.UnescapeString(s)

	s = scriptTagRegex.ReplaceAllString(s, "")
	s = scriptOpenRegex.ReplaceAllString(s, "")
	s = onEventAttrRegex.ReplaceAllString(s, "")
	s = javascriptURIRegex.ReplaceAllString(s, "")
	s = iframeTagRegex.ReplaceAllString(s, "")
	s = objectTagRegex.ReplaceAllString(s, "")
	s = embedTagRegex.ReplaceAllString(s, "")
	s = imgTagRegex.ReplaceAllString(s, "")
	s = svgTagRegex.ReplaceAllString(s, "")
	s = baseTagRegex.ReplaceAllString(s, "")
	s = formTagRegex.ReplaceAllString(s, "")
	s = metaTagRegex.ReplaceAllString(s, "")
	return s
}

// SanitizeJSONString prepares a string for safe JSON embedding.
func SanitizeJSONString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// The json package handles proper escaping, but we remove control chars
	s = controlCharRegex.ReplaceAllString(s, "")

	return s
}
