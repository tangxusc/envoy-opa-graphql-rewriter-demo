package opa

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"authz-server/internal/privilege"
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

// buildTestUserInput is a helper to construct UserInput with privileges encoded from roles.
func buildTestUserInput(t *testing.T, authenticated bool, subject string, roles []string) UserInput {
	t.Helper()
	privStr, err := privilege.Encode(roles)
	if err != nil {
		t.Fatalf("privilege.Encode: %v", err)
	}
	return UserInput{
		Authenticated: authenticated,
		Subject:       subject,
		Roles:         roles,
		CurrentTime:   time.Now().UTC().Format(time.RFC3339),
		Privileges:    privStr,
	}
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
		User: buildTestUserInput(t, true, "alice", []string{"admin"}),
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
		User: buildTestUserInput(t, true, "alice", []string{"user"}),
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
		User: buildTestUserInput(t, true, "bob", []string{"admin"}),
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

func TestEvaluate_HRCanReadSalary(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: buildTestUserInput(t, true, "carol", []string{"hr"}),
		Request: RequestInput{
			Query:  `{ users { name salary } }`,
			Fields: []string{"name", "salary"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Error("expected Allow=true for hr")
	}
	// HR has read:salary privilege, so salary should NOT be denied
	for _, f := range decision.DeniedFields {
		if f == "salary" {
			t.Error("expected salary NOT in denied_fields for hr role")
		}
	}
}

func TestEvaluate_HasPrivilegeBuiltin(t *testing.T) {
	t.Parallel()
	eval := newTestEvaluator(t)

	// User role does NOT have read:salary -> salary should be denied
	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: buildTestUserInput(t, true, "dave", []string{"user"}),
		Request: RequestInput{
			Query:  `{ users { name salary } }`,
			Fields: []string{"name", "salary"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Error("expected Allow=true")
	}
	found := false
	for _, f := range decision.DeniedFields {
		if f == "salary" {
			found = true
		}
	}
	if !found {
		t.Error("expected salary in denied_fields for user role without read:salary")
	}
}

// writeFile is a test helper to write a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func testPoliciesDirPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "testdata", "policies")
}

func testRecursiveDirPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "testdata", "recursive")
}

func TestNewEvaluator_Directory(t *testing.T) {
	t.Parallel()
	eval, err := NewEvaluator(context.Background(), testPoliciesDirPath(t))
	if err != nil {
		t.Fatalf("NewEvaluator from directory: %v", err)
	}
	if eval == nil {
		t.Fatal("expected non-nil evaluator for directory")
	}
}

func TestNewEvaluator_Subdirectory(t *testing.T) {
	t.Parallel()
	eval, err := NewEvaluator(context.Background(), testRecursiveDirPath(t))
	if err != nil {
		t.Fatalf("NewEvaluator from recursive directory: %v", err)
	}
	if eval == nil {
		t.Fatal("expected non-nil evaluator for recursive directory")
	}
}

func TestNewEvaluator_InvalidPath_SingleFile(t *testing.T) {
	t.Parallel()
	_, err := NewEvaluator(context.Background(), "/nonexistent/policy.rego")
	if err == nil {
		t.Fatal("expected error for invalid policy path")
	}
}

func TestNewEvaluator_InvalidRegoInDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.rego")
	if err := writeFile(badPath, "invalid syntax {{{"); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	_, err := NewEvaluator(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for directory containing invalid rego")
	}
}

func TestEvaluate_MultiFileMerged(t *testing.T) {
	t.Parallel()
	eval, err := NewEvaluator(context.Background(), testPoliciesDirPath(t))
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}

	decision, err := eval.Evaluate(context.Background(), EvalInput{
		User: buildTestUserInput(t, true, "alice", []string{"user"}),
		Request: RequestInput{
			Query:  `{ users { name salary email } }`,
			Fields: []string{"name", "salary", "email"},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allow {
		t.Error("expected Allow=true for authenticated user")
	}

	denied := make(map[string]bool)
	for _, f := range decision.DeniedFields {
		denied[f] = true
	}

	if !denied["salary"] {
		t.Error("expected salary in denied_fields (from authz.rego)")
	}
	if !denied["email"] {
		t.Error("expected email in denied_fields (from rbac.rego)")
	}
}
