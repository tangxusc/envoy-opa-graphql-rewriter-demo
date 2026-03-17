package authz

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testSchema = `
type Employee {
  id: String!
  name: String!
  salary: Int!
}

schema {
  query: Query
}

type Query {
  employeeByID(id: String!): Employee
}
`

const salaryByVariableQuery = `
query EmployeeSalary($id: String!) {
  employeeByID(id: $id) {
    salary
  }
}
`

const employeeFieldsQuery = `
query EmployeeFields($id: String!) {
  employeeByID(id: $id) {
    id
    salary
  }
}
`

const invalidFieldQuery = `
query InvalidField($id: String!) {
  employeeByID(id: $id) {
    salary
    secret
  }
}
`

const salaryViaFragmentQuery = `
query EmployeeSalaryWithFragment($id: String!) {
  employeeByID(id: $id) {
    id
    ...SalaryPart
  }
}

fragment SalaryPart on Employee {
  salary
}
`

func TestEvaluatorEvaluate(t *testing.T) {
	ctx := context.Background()
	evaluator := newTestEvaluator(t, ctx)

	tests := []struct {
		name                   string
		input                  Input
		wantAllow              bool
		wantQueryValid         bool
		wantReasons            []string
		wantRemovedFields      []string
		wantFilteredEmpty      bool
		wantFilteredContains   []string
		wantFilteredNotContain []string
	}{
		{
			name: "allow own salary by variable",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     salaryByVariableQuery,
					User:      User{ID: "alice", Role: "user"},
					Variables: map[string]any{"id": "alice"},
				},
			},
			wantAllow:         true,
			wantQueryValid:    true,
			wantReasons:       []string{},
			wantRemovedFields: []string{},
			wantFilteredContains: []string{
				"salary",
			},
		},
		{
			name: "filter salary for another user and keep id",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     employeeFieldsQuery,
					User:      User{ID: "alice", Role: "user"},
					Variables: map[string]any{"id": "bob"},
				},
			},
			wantAllow:      true,
			wantQueryValid: true,
			wantReasons: []string{
				"removed unauthorized fields: employeeByID.salary",
			},
			wantRemovedFields: []string{"employeeByID.salary"},
			wantFilteredContains: []string{
				"id",
			},
			wantFilteredNotContain: []string{
				"salary",
			},
		},
		{
			name: "remove all fields when only unauthorized salary is requested",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     salaryByVariableQuery,
					User:      User{ID: "alice", Role: "user"},
					Variables: map[string]any{"id": "bob"},
				},
			},
			wantAllow:      false,
			wantQueryValid: true,
			wantReasons: []string{
				"no fields left after authorization filtering",
				"removed unauthorized fields: employeeByID.salary",
			},
			wantRemovedFields: []string{"employeeByID.salary"},
			wantFilteredEmpty: true,
		},
		{
			name: "deny invalid query against schema",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     invalidFieldQuery,
					User:      User{ID: "alice", Role: "admin"},
					Variables: map[string]any{"id": "alice"},
				},
			},
			wantAllow:         false,
			wantQueryValid:    false,
			wantReasons:       []string{"query failed GraphQL schema verification"},
			wantRemovedFields: []string{},
			wantFilteredEmpty: true,
		},
		{
			name: "filter salary when variable is missing and keep id",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     employeeFieldsQuery,
					User:      User{ID: "alice", Role: "user"},
					Variables: map[string]any{},
				},
			},
			wantAllow:      true,
			wantQueryValid: true,
			wantReasons: []string{
				"removed unauthorized fields: employeeByID.salary",
			},
			wantRemovedFields: []string{"employeeByID.salary"},
			wantFilteredContains: []string{
				"id",
			},
			wantFilteredNotContain: []string{
				"salary",
			},
		},
		{
			name: "filter salary inside fragment and keep other fields",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     salaryViaFragmentQuery,
					User:      User{ID: "alice", Role: "user"},
					Variables: map[string]any{"id": "bob"},
				},
			},
			wantAllow:      true,
			wantQueryValid: true,
			wantReasons: []string{
				"removed unauthorized fields: employeeByID.salary",
			},
			wantRemovedFields: []string{"employeeByID.salary"},
			wantFilteredContains: []string{
				"id",
			},
			wantFilteredNotContain: []string{
				"salary",
				"fragment SalaryPart",
			},
		},
		{
			name: "admin can keep salary for any user",
			input: Input{
				Input: GraphQLRequest{
					Schema:    testSchema,
					Query:     salaryByVariableQuery,
					User:      User{ID: "alice", Role: "admin"},
					Variables: map[string]any{"id": "bob"},
				},
			},
			wantAllow:         true,
			wantQueryValid:    true,
			wantReasons:       []string{},
			wantRemovedFields: []string{},
			wantFilteredContains: []string{
				"salary",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decision, err := evaluator.Evaluate(ctx, tc.input)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Allow != tc.wantAllow {
				t.Fatalf("allow mismatch: got %v, want %v", decision.Allow, tc.wantAllow)
			}
			if decision.QueryValid != tc.wantQueryValid {
				t.Fatalf("query_valid mismatch: got %v, want %v", decision.QueryValid, tc.wantQueryValid)
			}

			if !sameStringSlice(decision.Reasons, tc.wantReasons) {
				t.Fatalf("reasons mismatch: got %v, want %v", decision.Reasons, tc.wantReasons)
			}
			if !sameStringSlice(decision.RemovedFields, tc.wantRemovedFields) {
				t.Fatalf("removed_fields mismatch: got %v, want %v", decision.RemovedFields, tc.wantRemovedFields)
			}

			if tc.wantFilteredEmpty {
				if decision.FilteredQuery != "" {
					t.Fatalf("filtered_query mismatch: got %q, want empty", decision.FilteredQuery)
				}
			} else {
				if decision.FilteredQuery == "" {
					t.Fatal("filtered_query mismatch: got empty, want non-empty")
				}
			}

			for _, wantContains := range tc.wantFilteredContains {
				if !strings.Contains(decision.FilteredQuery, wantContains) {
					t.Fatalf("filtered_query should contain %q, got %q", wantContains, decision.FilteredQuery)
				}
			}
			for _, wantNotContain := range tc.wantFilteredNotContain {
				if strings.Contains(decision.FilteredQuery, wantNotContain) {
					t.Fatalf("filtered_query should not contain %q, got %q", wantNotContain, decision.FilteredQuery)
				}
			}
		})
	}
}

func newTestEvaluator(t *testing.T, ctx context.Context) *Evaluator {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}

	policyPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "policy", "graphql_authz.rego")

	evaluator, err := NewEvaluator(ctx, policyPath)
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	return evaluator
}

func sameStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
