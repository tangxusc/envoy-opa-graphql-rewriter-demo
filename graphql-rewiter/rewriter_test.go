package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestRewriteQuery_Scenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		decision string
		want     string
	}{
		{
			name: "remove fields from nested fragment and deep object",
			query: `query Q($id: String!) {
  employeeByID(id: $id) {
    id
    profile {
      title
      ...ProfileSecrets
      contact {
        email
        phone
      }
    }
  }
}

fragment ProfileSecrets on Profile {
  salary
  bankAccount
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "employeeByID.profile.salary",
    "employeeByID.profile.bankAccount",
    "employeeByID.profile.contact.phone"
  ]
}`,
			want: `query Q($id: String!) {
  employeeByID(id: $id) {
    id
    profile {
      title
      contact {
        email
      }
    }
  }
}
`,
		},
		{
			name: "inline fragment alias and directives",
			query: `query Q {
  emp: employeeByID(id: "1") {
    id
    ... on Employee @include(if: true) {
      salary
      manager {
        name
        bonus
      }
    }
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary",
    "employeeByID.manager.bonus"
  ]
}`,
			want: `query Q {
  emp: employeeByID(id: "1") {
    id
    ... on Employee @include(if: true) {
      manager {
        name
      }
    }
  }
}
`,
		},
		{
			name: "remove parent field directly",
			query: `query Q {
  employeeByID(id: "1") {
    id
    salary
  }
  departmentByID(id: "2") {
    id
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "employeeByID"
  ]
}`,
			want: `query Q {
  departmentByID(id: "2") {
    id
  }
}
`,
		},
		{
			name: "drop empty field chain recursively",
			query: `query Q {
  company {
    org {
      employee {
        salary
      }
    }
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "company.org.employee.salary"
  ]
}`,
			want: `query Q
`,
		},
		{
			name: "multiple operations",
			query: `query A {
  employeeByID(id: "1") {
    id
    salary
  }
}

query B {
  employeeByID(id: "2") {
    salary
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary"
  ]
}`,
			want: `query A {
  employeeByID(id: "1") {
    id
  }
}
query B
`,
		},
		{
			name: "nested fragment spreads",
			query: `query Q {
  employeeByID(id: "1") {
    ...F1
  }
}

fragment F1 on Employee {
  id
  ...F2
}

fragment F2 on Employee {
  salary
  address {
    city
    zipcode
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary",
    "employeeByID.address.zipcode"
  ]
}`,
			want: `query Q {
  employeeByID(id: "1") {
    id
    address {
      city
    }
  }
}
`,
		},
		{
			name: "normalize removed field path whitespace",
			query: `query Q {
  employeeByID(id: "1") {
    id
    salary
  }
}
`,
			decision: `{
  "allow": true,
  "removed_fields": [
    "  employeeByID . salary  ",
    "",
    "   "
  ]
}`,
			want: `query Q {
  employeeByID(id: "1") {
    id
  }
}
`,
		},
		{
			name: "no removed fields still inlines fragments",
			query: `query Q {
  employeeByID(id: "1") {
    ...SalaryPart
  }
}

fragment SalaryPart on Employee {
  id
  salary
}
`,
			decision: `{"allow":true,"removed_fields":[]}`,
			want: `query Q {
  employeeByID(id: "1") {
    id
    salary
  }
}
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertRewriteResult(t, tt.query, tt.decision, tt.want)
		})
	}
}

func TestRewriteQuery_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		query        string
		decisionJSON string
		wantErr      string
	}{
		{
			name:         "deny by policy",
			query:        `query Q { ping }`,
			decisionJSON: `{"allow":false,"removed_fields":[]}`,
			wantErr:      "request denied by policy",
		},
		{
			name:         "invalid decision json",
			query:        `query Q { ping }`,
			decisionJSON: `{invalid-json}`,
			wantErr:      "unmarshal decision",
		},
		{
			name:         "invalid graphql",
			query:        `query Q { ping `,
			decisionJSON: `{"allow":true,"removed_fields":[]}`,
			wantErr:      "parse graphql query",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := RewriteQuery(tt.query, []byte(tt.decisionJSON))
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error mismatch, want contains %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func assertRewriteResult(t *testing.T, query string, decision string, want string) {
	t.Helper()

	got, err := RewriteQuery(query, []byte(decision))
	if err != nil {
		t.Fatalf("RewriteQuery returned error: %v", err)
	}

	if normalizeWhitespace(got) != normalizeWhitespace(want) {
		t.Fatalf("unexpected rewrite result\nwant:\n%s\ngot:\n%s", want, got)
	}
}

var wsRE = regexp.MustCompile(`\s+`)

func normalizeWhitespace(s string) string {
	return wsRE.ReplaceAllString(s, "")
}
