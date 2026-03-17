package security

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAuthorizerEvaluate(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	policyPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "policy", "authz.rego")
	authorizer, err := NewAuthorizer(policyPath)
	if err != nil {
		t.Fatalf("NewAuthorizer() error = %v", err)
	}

	tests := []struct {
		name   string
		input  PolicyInput
		allow  bool
		reason string
	}{
		{
			name: "public field allows anonymous",
			input: PolicyInput{
				Operation: "query",
				Field:     "publicInfo",
				User: PolicyUser{
					Authenticated: false,
				},
			},
			allow:  true,
			reason: "allowed",
		},
		{
			name: "me requires authentication",
			input: PolicyInput{
				Operation: "query",
				Field:     "me",
				User: PolicyUser{
					Authenticated: false,
				},
			},
			allow:  false,
			reason: "requires authentication",
		},
		{
			name: "createPost rejects normal user",
			input: PolicyInput{
				Operation: "mutation",
				Field:     "createPost",
				User: PolicyUser{
					Authenticated: true,
					Subject:       "user-1",
					Roles:         []string{"user"},
				},
				Args: map[string]interface{}{"title": "Hello"},
			},
			allow:  false,
			reason: "insufficient role",
		},
		{
			name: "createPost allows admin",
			input: PolicyInput{
				Operation: "mutation",
				Field:     "createPost",
				User: PolicyUser{
					Authenticated: true,
					Subject:       "admin-1",
					Roles:         []string{"admin"},
				},
				Args: map[string]interface{}{"title": "Hello"},
			},
			allow:  true,
			reason: "allowed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := authorizer.Evaluate(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if decision.Allow != tc.allow {
				t.Fatalf("Allow = %v, want %v", decision.Allow, tc.allow)
			}
			if decision.Reason != tc.reason {
				t.Fatalf("Reason = %q, want %q", decision.Reason, tc.reason)
			}
		})
	}
}
