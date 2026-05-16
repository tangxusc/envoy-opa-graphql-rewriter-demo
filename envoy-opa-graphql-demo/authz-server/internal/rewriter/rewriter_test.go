package rewriter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
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
	_, err := RewriteBody(body, []string{"salary"})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
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

func TestRewriteBody_RemoveObjectFieldWithNestedFields(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employeeByID(id: "emp-1") { id name todos { id name } } }`)
	out, err := RewriteBody(body, []string{"todos"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(string(out), "todos") {
		t.Errorf("expected todos object field to be removed, got %s", out)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	query, ok := payload["query"].(string)
	if !ok {
		t.Fatalf("rewritten query is not a string: %T", payload["query"])
	}
	if _, err := parser.ParseQuery(&ast.Source{Input: query}); err != nil {
		t.Fatalf("rewritten query is invalid GraphQL: %v; query=%s", err, query)
	}
}

func TestRewriteBody_NestedEmptySelectionSet_ParentRemoved(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { id profile { salary } } }`)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "profile") {
		t.Errorf("expected profile to be removed (empty children), got %s", out)
	}
	if !strings.Contains(string(out), "id") {
		t.Errorf("expected id to remain, got %s", out)
	}
}

func TestRewriteBody_NestedAllRemoved_ReturnsError(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { profile { salary } } }`)
	_, err := RewriteBody(body, []string{"salary"})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRewriteBody_NamedFragment_Empty_SpreadRemoved(t *testing.T) {
	t.Parallel()
	query := `fragment Sensitive on Employee { salary }
query { employees { name ...Sensitive } }`
	body := mustMakeBody(t, query)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "Sensitive") {
		t.Errorf("expected empty fragment spread removed, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
}

func TestRewriteBody_NamedFragment_Partial(t *testing.T) {
	t.Parallel()
	query := `fragment EmpFields on Employee { salary name department }
query { employees { id ...EmpFields } }`
	body := mustMakeBody(t, query)
	out, err := RewriteBody(body, []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed from fragment, got %s", out)
	}
	if !strings.Contains(string(out), "EmpFields") {
		t.Errorf("expected fragment spread to remain, got %s", out)
	}
	if !strings.Contains(string(out), "department") {
		t.Errorf("expected department to remain, got %s", out)
	}
}

func TestRewriteBody_BatchQuery(t *testing.T) {
	t.Parallel()
	batch := `[{"query":"{ users { name salary } }"},{"query":"{ posts { title } }"}]`
	out, err := RewriteBody([]byte(batch), []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed from batch, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
	if !strings.Contains(string(out), "title") {
		t.Errorf("expected title to remain, got %s", out)
	}
}

func TestRewriteBody_BatchQuery_OneEmpty(t *testing.T) {
	t.Parallel()
	batch := `[{"query":"{ users { salary } }"},{"query":"{ posts { title } }"}]`
	_, err := RewriteBody([]byte(batch), []string{"salary"})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestRewriteBody_BatchQuery_NoQueryField(t *testing.T) {
	t.Parallel()
	batch := `[{"variables":{}},{"query":"{ posts { title salary } }"}]`
	out, err := RewriteBody([]byte(batch), []string{"salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed, got %s", out)
	}
	if !strings.Contains(string(out), "title") {
		t.Errorf("expected title to remain, got %s", out)
	}
}

func TestPathDenied_BasicPath(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { name salary } }`)
	out, err := RewriteBody(body, []string{"employees.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed under employees, got %s", out)
	}
	if !strings.Contains(string(out), "name") {
		t.Errorf("expected name to remain, got %s", out)
	}
}

func TestPathDenied_NoMatchAtWrongLevel(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { name salary } company { salary revenue } }`)
	out, err := RewriteBody(body, []string{"employees.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	query := payload["query"].(string)
	if !strings.Contains(query, "salary") {
		t.Errorf("expected company.salary to remain, got query: %s", query)
	}
}

func TestPathDenied_MultiLevel(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { profile { salary bio } } }`)
	out, err := RewriteBody(body, []string{"employees.profile.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed, got %s", out)
	}
	if !strings.Contains(string(out), "bio") {
		t.Errorf("expected bio to remain, got %s", out)
	}
}

func TestPathDenied_MultiLevel_NoMatchShallow(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { salary profile { salary } } }`)
	out, err := RewriteBody(body, []string{"employees.profile.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	query := payload["query"].(string)
	if strings.Contains(query, "profile") {
		t.Errorf("expected profile removed (empty), got: %s", query)
	}
	if !strings.Contains(query, "salary") {
		t.Errorf("expected shallow salary to remain, got: %s", query)
	}
}

func TestPathDenied_GlobalStillWorks(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { salary } company { salary } }`)
	_, err := RewriteBody(body, []string{"salary"})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestPathDenied_MixedGlobalAndPath(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { salary ssn } company { salary ssn } }`)
	out, err := RewriteBody(body, []string{"ssn", "employees.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "ssn") {
		t.Errorf("expected ssn removed globally, got %s", out)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	query := payload["query"].(string)
	if !strings.Contains(query, "salary") {
		t.Errorf("expected company.salary to remain, got: %s", query)
	}
}

func TestPathDenied_InlineFragment(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { ... on Manager { salary bonus } } }`)
	out, err := RewriteBody(body, []string{"employees.salary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "salary") {
		t.Errorf("expected salary removed, got %s", out)
	}
	if !strings.Contains(string(out), "bonus") {
		t.Errorf("expected bonus to remain, got %s", out)
	}
}

func TestPathDenied_EmptyAfterPathFilter(t *testing.T) {
	t.Parallel()
	body := mustMakeBody(t, `{ employees { salary } }`)
	_, err := RewriteBody(body, []string{"employees.salary"})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}
