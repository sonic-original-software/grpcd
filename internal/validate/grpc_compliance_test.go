package validate

import (
	"testing"
)

// TestValidation_GRPCCompliance verifies that our validation follows gRPC's
// method name format: /package.Service/Method
//
// gRPC spec: https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md
// Method names are: "/" Service-Name "/" {method name}
func TestValidation_GRPCCompliance(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		shouldAccept bool
		reason      string
	}{
		// Valid gRPC formats - MUST be accepted
		{
			name:        "standard format with package",
			method:      "/package.Service/Method",
			shouldAccept: true,
			reason:      "Standard gRPC format",
		},
		{
			name:        "nested package",
			method:      "/com.example.api.Service/Method",
			shouldAccept: true,
			reason:      "Nested packages are valid",
		},
		{
			name:        "pb suffix common pattern",
			method:      "/package.pb.Service/Method",
			shouldAccept: true,
			reason:      "Proto packages often use .pb suffix",
		},
		{
			name:        "short format without package",
			method:      "/Service/Method",
			shouldAccept: true,
			reason:      "Service without package is valid",
		},

		// Invalid formats - MUST be rejected
		{
			name:        "missing leading slash",
			method:      "package.Service/Method",
			shouldAccept: false,
			reason:      "gRPC requires leading /",
		},
		{
			name:        "missing method separator slash",
			method:      "/package.Service",
			shouldAccept: false,
			reason:      "gRPC requires /Method after service",
		},
		{
			name:        "old dot notation",
			method:      "package.Service.Method",
			shouldAccept: false,
			reason:      "Not gRPC format",
		},
		{
			name:        "dot notation with leading slash",
			method:      "/package.Service.Method",
			shouldAccept: false,
			reason:      "Has / but missing method separator",
		},
		{
			name:        "consecutive slashes",
			method:      "/package.Service//Method",
			shouldAccept: false,
			reason:      "gRPC does not allow //",
		},
		{
			name:        "trailing slash",
			method:      "/package.Service/Method/",
			shouldAccept: false,
			reason:      "gRPC does not allow trailing /",
		},
		{
			name:        "whitespace in method",
			method:      "/package.Service/ Method",
			shouldAccept: false,
			reason:      "Proto identifiers cannot contain whitespace",
		},
		{
			name:        "empty string",
			method:      "",
			shouldAccept: false,
			reason:      "Empty method name invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := MethodName(tt.method)
			hasViolations := len(violations) > 0

			if tt.shouldAccept && hasViolations {
				t.Errorf("FAILED: Should accept %q (%s) but got violations: %v",
					tt.method, tt.reason, violations)
			} else if !tt.shouldAccept && !hasViolations {
				t.Errorf("FAILED: Should reject %q (%s) but validation passed",
					tt.method, tt.reason)
			} else if tt.shouldAccept {
				t.Logf("✓ Accepts: %s - %s", tt.method, tt.reason)
			} else {
				t.Logf("✓ Rejects: %s - %s", tt.method, tt.reason)
			}
		})
	}
}
