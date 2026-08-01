package audit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
)

func TestSanitizedMetadataRemovesNestedCredentials(t *testing.T) {
	const credential = "sec005-audit-fixture-never-use"
	metadata := map[string]interface{}{
		"password": credential,
		"nested": map[string]interface{}{
			"access_token": credential,
			"outcome":      "denied",
		},
	}
	sanitized := sanitizedMetadata(metadata)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	if strings.Contains(output, credential) {
		t.Fatalf("audit metadata leaked credential: %s", output)
	}
	if !strings.Contains(output, observability.RedactedValue) || !strings.Contains(output, "denied") {
		t.Fatalf("audit metadata lost safe evidence: %s", output)
	}
	if metadata["password"] != credential {
		t.Fatal("sanitization mutated caller metadata")
	}
}
