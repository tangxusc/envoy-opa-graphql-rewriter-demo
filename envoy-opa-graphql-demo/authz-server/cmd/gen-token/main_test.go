package main

import (
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"authz-server/internal/privilege"
)

func TestIssueToken(t *testing.T) {
	t.Parallel()
	token, err := issueToken("test-user", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("issueToken: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestIssueToken_EmptySubject(t *testing.T) {
	t.Parallel()
	token, err := issueToken("", []string{}, time.Hour)
	if err != nil {
		t.Fatalf("issueToken: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token even with empty subject")
	}
}

func TestIssueToken_ContainsPrivileges(t *testing.T) {
	t.Parallel()
	roles := []string{"admin"}
	token, err := issueToken("test-user", roles, time.Hour)
	if err != nil {
		t.Fatalf("issueToken: %v", err)
	}

	// Parse the token and verify privileges field
	claims := &Claims{}
	_, err = gojwt.ParseWithClaims(token, claims, func(token *gojwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Privileges == "" {
		t.Error("expected non-empty privileges in token")
	}

	// Verify the bloom filter contains expected privileges
	for _, p := range privilege.RolePrivileges["admin"] {
		ok, err := privilege.HasPrivilege(claims.Privileges, p)
		if err != nil {
			t.Fatalf("HasPrivilege(%q): %v", p, err)
		}
		if !ok {
			t.Errorf("expected privilege %q to be present in bloom filter", p)
		}
	}
}
