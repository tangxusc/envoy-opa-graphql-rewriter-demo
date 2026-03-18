package opa

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testPolicyPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "testdata", "test_policy.rego")
}

func newTestEvaluator(t *testing.T) *Evaluator {
	t.Helper()
	eval, err := NewEvaluator(context.Background(), testPolicyPath(t))
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return eval
}

func TestNewEvaluator_ValidPolicy(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)
	if eval == nil {
		t.Fatal("expected non-nil evaluator")
	}
}

func TestNewEvaluator_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := NewEvaluator(context.Background(), "/nonexistent/policy.rego")
	if err == nil {
		t.Fatal("expected error for invalid policy path")
	}
}

func TestNewEvaluator_InvalidRego(t *testing.T) {
	t.Parallel()
	// Create a temp file with invalid rego
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.rego")
	if err := writeFile(badPath, "not valid rego {{{"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	_, err := NewEvaluator(context.Background(), badPath)
	if err == nil {
		t.Fatal("expected error for invalid rego")
	}
}

func TestEvaluate_AuthenticatedAllow(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: UserInput{
			Authenticated: true,
			Subject:       "alice",
			Roles:         []string{"admin"},
		},
		Request: RequestInput{
			Query:  `{ users { name salary } }`,
			Fields: []string{"name", "salary"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Errorf("expected Allow=true, got false; reason=%s", decision.Reason)
	}
}

func TestEvaluate_UnauthenticatedDeny(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: UserInput{
			Authenticated: false,
		},
		Request: RequestInput{
			Query:  `{ users { name } }`,
			Fields: []string{"name"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Allow {
		t.Error("expected Allow=false for unauthenticated user")
	}
}

func TestEvaluate_DeniedFields(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: UserInput{
			Authenticated: true,
			Subject:       "alice",
			Roles:         []string{"user"},
		},
		Request: RequestInput{
			Query:  `{ users { name salary } }`,
			Fields: []string{"name", "salary"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Error("expected Allow=true for authenticated user")
	}
	found := false
	for _, f := range decision.DeniedFields {
		if f == "salary" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected salary in denied_fields, got %v", decision.DeniedFields)
	}
}

func TestEvaluate_AdminNoFieldDenied(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: UserInput{
			Authenticated: true,
			Subject:       "bob",
			Roles:         []string{"admin"},
		},
		Request: RequestInput{
			Query:  `{ users { name salary } }`,
			Fields: []string{"name", "salary"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Error("expected Allow=true for admin")
	}
	if len(decision.DeniedFields) != 0 {
		t.Errorf("expected no denied_fields for admin, got %v", decision.DeniedFields)
	}
}

func TestEvaluate_EmptyInput(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// With empty input, user is unauthenticated -> deny
	if decision.Allow {
		t.Error("expected Allow=false for empty input")
	}
}

// writeFile is a test helper to write a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
