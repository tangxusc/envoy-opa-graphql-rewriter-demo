package main

import (
	"context"
	"fmt"
	"log"

	"graphql-opa/internal/authz"
)

const schema = `
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

const salaryQuery = `
query EmployeeSalary($id: String!) {
  employeeByID(id: $id) {
    salary
  }
}
`

const employeeProfileAndSalaryQuery = `
query EmployeeProfileAndSalary($id: String!) {
  employeeByID(id: $id) {
    id
    name
    salary
  }
}
`

const salaryByLiteralIDQuery = `
query EmployeeSalaryLiteral {
  employeeByID(id: "bob") {
    id
    salary
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

const publicOnlyQuery = `
query EmployeePublicInfo($id: String!) {
  employeeByID(id: $id) {
    id
    name
  }
}
`

const invalidQuery = `
query InvalidField($id: String!) {
  employeeByID(id: $id) {
    salary
    secret
  }
}
`

type demoCase struct {
	name      string
	user      authz.User
	query     string
	variables map[string]any
}

func main() {
	ctx := context.Background()

	evaluator, err := authz.NewEvaluator(ctx, "policy/graphql_authz.rego")
	if err != nil {
		log.Fatalf("create evaluator: %v", err)
	}

	cases := []demoCase{
		{
			name:      "user_read_own_salary_by_variable",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     salaryQuery,
			variables: map[string]any{"id": "alice"},
		},
		{
			name:      "user_read_other_salary_only_field_removed_all",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     salaryQuery,
			variables: map[string]any{"id": "bob"},
		},
		{
			name:      "user_read_other_salary_keep_public_fields",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     employeeProfileAndSalaryQuery,
			variables: map[string]any{"id": "bob"},
		},
		{
			name:      "user_missing_variable_salary_removed",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     salaryQuery,
			variables: map[string]any{},
		},
		{
			name:      "user_read_by_literal_id_salary_removed",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     salaryByLiteralIDQuery,
			variables: map[string]any{},
		},
		{
			name:      "user_fragment_salary_removed_keep_id",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     salaryViaFragmentQuery,
			variables: map[string]any{"id": "bob"},
		},
		{
			name:      "user_public_fields_only_no_removal",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     publicOnlyQuery,
			variables: map[string]any{"id": "bob"},
		},
		{
			name:      "admin_read_other_salary_allowed",
			user:      authz.User{ID: "admin-1", Role: "admin"},
			query:     employeeProfileAndSalaryQuery,
			variables: map[string]any{"id": "bob"},
		},
		{
			name:      "invalid_graphql_query",
			user:      authz.User{ID: "alice", Role: "user"},
			query:     invalidQuery,
			variables: map[string]any{"id": "alice"},
		},
	}

	for _, tc := range cases {
		input := authz.Input{
			Input: authz.GraphQLRequest{
				Schema:    schema,
				Query:     tc.query,
				User:      tc.user,
				Variables: tc.variables,
			},
		}

		decision, err := evaluator.Evaluate(ctx, input)
		if err != nil {
			log.Fatalf("%s: evaluate policy: %v", tc.name, err)
		}

		fmt.Printf(
			"[%s] allow=%t query_valid=%t reasons=%v removed_fields=%v filtered_query=%q user=%+v variables=%v\n",
			tc.name,
			decision.Allow,
			decision.QueryValid,
			decision.Reasons,
			decision.RemovedFields,
			decision.FilteredQuery,
			tc.user,
			tc.variables,
		)
	}
}
