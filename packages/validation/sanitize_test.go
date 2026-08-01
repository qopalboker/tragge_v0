package validation

import (
	"testing"
)

func TestSanitizeSymbol(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"simple stock", "AAPL", "AAPL"},
		{"lowercase stock", "aapl", "AAPL"},
		{"forex pair preserved", "EUR/USD", "EUR/USD"},
		{"lowercase forex pair", "eur/usd", "EUR/USD"},
		{"crypto pair preserved", "BTC/USD", "BTC/USD"},
		{"metal pair preserved", "XAU/USD", "XAU/USD"},
		{"strips invalid chars", "AA-PL", "AAPL"},
		{"strips spaces", " AAPL ", "AAPL"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSymbol(tt.input)
			if got != tt.expect {
				t.Errorf("SanitizeSymbol(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestSanitizeStringForDisplay(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"plain text", "hello world", "hello world"},
		{"html tags escaped", "<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"ampersand escaped", "foo & bar", "foo &amp; bar"},
		{"control chars removed and html escaped", "\x00<b>test</b>", "&lt;b&gt;test&lt;/b&gt;"},
		{"whitespace trimmed", "  hello  ", "hello"},
		{"empty string", "", ""},
		{"quotes escaped", `"quoted"`, "&#34;quoted&#34;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeStringForDisplay(tt.input)
			if got != tt.expect {
				t.Errorf("SanitizeStringForDisplay(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestSanitizeForSQL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"normal string", "hello world", "hello world"},
		{"single quotes escaped", "it's a test", "it''s a test"},
		{"null bytes removed", "hello\x00world", "helloworld"},
		{"backslash escaped", "hello\\world", "hello\\\\world"},
		{"comment markers removed", "hello -- drop table", "hello  drop table"},
		{"block comment removed", "hello /* comment */ world", "hello  comment  world"},
		{"semicolons removed", "hello; DROP TABLE users;", "hello DROP TABLE users"},
		{"combined attack", "'; DROP TABLE users; --", "'' DROP TABLE users "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForSQL(tt.input)
			if got != tt.expect {
				t.Errorf("SanitizeForSQL(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
