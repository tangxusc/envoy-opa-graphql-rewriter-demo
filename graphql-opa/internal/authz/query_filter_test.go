package authz

import (
	"context"
	"strings"
	"testing"
)

// salaryReadCheckerFunc is a mock callback for canReadSalary.
func alwaysAllowSalary(_ context.Context, _ User, _ string) (bool, error) {
	return true, nil
}

func alwaysDenySalary(_ context.Context, _ User, _ string) (bool, error) {
	return false, nil
}

func TestFilterGraphQLQuery_BasicFieldRemoval(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) { employeeByID(id: $id) { id name salary } }`
	vars := map[string]any{"id": "bob"}
	user := User{ID: "alice", Role: "user"}

	filtered, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, user, vars, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAllowed {
		t.Fatal("expected hasAllowed=true")
	}
	if len(removed) != 1 || removed[0] != "employeeByID.salary" {
		t.Errorf("removed = %v, want [employeeByID.salary]", removed)
	}
	if strings.Contains(filtered, "salary") {
		t.Errorf("filtered should not contain salary: %s", filtered)
	}
	if !strings.Contains(filtered, "id") || !strings.Contains(filtered, "name") {
		t.Errorf("filtered should contain id and name: %s", filtered)
	}
}

func TestFilterGraphQLQuery_AllAllowed(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) { employeeByID(id: $id) { id name salary } }`
	vars := map[string]any{"id": "alice"}

	filtered, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "admin"}, vars, alwaysAllowSalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAllowed {
		t.Fatal("expected hasAllowed=true")
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty", removed)
	}
	if !strings.Contains(filtered, "salary") {
		t.Errorf("filtered should contain salary: %s", filtered)
	}
}

func TestFilterGraphQLQuery_AllDenied(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) { employeeByID(id: $id) { salary } }`
	vars := map[string]any{"id": "bob"}

	_, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, vars, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasAllowed {
		t.Error("expected hasAllowed=false when all fields denied")
	}
	if len(removed) != 1 {
		t.Errorf("removed = %v, want 1 item", removed)
	}
}

func TestFilterGraphQLQuery_InvalidQuery(t *testing.T) {
	t.Parallel()
	_, _, _, err := filterGraphQLQuery(
		context.Background(), "{{{ invalid", User{}, nil, alwaysAllowSalary,
	)
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestFilterGraphQLQuery_NilVariables(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) { employeeByID(id: $id) { id salary } }`

	_, removed, _, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, nil, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removed) != 1 || removed[0] != "employeeByID.salary" {
		t.Errorf("removed = %v, want [employeeByID.salary]", removed)
	}
}

func TestFilterGraphQLQuery_InlineFragment(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) { employeeByID(id: $id) { id ... on Employee { salary name } } }`
	vars := map[string]any{"id": "bob"}

	filtered, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, vars, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAllowed {
		t.Fatal("expected hasAllowed=true")
	}
	if len(removed) != 1 {
		t.Errorf("removed = %v, want 1 item", removed)
	}
	if strings.Contains(filtered, "salary") {
		t.Errorf("filtered should not contain salary: %s", filtered)
	}
}

func TestFilterGraphQLQuery_FragmentSpread(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) {
  employeeByID(id: $id) {
    id
    ...SalaryPart
  }
}
fragment SalaryPart on Employee {
  salary
}`
	vars := map[string]any{"id": "bob"}

	filtered, removed, _, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, vars, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removed) != 1 || removed[0] != "employeeByID.salary" {
		t.Errorf("removed = %v, want [employeeByID.salary]", removed)
	}
	// Fragment with only salary should be pruned
	if strings.Contains(filtered, "SalaryPart") {
		t.Errorf("filtered should not contain SalaryPart fragment: %s", filtered)
	}
}

func TestFilterGraphQLQuery_FragmentSpreadKept(t *testing.T) {
	t.Parallel()
	query := `query Q($id: String!) {
  employeeByID(id: $id) {
    id
    ...EmployeeFields
  }
}
fragment EmployeeFields on Employee {
  name
  salary
}`
	vars := map[string]any{"id": "bob"}

	filtered, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, vars, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAllowed {
		t.Fatal("expected hasAllowed=true")
	}
	if len(removed) != 1 {
		t.Errorf("removed = %v, want 1 item", removed)
	}
	if strings.Contains(filtered, "salary") {
		t.Errorf("filtered should not contain salary: %s", filtered)
	}
	if !strings.Contains(filtered, "name") {
		t.Errorf("filtered should contain name: %s", filtered)
	}
}

func TestFilterGraphQLQuery_NoEmployeeByID(t *testing.T) {
	t.Parallel()
	query := `query Q { allUsers { id name } }`

	filtered, removed, hasAllowed, err := filterGraphQLQuery(
		context.Background(), query, User{ID: "alice", Role: "user"}, nil, alwaysDenySalary,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAllowed {
		t.Fatal("expected hasAllowed=true")
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty (no employeeByID)", removed)
	}
	if !strings.Contains(filtered, "allUsers") {
		t.Errorf("filtered should contain allUsers: %s", filtered)
	}
}

func TestUniqueSortedStrings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"dedup", []string{"b", "a", "b"}, []string{"a", "b"}},
		{"blank_removed", []string{"a", "", "b"}, []string{"a", "b"}},
		{"nil_input", nil, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueSortedStrings(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestResolveTargetID(t *testing.T) {
	t.Parallel()
	// nil argument
	got := resolveTargetID(nil, nil)
	if got != "" {
		t.Errorf("resolveTargetID(nil, nil) = %q, want empty", got)
	}
}
