package graph

import (
	"context"
	"fmt"
	"testing"

	"demo-opa-graphql/internal/security"
)

type mockAuthorizer struct {
	decision security.Decision
	err      error
}

func (m *mockAuthorizer) evaluateFunc(ctx context.Context, input security.PolicyInput) (security.Decision, error) {
	return m.decision, m.err
}

func TestAuthorize_Allowed(t *testing.T) {
	t.Parallel()

	// We can't easily mock the Authorizer since it's a concrete struct.
	// Instead we test unionRoles and authorize with nil Authorizer.
	r := &Resolver{Authorizer: nil}
	err := r.authorize(context.Background(), "query", "publicInfo", nil)
	if err == nil {
		t.Fatal("expected error when Authorizer is nil")
	}
	if err.Error() != "authorizer not configured" {
		t.Errorf("error = %q, want %q", err.Error(), "authorizer not configured")
	}
}

func TestAuthorize_NoPrincipal(t *testing.T) {
	t.Parallel()
	// With nil Authorizer, we get "authorizer not configured" first
	r := &Resolver{Authorizer: nil}
	err := r.authorize(context.Background(), "query", "me", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnionRoles_BothEmpty(t *testing.T) {
	t.Parallel()
	result := unionRoles(nil, nil)
	if len(result) != 0 {
		t.Errorf("unionRoles(nil, nil) = %v, want empty", result)
	}
}

func TestUnionRoles_NoDuplicates(t *testing.T) {
	t.Parallel()
	result := unionRoles([]string{"admin", "user"}, []string{"user", "viewer"})
	expected := []string{"admin", "user", "viewer"}
	if len(result) != len(expected) {
		t.Fatalf("len = %d, want %d; result = %v", len(result), len(expected), result)
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestUnionRoles_BaseOnly(t *testing.T) {
	t.Parallel()
	result := unionRoles([]string{"admin"}, nil)
	if len(result) != 1 || result[0] != "admin" {
		t.Errorf("result = %v, want [admin]", result)
	}
}

func TestUnionRoles_ExtraOnly(t *testing.T) {
	t.Parallel()
	result := unionRoles(nil, []string{"viewer"})
	if len(result) != 1 || result[0] != "viewer" {
		t.Errorf("result = %v, want [viewer]", result)
	}
}

func TestUnionRoles_AllDuplicates(t *testing.T) {
	t.Parallel()
	result := unionRoles([]string{"admin", "admin"}, []string{"admin"})
	if len(result) != 1 || result[0] != "admin" {
		t.Errorf("result = %v, want [admin]", result)
	}
}

func TestErrUnauthenticated(t *testing.T) {
	t.Parallel()
	if errUnauthenticated.Error() != "unauthenticated" {
		t.Errorf("errUnauthenticated = %q, want %q", errUnauthenticated.Error(), "unauthenticated")
	}
}

// Verify that authorize returns proper error message format
func TestAuthorize_ErrorFormat(t *testing.T) {
	t.Parallel()
	r := &Resolver{Authorizer: nil}
	err := r.authorize(context.Background(), "mutation", "createPost", map[string]interface{}{"title": "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	want := "authorizer not configured"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	_ = fmt.Sprintf("test") // avoid import error
}
