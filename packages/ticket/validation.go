package ticket

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
)

// MaxFileSize is the maximum allowed ticket attachment size (10MB).
const MaxFileSize = 10 * 1024 * 1024

// AllowedMimeTypes defines permitted MIME types for ticket attachments.
var AllowedMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"application/pdf": true,
}

// S3KeyRegex validates generated S3 object keys for ticket uploads.
var S3KeyRegex = regexp.MustCompile(`^tickets/[a-f0-9-]+/[a-f0-9-]+/[a-f0-9-]+\.(jpg|png|webp|pdf)$`)

// Categories are the valid ticket categories.
var Categories = map[string]bool{
	"account": true, "payment": true, "contest": true,
	"technical": true, "kyc": true, "other": true,
}

// Statuses are the valid ticket statuses.
var Statuses = map[string]bool{
	"open": true, "answered": true, "user_replied": true,
	"closed": true, "resolved": true,
}

// Priorities are the valid ticket priorities.
var Priorities = map[string]bool{
	"low": true, "medium": true, "high": true, "urgent": true,
}

// ValidateFile validates a ticket file upload and returns the detected MIME type.
func ValidateFile(header *multipart.FileHeader) (string, error) {
	if header.Size > MaxFileSize {
		sizeMB := float64(header.Size) / (1024 * 1024)
		return "", fmt.Errorf("file size (%.1fMB) exceeds the 10MB limit", sizeMB)
	}

	contentType := header.Header.Get("Content-Type")
	if !AllowedMimeTypes[contentType] {
		return "", fmt.Errorf("invalid file type '%s'. Allowed: JPEG, PNG, WebP, or PDF", contentType)
	}

	file, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read file header: %w", err)
	}

	detectedType := http.DetectContentType(buf[:n])
	if contentType == "application/pdf" {
		if n >= 4 && string(buf[:4]) == "%PDF" {
			detectedType = "application/pdf"
		}
	}

	if !AllowedMimeTypes[detectedType] {
		return "", fmt.Errorf("file content does not match allowed types (detected: %s)", detectedType)
	}

	// PDF security: scan first 4KB for dangerous entries
	if detectedType == "application/pdf" {
		file.Seek(0, io.SeekStart)
		scanBuf := make([]byte, 4096)
		scanN, _ := file.Read(scanBuf)
		if scanN > 0 {
			pdfContent := strings.ToUpper(string(scanBuf[:scanN]))
			dangerousEntries := []string{"/JS", "/JAVASCRIPT", "/LAUNCH", "/OPENACTION", "/AA"}
			for _, entry := range dangerousEntries {
				if strings.Contains(pdfContent, entry) {
					return "", fmt.Errorf("PDF contains potentially unsafe content")
				}
			}
		}
		file.Seek(0, io.SeekStart)
	}

	return detectedType, nil
}

// ValidateS3Key ensures the generated S3 key is safe.
func ValidateS3Key(key string) error {
	if strings.Contains(key, "..") || strings.Contains(key, "\\") {
		return fmt.Errorf("invalid S3 key: path traversal detected")
	}
	if !S3KeyRegex.MatchString(key) {
		return fmt.Errorf("invalid S3 key format: %s", key)
	}
	return nil
}

// SanitizeFileName removes characters that could cause HTTP header injection
// or path traversal in Content-Disposition headers.
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" {
		name = "attachment"
	}
	return name
}

// MimeToExt maps MIME types to file extensions (without dot).
func MimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "application/pdf":
		return "pdf"
	default:
		return "bin"
	}
}
