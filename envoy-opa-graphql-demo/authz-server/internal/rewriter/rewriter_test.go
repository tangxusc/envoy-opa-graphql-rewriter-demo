package rewriter

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustMakeBody(t *testing.T, query string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{"query": query})
	if err != nil {
		t.Fatalf("mustMakeBody: %v", err)
	}
	return b
}

func TestRewriteBody_NoDeniedFields(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name salary } }`)
	out, err := RewriteBody(body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("expected unchanged body, got %s", out)
	}
}

func TestRewriteBody_SingleFieldRemoval(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name salary age } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
	if !strings.Contains(string(out), "age") {
		t.Errorf("expected age to remain, got %s", out)
	}
}

func TestRewriteBody_MultipleFieldRemoval(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name salary age } }`)
	out, err := RewriteBody(body, []string{"salary", "age"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") || strings.Contains(string(out), "age") {
		t.Errorf("expected salary and age to be removed, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
}

func TestRewriteBody_AllFieldsRemoved(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { salary } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed, got %s", out)
	}
	// Should still parse as valid JSON
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestRewriteBody_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := RewriteBody([]byte("not-json"), []string{"salary"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRewriteBody_InvalidGraphQL(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{{{ invalid`)
	_, err := RewriteBody(body, []string{"salary"})
	if err == nil {
		t.Fatal("expected error for invalid GraphQL")
	}
}

func TestRewriteBody_NoQueryField(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(map[string]interface{}{"variables": map[string]interface{}{}})
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("expected unchanged body when no query field, got %s", out)
	}
}

func TestRewriteBody_QueryFieldNotString(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(map[string]interface{}{"query": 123})
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return body unchanged since query is not a string
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestRewriteBody_NestedFields(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name profile { salary bio } } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected nested salary to be removed, got %s", out)
	}
	if !strings.Contains(string(out), "bio") {
		t.Errorf("expected bio to remain, got %s", out)
	}
}

func TestRewriteBody_InlineFragment(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name ... on Employee { salary department } } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary in inline fragment to be removed, got %s", out)
	}
	if !strings.Contains(string(out), "department") {
		t.Errorf("expected department to remain, got %s", out)
	}
}

func TestRewriteBody_InlineFragmentAllRemoved(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ users { name ... on Employee { salary } } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed, got %s", out)
	}
	// The empty inline fragment should be removed entirely
	if strings.Contains(string(out), "Employee") {
		t.Errorf("expected empty inline fragment to be removed, got %s", out)
	}
}

func TestRewriteBody_SubscriptionRewrite(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `subscription { employeeUpdated { id name salary } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed from subscription, got %s", out)
	}
	if !strings.Contains(string(out), "subscription") {
		t.Errorf("expected subscription keyword to remain, got %s", out)
	}
	if !strings.Contains(string(out), "id") {
		t.Errorf("expected id to remain, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
}

func TestRewriteBody_SubscriptionWithArgs(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `subscription { employeeUpdated(id: "emp-1") { id name salary } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed from subscription with args, got %s", out)
	}
	if !strings.Contains(string(out), "employeeUpdated") {
		t.Errorf("expected employeeUpdated to remain, got %s", out)
	}
}

func TestRewriteBody_MutationRewrite(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `mutation { updateEmployee(id: "emp-1", name: "Alice Updated") { id name salary } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary to be removed from mutation, got %s", out)
	}
	if !strings.Contains(string(out), "mutation") {
		t.Errorf("expected mutation keyword to remain, got %s", out)
	}
}
