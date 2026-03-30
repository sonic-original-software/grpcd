package service

import (
	"strings"
)

// isValidMethodName checks if a method name is valid according to validation rules
// This must match the validation logic in internal/validate/common.go
// Expects gRPC format: /package.Service/Method
func isValidMethodName(name string) bool {
	if name == "" {
		return false
	}

	// Check for any whitespace
	if strings.ContainsAny(name, " \t\n\r") {
		return false
	}

	// Must start with /
	if !strings.HasPrefix(name, "/") {
		return false
	}

	// Must contain at least two slashes (leading + method separator)
	if strings.Count(name, "/") < 2 {
		return false
	}

	// Cannot contain consecutive slashes
	if strings.Contains(name, "//") {
		return false
	}

	// Cannot end with slash
	if strings.HasSuffix(name, "/") {
		return false
	}

	return true
}
