package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_FlagError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"-unknown-flag"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "flag error:") {
		t.Fatalf("expected flag error, got %q", errOut.String())
	}
}

func TestRun_ServerMode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		var gotAddr string

		restore := setServerRunner(func(addr string) error {
			gotAddr = addr
			return nil
		})
		defer restore()

		code := run([]string{"-serve", "-addr", ":19090"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d, err=%q", code, errOut.String())
		}
		if gotAddr != ":19090" {
			t.Fatalf("expected addr :19090, got %q", gotAddr)
		}
	})

	t.Run("error", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		restore := setServerRunner(func(addr string) error {
			return fmt.Errorf("boom")
		})
		defer restore()

		code := run([]string{"-serve"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(errOut.String(), "server error: boom") {
			t.Fatalf("expected server error output, got %q", errOut.String())
		}
	})
}

func TestRun_CLIRewrite(t *testing.T) {
	dir := t.TempDir()
	queryPath := filepath.Join(dir, "query.graphql")
	decisionPath := filepath.Join(dir, "decision.json")

	query := `query Q {
  employeeByID(id: "1") {
    id
    salary
  }
}
`
	decision := `{"allow":true,"removed_fields":["employeeByID.salary"]}`
	if err := os.WriteFile(queryPath, []byte(query), 0o600); err != nil {
		t.Fatalf("write query file: %v", err)
	}
	if err := os.WriteFile(decisionPath, []byte(decision), 0o600); err != nil {
		t.Fatalf("write decision file: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"-query", queryPath, "-decision", decisionPath}, &out, &errOut)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, err=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "employeeByID") || strings.Contains(out.String(), "salary") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRun_InputAndRewriteErrors(t *testing.T) {
	t.Run("missing paired flags", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		code := run([]string{"-query", "only.graphql"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(errOut.String(), "both -query and -decision must be provided together") {
			t.Fatalf("unexpected error: %q", errOut.String())
		}
	})

	t.Run("read query file error", func(t *testing.T) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		code := run([]string{"-query", "/no/such/query.graphql", "-decision", "/no/such/decision.json"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(errOut.String(), "read query file") {
			t.Fatalf("unexpected error: %q", errOut.String())
		}
	})

	t.Run("read decision file error", func(t *testing.T) {
		dir := t.TempDir()
		queryPath := filepath.Join(dir, "query.graphql")
		if err := os.WriteFile(queryPath, []byte(`query Q { ping }`), 0o600); err != nil {
			t.Fatalf("write query file: %v", err)
		}

		var out bytes.Buffer
		var errOut bytes.Buffer

		code := run([]string{"-query", queryPath, "-decision", "/no/such/decision.json"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(errOut.String(), "read decision file") {
			t.Fatalf("unexpected error: %q", errOut.String())
		}
	})

	t.Run("rewrite error", func(t *testing.T) {
		dir := t.TempDir()
		queryPath := filepath.Join(dir, "query.graphql")
		decisionPath := filepath.Join(dir, "decision.json")
		if err := os.WriteFile(queryPath, []byte(`query Q { ping }`), 0o600); err != nil {
			t.Fatalf("write query file: %v", err)
		}
		if err := os.WriteFile(decisionPath, []byte(`{"allow":false,"removed_fields":[]}`), 0o600); err != nil {
			t.Fatalf("write decision file: %v", err)
		}

		var out bytes.Buffer
		var errOut bytes.Buffer
		code := run([]string{"-query", queryPath, "-decision", decisionPath}, &out, &errOut)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(errOut.String(), "rewrite error: request denied by policy") {
			t.Fatalf("unexpected error: %q", errOut.String())
		}
	})
}

func TestLoadInputs(t *testing.T) {
	t.Run("default sample", func(t *testing.T) {
		query, decision, err := loadInputs("", "")
		if err != nil {
			t.Fatalf("loadInputs returned error: %v", err)
		}
		if !strings.Contains(query, "EmployeeSalaryWithFragment") {
			t.Fatalf("unexpected default query")
		}
		if !strings.Contains(decision, "removed_fields") {
			t.Fatalf("unexpected default decision")
		}
	})

	t.Run("from files", func(t *testing.T) {
		dir := t.TempDir()
		queryPath := filepath.Join(dir, "query.graphql")
		decisionPath := filepath.Join(dir, "decision.json")

		if err := os.WriteFile(queryPath, []byte("query Q { ping }"), 0o600); err != nil {
			t.Fatalf("write query file: %v", err)
		}
		if err := os.WriteFile(decisionPath, []byte(`{"allow":true,"removed_fields":[]}`), 0o600); err != nil {
			t.Fatalf("write decision file: %v", err)
		}

		query, decision, err := loadInputs(queryPath, decisionPath)
		if err != nil {
			t.Fatalf("loadInputs returned error: %v", err)
		}
		if query != "query Q { ping }" {
			t.Fatalf("unexpected query: %q", query)
		}
		if decision != `{"allow":true,"removed_fields":[]}` {
			t.Fatalf("unexpected decision: %q", decision)
		}
	})
}

func setServerRunner(fn func(addr string) error) func() {
	old := serverRunner
	serverRunner = fn
	return func() {
		serverRunner = old
	}
}
