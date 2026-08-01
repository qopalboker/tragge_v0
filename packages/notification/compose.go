package notification

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// FontConfig represents font settings for a single language.
type FontConfig struct {
	Family string `json:"family"`
	URL    string `json:"url"`
}

// TemplateComposition holds the components needed to compose a full email HTML document.
type TemplateComposition struct {
	HTMLBody   string
	CSSContent string
	FontConfig map[string]FontConfig // "en" -> {...}, "fa" -> {...}
}

// isAllowedFontURL checks that a font URL uses a trusted Google Fonts origin.
func isAllowedFontURL(u string) bool {
	return strings.HasPrefix(u, "https://fonts.googleapis.com/") ||
		strings.HasPrefix(u, "https://fonts.gstatic.com/")
}

// unsafeCSSPattern matches CSS constructs that can be used for data exfiltration
// or code execution: url(), expression(), @import, and behavior.
var unsafeCSSPattern = regexp.MustCompile(`(?i)(url\s*\(|expression\s*\(|@import\b|behavior\s*:)`)

// sanitizeFontFamily removes characters that could break out of a CSS font-family
// declaration. Only allows alphanumeric, spaces, hyphens, and commas.
func sanitizeFontFamily(family string) string {
	var sb strings.Builder
	for _, r := range family {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == ' ' || r == '-' || r == ',' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// sanitizeCSSContent removes dangerous CSS constructs while preserving safe styling.
func sanitizeCSSContent(css string) string {
	return unsafeCSSPattern.ReplaceAllString(css, "/* blocked */")
}

// ComposeEmailHTML builds a complete HTML document from body, CSS, and font configuration.
// This is used by both the admin-bff preview handler and the email renderer when
// an active template version is found.
func ComposeEmailHTML(comp TemplateComposition) string {
	var fontLinks strings.Builder
	var fontStyles strings.Builder

	if en, ok := comp.FontConfig["en"]; ok {
		if en.URL != "" && isAllowedFontURL(en.URL) {
			fontLinks.WriteString(fmt.Sprintf(`  <link href="%s" rel="stylesheet">`+"\n", html.EscapeString(en.URL)))
		}
		if en.Family != "" {
			family := sanitizeFontFamily(en.Family)
			fontStyles.WriteString(fmt.Sprintf("    :lang(en) { font-family: '%s', sans-serif; }\n", family))
		}
	}
	if fa, ok := comp.FontConfig["fa"]; ok {
		if fa.URL != "" && isAllowedFontURL(fa.URL) {
			fontLinks.WriteString(fmt.Sprintf(`  <link href="%s" rel="stylesheet">`+"\n", html.EscapeString(fa.URL)))
		}
		if fa.Family != "" {
			family := sanitizeFontFamily(fa.Family)
			fontStyles.WriteString(fmt.Sprintf("    :lang(fa) { font-family: '%s', sans-serif; }\n", family))
		}
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	if fontLinks.Len() > 0 {
		sb.WriteString(fontLinks.String())
	}
	sb.WriteString("  <style>\n")
	if fontStyles.Len() > 0 {
		sb.WriteString(fontStyles.String())
	}
	if comp.CSSContent != "" {
		sanitizedCSS := sanitizeCSSContent(comp.CSSContent)
		sb.WriteString("    " + strings.ReplaceAll(sanitizedCSS, "\n", "\n    ") + "\n")
	}
	sb.WriteString("  </style>\n")
	sb.WriteString("</head>\n<body>")
	sb.WriteString(comp.HTMLBody)
	sb.WriteString("</body>\n</html>")

	return sb.String()
}

// ComposeEmailHTMLFromJSON builds a complete HTML document from body, CSS, and raw JSON font config.
// This is a convenience wrapper that parses the JSON font config before calling ComposeEmailHTML.
func ComposeEmailHTMLFromJSON(htmlBody, cssContent string, fontConfigJSON json.RawMessage) string {
	var fonts map[string]FontConfig
	_ = json.Unmarshal(fontConfigJSON, &fonts)

	return ComposeEmailHTML(TemplateComposition{
		HTMLBody:   htmlBody,
		CSSContent: cssContent,
		FontConfig: fonts,
	})
}
