package validate

import (
	"testing"
)

func TestMethodName(t *testing.T) {
	tests := []struct {
		name           string
		methodName     string
		expectViolation bool
	}{
		{
			name:           "valid gRPC method",
			methodName:     "/principal.pb.PrincipalService/GetPrincipal",
			expectViolation: false,
		},
		{
			name:           "valid short gRPC method",
			methodName:     "/PrincipalService/GetPrincipal",
			expectViolation: false,
		},
		{
			name:           "empty method name",
			methodName:     "",
			expectViolation: true,
		},
		{
			name:           "method with leading whitespace",
			methodName:     " /principal.pb.PrincipalService/GetPrincipal",
			expectViolation: true,
		},
		{
			name:           "method with trailing whitespace",
			methodName:     "/principal.pb.PrincipalService/GetPrincipal ",
			expectViolation: true,
		},
		{
			name:           "method with internal whitespace",
			methodName:     "/principal.pb.PrincipalService/ GetPrincipal",
			expectViolation: true,
		},
		{
			name:           "method without leading slash",
			methodName:     "principal.pb.PrincipalService/GetPrincipal",
			expectViolation: true,
		},
		{
			name:           "method with consecutive slashes",
			methodName:     "/principal.pb.PrincipalService//GetPrincipal",
			expectViolation: true,
		},
		{
			name:           "method ending with slash",
			methodName:     "/principal.pb.PrincipalService/GetPrincipal/",
			expectViolation: true,
		},
		{
			name:           "method without method separator",
			methodName:     "/principal.pb.PrincipalService",
			expectViolation: true,
		},
		{
			name:           "old dot format without leading slash",
			methodName:     "principal.Service.Method",
			expectViolation: true,
		},
		{
			name:           "old dot format with leading slash but no slash separator",
			methodName:     "/principal.Service.Method",
			expectViolation: true,
		},
		{
			name:           "dots in method name after slash",
			methodName:     "/principal.Service/Method.Name",
			expectViolation: false, // dots are technically valid in identifiers
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := MethodName(tt.methodName)

			if tt.expectViolation {
				if len(violations) == 0 {
					t.Errorf("expected violation, got none")
				}
			} else {
				if len(violations) > 0 {
					t.Errorf("expected no violations, got: %v", violations)
				}
			}
		})
	}
}
