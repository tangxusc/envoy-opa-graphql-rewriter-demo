package main

import (
	"testing"
	"time"
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
