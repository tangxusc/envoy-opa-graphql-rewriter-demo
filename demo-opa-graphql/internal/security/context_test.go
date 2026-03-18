package security

import (
	"context"
	"testing"
)

func TestWithPrincipalAndPrincipalFromContext(t *testing.T) {
	t.Parallel()

	p := &Principal{Subject: "alice", Roles: []string{"admin", "viewer"}}
	ctx := WithPrincipal(context.Background(), p)

	got, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("expected principal to be found in context")
	}
	if got.Subject != "alice" {
		t.Errorf("Subject = %q, want %q", got.Subject, "alice")
	}
	if len(got.Roles) != 2 || got.Roles[0] != "admin" || got.Roles[1] != "viewer" {
		t.Errorf("Roles = %v, want [admin viewer]", got.Roles)
	}
}

func TestPrincipalFromContext_Missing(t *testing.T) {
	t.Parallel()

	_, ok := PrincipalFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
}

func TestPrincipalFromContext_NilPrincipal(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), nil)
	_, ok := PrincipalFromContext(ctx)
	if ok {
		t.Error("expected ok=false for nil principal")
	}
}
