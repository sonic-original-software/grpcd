package validate

import (
	"testing"
)

func TestMethods(t *testing.T) {
	tests := []struct {
		name           string
		methods        []string
		expectViolation bool
		expectedField  string
	}{
		{
			name:           "empty list",
			methods:        []string{},
			expectViolation: true,
			expectedField:  "methods",
		},
		{
			name:           "nil list",
			methods:        nil,
			expectViolation: true,
			expectedField:  "methods",
		},
		{
			name:           "valid single method",
			methods:        []string{"/package.Service/Method"},
			expectViolation: false,
		},
		{
			name:           "valid multiple methods",
			methods:        []string{"/package.Service/Method", "/package.Service/Other"},
			expectViolation: false,
		},
		{
			name:           "valid short method",
			methods:        []string{"/Service/Method"},
			expectViolation: false,
		},
		{
			name:           "empty method name",
			methods:        []string{""},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method with leading whitespace",
			methods:        []string{" /package.Service/Method"},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method with trailing whitespace",
			methods:        []string{"/package.Service/Method "},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method without leading slash",
			methods:        []string{"package.Service/Method"},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method with consecutive slashes",
			methods:        []string{"/package.Service//Method"},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method ending with slash",
			methods:        []string{"/package.Service/Method/"},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "method without method separator",
			methods:        []string{"/package.Service"},
			expectViolation: true,
			expectedField:  "methods[0]",
		},
		{
			name:           "mixed valid and invalid",
			methods:        []string{"/package.Service/Method", "InvalidMethod"},
			expectViolation: true,
			expectedField:  "methods[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := Methods(tt.methods)

			if tt.expectViolation {
				if len(violations) == 0 {
					t.Errorf("expected violation, got none")
				}
				if tt.expectedField != "" {
					found := false
					for _, v := range violations {
						if v.Field == tt.expectedField {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected violation for field %s, got fields: %v", tt.expectedField, violations)
					}
				}
			} else {
				if len(violations) > 0 {
					t.Errorf("expected no violations, got: %v", violations)
				}
			}
		})
	}
}
