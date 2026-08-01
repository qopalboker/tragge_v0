package server

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func TestValidateIranianNationalCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		// Valid codes
		{"valid code 0012345687", "0012345687", true},
		{"valid code 0499370899", "0499370899", true},

		// Invalid: all same digits
		{"all same digits 0", "0000000000", false},
		{"all same digits 1", "1111111111", false},
		{"all same digits 9", "9999999999", false},

		// Invalid: wrong length
		{"too short", "123456789", false},
		{"too long", "12345678901", false},

		// Invalid: non-digits
		{"has letters", "123456789a", false},
		{"empty", "", false},

		// Invalid: bad checksum
		{"bad checksum", "0012345688", false},
		{"bad checksum 2", "1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateIranianNationalCode(tt.code)
			if got != tt.want {
				t.Errorf("validateIranianNationalCode(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestPersianNameRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Valid Persian names
		{"simple persian", "حسام", true},
		{"two word persian", "محمد رضا", true},
		{"with ZWNJ", "علی\u200Cرضا", true},
		{"long name", "سید محمدرضا حسینی", true},

		// Invalid
		{"digits", "123", false},
		{"latin", "ali", false},
		{"mixed persian digits", "حسام123", false},
		{"empty", "", false},
		{"single char", "ح", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := persianNameRegex.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("persianNameRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateKYCS3Key(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid front key", "kyc/a1b2c3d4-e5f6-7890-abcd-ef1234567890/front_a1b2c3d4-e5f6-7890-abcd-ef1234567890.jpg", false},
		{"valid selfie_with_doc key", "kyc/a1b2c3d4-e5f6-7890-abcd-ef1234567890/selfie_with_doc_a1b2c3d4-e5f6-7890-abcd-ef1234567890.png", false},
		{"valid back key", "kyc/a1b2c3d4-e5f6-7890-abcd-ef1234567890/back_a1b2c3d4-e5f6-7890-abcd-ef1234567890.webp", false},
		{"path traversal", "kyc/../etc/passwd", true},
		{"backslash traversal", "kyc/..\\etc\\passwd", true},
		{"wrong prefix", "uploads/front_abc.jpg", true},
		{"random string", "not-a-valid-key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKYCS3Key(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKYCS3Key(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateKYCFormFileFields(t *testing.T) {
	// Test nil form
	err := validateKYCFormFileFields(nil)
	if err != nil {
		t.Errorf("validateKYCFormFileFields(nil) = %v, want nil", err)
	}
}

func createTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func createMockFileHeader(filename, contentType string, data []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", contentType)
	part, _ := writer.CreatePart(h)
	part.Write(data)
	writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(32 << 20)
	return form.File["file"][0]
}

func TestValidateKYCFileDimensions(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		format  string // "jpeg" or "png"
		wantErr bool
		errMsg  string
	}{
		{"valid 800x600 jpeg", 800, 600, "jpeg", false, ""},
		{"too small 100x100 jpeg", 100, 100, "jpeg", true, "too small"},
		{"minimum 200x200 png", 200, 200, "png", false, ""},
		{"one dimension too small", 200, 50, "jpeg", true, "too small"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imgBytes []byte
			var ext string
			if tt.format == "jpeg" {
				imgBytes = createTestJPEG(tt.width, tt.height)
				ext = "jpg"
			} else {
				imgBytes = createTestPNG(tt.width, tt.height)
				ext = "png"
			}

			header := createMockFileHeader("test."+ext, "image/"+tt.format, imgBytes)

			result, err := validateKYCFile(header)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result.DetectedType == "" {
					t.Error("expected non-empty DetectedType")
				}
			}
		})
	}
}
