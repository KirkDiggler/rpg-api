package errors

import (
	"fmt"
	"strings"
)

// ValidationError represents a validation error with multiple fields.
// It collects validation errors for multiple fields and can convert
// itself to a standard Error with InvalidArgument code.
type ValidationError struct {
	// Fields maps field names to their validation error messages
	Fields map[string][]string `json:"fields"`
}

// Error implements the error interface
func (v *ValidationError) Error() string {
	if len(v.Fields) == 0 {
		return "validation failed"
	}

	parts := make([]string, len(v.Fields))
	i := 0
	for field, errs := range v.Fields {
		parts[i] = fmt.Sprintf("%s: %s", field, strings.Join(errs, ", "))
		i++
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(parts, "; "))
}

// NewValidationError creates a new validation error
func NewValidationError() *ValidationError {
	return &ValidationError{
		Fields: make(map[string][]string),
	}
}

// AddFieldError adds an error for a specific field
func (v *ValidationError) AddFieldError(field, message string) {
	v.Fields[field] = append(v.Fields[field], message)
}

// AddFieldErrorf adds a formatted error for a specific field
func (v *ValidationError) AddFieldErrorf(field, format string, args ...interface{}) {
	v.AddFieldError(field, fmt.Sprintf(format, args...))
}

// HasErrors returns true if there are any validation errors
func (v *ValidationError) HasErrors() bool {
	return len(v.Fields) > 0
}

// ToError converts the validation error to our standard error type
func (v *ValidationError) ToError() *Error {
	if !v.HasErrors() {
		return nil
	}

	err := InvalidArgument(v.Error())
	return err.WithMeta("validation_errors", v.Fields)
}

// ValidationBuilder provides a fluent interface for building validation errors.
// It accumulates field-level validation errors and returns nil if no errors
// are present, or an InvalidArgument error with detailed field information.
type ValidationBuilder struct {
	err *ValidationError
}

// NewValidationBuilder creates a new validation builder
func NewValidationBuilder() *ValidationBuilder {
	return &ValidationBuilder{
		err: NewValidationError(),
	}
}

// Field adds a validation error for a field
func (vb *ValidationBuilder) Field(field, message string) *ValidationBuilder {
	vb.err.AddFieldError(field, message)
	return vb
}

// Fieldf adds a formatted validation error for a field
func (vb *ValidationBuilder) Fieldf(field, format string, args ...interface{}) *ValidationBuilder {
	vb.err.AddFieldErrorf(field, format, args...)
	return vb
}

// RequiredField adds a required field error
func (vb *ValidationBuilder) RequiredField(field string) *ValidationBuilder {
	return vb.Field(field, "is required")
}

// InvalidField adds an invalid field error
func (vb *ValidationBuilder) InvalidField(field, reason string) *ValidationBuilder {
	return vb.Fieldf(field, "is invalid: %s", reason)
}

// Build returns the error if there are validation errors, nil otherwise
func (vb *ValidationBuilder) Build() error {
	if vb.err.HasErrors() {
		return vb.err.ToError()
	}
	return nil
}

// Validation helper functions

// ValidateRequired checks if a string field is required
func ValidateRequired(field, value string, vb *ValidationBuilder) {
	if strings.TrimSpace(value) == "" {
		vb.RequiredField(field)
	}
}

// ValidateMinLength checks if a string meets minimum length
func ValidateMinLength(field, value string, minValue int, vb *ValidationBuilder) {
	if len(value) < minValue {
		vb.Fieldf(field, "must be at least %d characters", minValue)
	}
}

// ValidateMaxLength checks if a string meets maximum length
func ValidateMaxLength(field, value string, maxValue int, vb *ValidationBuilder) {
	if len(value) > maxValue {
		vb.Fieldf(field, "must be no more than %d characters", maxValue)
	}
}

// ValidateRange checks if a value is within a range
func ValidateRange(field string, value, minValue, maxValue int, vb *ValidationBuilder) {
	if value < minValue || value > maxValue {
		vb.Fieldf(field, "must be between %d and %d", minValue, maxValue)
	}
}

// ValidateEnum checks if a value is in a list of allowed values
func ValidateEnum(field, value string, allowed []string, vb *ValidationBuilder) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	vb.Fieldf(field, "must be one of: %s", strings.Join(allowed, ", "))
}
