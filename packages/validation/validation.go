// Package validation provides input validation utilities for the Tragge platform.
// It includes validators for common data types and a consistent error response format.
package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represents a standardized API error response.
type ErrorResponse struct {
	Error   string            `json:"error"`
	Code    string            `json:"code,omitempty"`
	Message string            `json:"message,omitempty"`
	Details []ValidationError `json:"details,omitempty"`
}

// Validator provides validation methods for common input types.
type Validator struct {
	errors []ValidationError
}

// New creates a new Validator instance.
func New() *Validator {
	return &Validator{
		errors: make([]ValidationError, 0),
	}
}

// HasErrors returns true if any validation errors have been recorded.
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all recorded validation errors.
func (v *Validator) Errors() []ValidationError {
	return v.errors
}

// AddError adds a validation error.
func (v *Validator) AddError(field, code, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Code:    code,
		Message: message,
	})
}

// Reset clears all validation errors.
func (v *Validator) Reset() {
	v.errors = v.errors[:0]
}

// ===========================================
// Email Validation
// ===========================================

// emailRegex is a more comprehensive email validation pattern.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail validates an email address.
// Returns the normalized (lowercase, trimmed) email and whether it's valid.
func ValidateEmail(email string) (string, bool) {
	email = strings.TrimSpace(strings.ToLower(email))

	if email == "" {
		return "", false
	}

	// Length check
	if len(email) > 254 {
		return email, false
	}

	// Basic regex check
	if !emailRegex.MatchString(email) {
		return email, false
	}

	// RFC 5322 compliant parsing
	_, err := mail.ParseAddress(email)
	if err != nil {
		return email, false
	}

	return email, true
}

// Email validates an email field and adds an error if invalid.
func (v *Validator) Email(field, value string) string {
	normalized, valid := ValidateEmail(value)
	if !valid {
		v.AddError(field, "invalid_email", "Invalid email address format")
	}
	return normalized
}

// ===========================================
// UUID Validation
// ===========================================

// ValidateUUID validates a UUID string.
func ValidateUUID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	// Parse and validate UUID format
	parsed, err := uuid.Parse(value)
	if err != nil {
		return value, false
	}

	// Return lowercase canonical form
	return parsed.String(), true
}

// UUID validates a UUID field and adds an error if invalid.
func (v *Validator) UUID(field, value string) string {
	normalized, valid := ValidateUUID(value)
	if !valid {
		v.AddError(field, "invalid_uuid", fmt.Sprintf("Invalid UUID format for %s", field))
	}
	return normalized
}

// ===========================================
// Price Validation
// ===========================================

// PriceConstraints defines constraints for price validation.
type PriceConstraints struct {
	Min           float64
	Max           float64
	MaxDecimals   int
	AllowZero     bool
	AllowNegative bool
}

// DefaultPriceConstraints returns sensible defaults for price validation.
func DefaultPriceConstraints() PriceConstraints {
	return PriceConstraints{
		Min:           0.0001,
		Max:           1_000_000_000, // 1 billion
		MaxDecimals:   8,
		AllowZero:     false,
		AllowNegative: false,
	}
}

// ValidatePrice validates a price value against constraints.
func ValidatePrice(price float64, constraints PriceConstraints) (float64, bool, string) {
	// Check for NaN
	if math.IsNaN(price) {
		return price, false, "price cannot be NaN"
	}

	// Check for Inf
	if math.IsInf(price, 0) {
		return price, false, "price cannot be infinite"
	}

	// Check negative
	if !constraints.AllowNegative && price < 0 {
		return price, false, "price cannot be negative"
	}

	// Check zero
	if !constraints.AllowZero && price == 0 {
		return price, false, "price cannot be zero"
	}

	// Check minimum (only if not zero and zero is allowed)
	if price != 0 && price < constraints.Min {
		return price, false, fmt.Sprintf("price must be at least %.4f", constraints.Min)
	}

	// Check maximum
	if price > constraints.Max {
		return price, false, fmt.Sprintf("price cannot exceed %.2f", constraints.Max)
	}

	return price, true, ""
}

// Price validates a price field and adds an error if invalid.
func (v *Validator) Price(field string, value float64, constraints PriceConstraints) float64 {
	_, valid, msg := ValidatePrice(value, constraints)
	if !valid {
		v.AddError(field, "invalid_price", msg)
	}
	return value
}

// PricePtr validates an optional price field.
func (v *Validator) PricePtr(field string, value *float64, constraints PriceConstraints) *float64 {
	if value == nil {
		return nil
	}
	v.Price(field, *value, constraints)
	return value
}

// ===========================================
// Quantity Validation
// ===========================================

// QuantityConstraints defines constraints for quantity validation.
type QuantityConstraints struct {
	Min       int64
	Max       int64
	AllowZero bool
}

// DefaultQuantityConstraints returns sensible defaults for quantity validation.
func DefaultQuantityConstraints() QuantityConstraints {
	return QuantityConstraints{
		Min:       1,
		Max:       1_000_000_000, // 1 billion shares
		AllowZero: false,
	}
}

// ValidateQuantity validates a quantity value against constraints.
func ValidateQuantity(qty int64, constraints QuantityConstraints) (int64, bool, string) {
	// Check zero
	if !constraints.AllowZero && qty == 0 {
		return qty, false, "quantity cannot be zero"
	}

	// Check negative
	if qty < 0 {
		return qty, false, "quantity cannot be negative"
	}

	// Check minimum (only if not zero and zero is allowed)
	if qty != 0 && qty < constraints.Min {
		return qty, false, fmt.Sprintf("quantity must be at least %d", constraints.Min)
	}

	// Check maximum
	if qty > constraints.Max {
		return qty, false, fmt.Sprintf("quantity cannot exceed %d", constraints.Max)
	}

	return qty, true, ""
}

// Quantity validates a quantity field and adds an error if invalid.
func (v *Validator) Quantity(field string, value int64, constraints QuantityConstraints) int64 {
	_, valid, msg := ValidateQuantity(value, constraints)
	if !valid {
		v.AddError(field, "invalid_quantity", msg)
	}
	return value
}

// QuantityInt validates an int quantity field.
func (v *Validator) QuantityInt(field string, value int, constraints QuantityConstraints) int {
	v.Quantity(field, int64(value), constraints)
	return value
}

// ===========================================
// String Validation
// ===========================================

// StringConstraints defines constraints for string validation.
type StringConstraints struct {
	MinLength   int
	MaxLength   int
	Pattern     *regexp.Regexp
	PatternDesc string
	Required    bool
	TrimSpace   bool
}

// ValidateString validates a string value against constraints.
func ValidateString(value string, constraints StringConstraints) (string, bool, string) {
	if constraints.TrimSpace {
		value = strings.TrimSpace(value)
	}

	// Check required
	if constraints.Required && value == "" {
		return value, false, "this field is required"
	}

	// If empty and not required, skip other validations
	if value == "" {
		return value, true, ""
	}

	// Check minimum length
	if utf8.RuneCountInString(value) < constraints.MinLength {
		return value, false, fmt.Sprintf("must be at least %d characters", constraints.MinLength)
	}

	// Check maximum length
	if constraints.MaxLength > 0 && utf8.RuneCountInString(value) > constraints.MaxLength {
		return value, false, fmt.Sprintf("must not exceed %d characters", constraints.MaxLength)
	}

	// Check pattern
	if constraints.Pattern != nil && !constraints.Pattern.MatchString(value) {
		msg := "invalid format"
		if constraints.PatternDesc != "" {
			msg = constraints.PatternDesc
		}
		return value, false, msg
	}

	return value, true, ""
}

// String validates a string field and adds an error if invalid.
func (v *Validator) String(field, value string, constraints StringConstraints) string {
	normalized, valid, msg := ValidateString(value, constraints)
	if !valid {
		v.AddError(field, "invalid_string", msg)
	}
	return normalized
}

// Required validates that a string field is not empty.
func (v *Validator) Required(field, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		v.AddError(field, "required", fmt.Sprintf("%s is required", field))
	}
	return value
}

// MaxLength validates that a string field does not exceed the given length.
func (v *Validator) MaxLength(field, value string, maxLen int) {
	if utf8.RuneCountInString(value) > maxLen {
		v.AddError(field, "max_length", fmt.Sprintf("%s must not exceed %d characters", field, maxLen))
	}
}

// In validates that a string value is one of the allowed values.
func (v *Validator) In(field, value string, allowed []string) {
	for _, a := range allowed {
		if a == value {
			return
		}
	}
	v.AddError(field, "invalid_option", fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", ")))
}

// Valid returns true if there are no validation errors.
func (v *Validator) Valid() bool {
	return !v.HasErrors()
}

// ===========================================
// Symbol Validation
// ===========================================

// symbolRegex matches valid trading symbols (1-10 uppercase letters, optionally as a pair e.g. EUR/USD).
var symbolRegex = regexp.MustCompile(`^[A-Z]{1,10}(/[A-Z]{1,10})?$`)

// ValidateSymbol validates a trading symbol.
func ValidateSymbol(symbol string) (string, bool) {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	if symbol == "" {
		return "", false
	}

	if !symbolRegex.MatchString(symbol) {
		return symbol, false
	}

	return symbol, true
}

// Symbol validates a trading symbol field.
func (v *Validator) Symbol(field, value string) string {
	normalized, valid := ValidateSymbol(value)
	if !valid {
		v.AddError(field, "invalid_symbol", "Symbol must be 1-10 uppercase letters, optionally as a pair (e.g., AAPL or EUR/USD)")
	}
	return normalized
}

// ===========================================
// Password Validation
// ===========================================

// PasswordConstraints defines constraints for password validation.
type PasswordConstraints struct {
	MinLength        int
	MaxLength        int
	RequireUppercase bool
	RequireLowercase bool
	RequireDigit     bool
	RequireSpecial   bool
}

// DefaultPasswordConstraints returns strong defaults for a financial trading platform.
// Requires minimum 10 characters with uppercase, lowercase, digit, and special character.
func DefaultPasswordConstraints() PasswordConstraints {
	return PasswordConstraints{
		MinLength:        10,
		MaxLength:        128,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   true,
	}
}

// LenientPasswordConstraints returns relaxed password requirements for cases
// where the stricter defaults are not appropriate.
func LenientPasswordConstraints() PasswordConstraints {
	return PasswordConstraints{
		MinLength:        8,
		MaxLength:        128,
		RequireUppercase: true,
		RequireLowercase: false,
		RequireDigit:     true,
		RequireSpecial:   false,
	}
}

// StrictPasswordConstraints returns the strictest password requirements.
func StrictPasswordConstraints() PasswordConstraints {
	return PasswordConstraints{
		MinLength:        12,
		MaxLength:        128,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   true,
	}
}

// ValidatePassword validates a password against constraints.
func ValidatePassword(password string, constraints PasswordConstraints) (bool, string) {
	if utf8.RuneCountInString(password) < constraints.MinLength {
		return false, fmt.Sprintf("password must be at least %d characters", constraints.MinLength)
	}

	if constraints.MaxLength > 0 && utf8.RuneCountInString(password) > constraints.MaxLength {
		return false, fmt.Sprintf("password must not exceed %d characters", constraints.MaxLength)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if constraints.RequireUppercase && !hasUpper {
		return false, "password must contain at least one uppercase letter"
	}
	if constraints.RequireLowercase && !hasLower {
		return false, "password must contain at least one lowercase letter"
	}
	if constraints.RequireDigit && !hasDigit {
		return false, "password must contain at least one digit"
	}
	if constraints.RequireSpecial && !hasSpecial {
		return false, "password must contain at least one special character"
	}

	return true, ""
}

// Password validates a password field.
func (v *Validator) Password(field, value string, constraints PasswordConstraints) {
	valid, msg := ValidatePassword(value, constraints)
	if !valid {
		v.AddError(field, "invalid_password", msg)
	}
}

// ===========================================
// Phone Validation
// ===========================================

// iranPhoneRegex matches Iranian mobile numbers in various formats.
var iranPhoneRegex = regexp.MustCompile(`^(?:\+98|0098|98|0)?(9\d{9})$`)

// ValidateIranPhone validates and normalizes an Iranian mobile phone number.
// Returns the phone in +98 format (e.g. +989121234567) and an error if invalid.
func ValidateIranPhone(phone string) (string, error) {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(phone), " ", ""), "-", "")

	matches := iranPhoneRegex.FindStringSubmatch(cleaned)
	if matches == nil || len(matches) < 2 {
		return "", fmt.Errorf("invalid Iranian phone number")
	}

	return "+98" + matches[1], nil
}

// IranPhone validates an Iranian phone number field and adds an error if invalid.
// Returns the normalized phone number in +98 format.
func (v *Validator) IranPhone(field, value string) string {
	normalized, err := ValidateIranPhone(value)
	if err != nil {
		v.AddError(field, "invalid_phone", "شماره موبایل نامعتبر است")
	}
	return normalized
}

// ===========================================
// HTTP Response Helpers
// ===========================================

// WriteValidationError writes a validation error response.
func WriteValidationError(w http.ResponseWriter, errors []ValidationError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	resp := ErrorResponse{
		Error:   "validation_error",
		Code:    "VALIDATION_ERROR",
		Message: "One or more fields failed validation",
		Details: errors,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// WriteError writes a standardized error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := ErrorResponse{
		Error:   strings.ToLower(strings.ReplaceAll(code, " ", "_")),
		Code:    code,
		Message: message,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// Common error codes
const (
	ErrCodeBadRequest          = "BAD_REQUEST"
	ErrCodeUnauthorized        = "UNAUTHORIZED"
	ErrCodeForbidden           = "FORBIDDEN"
	ErrCodeNotFound            = "NOT_FOUND"
	ErrCodeConflict            = "CONFLICT"
	ErrCodeRateLimitExceeded   = "RATE_LIMIT_EXCEEDED"
	ErrCodeInternalServerError = "INTERNAL_SERVER_ERROR"
	ErrCodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
)

// WriteBadRequest writes a 400 Bad Request response.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteUnauthorized writes a 401 Unauthorized response.
func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// WriteForbidden writes a 403 Forbidden response.
func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// WriteNotFound writes a 404 Not Found response.
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteConflict writes a 409 Conflict response.
func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, ErrCodeConflict, message)
}

// WriteRateLimitExceeded writes a 429 Too Many Requests response.
func WriteRateLimitExceeded(w http.ResponseWriter) {
	WriteError(w, http.StatusTooManyRequests, ErrCodeRateLimitExceeded, "Too many requests. Please try again later.")
}

// WriteInternalError writes a 500 Internal Server Error response.
func WriteInternalError(w http.ResponseWriter) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternalServerError, "An internal error occurred")
}

// WriteServiceUnavailable writes a 503 Service Unavailable response.
func WriteServiceUnavailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, message)
}
