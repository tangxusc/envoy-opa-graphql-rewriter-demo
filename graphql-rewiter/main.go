package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const sampleQuery = `query EmployeeSalaryWithFragment($id: String!) {
  employeeByID(id: $id) {
    id
    ...SalaryPart
  }
}

fragment SalaryPart on Employee {
  salary
}
`

const sampleDecision = `{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary"
  ]
}`

var serverRunner = runServer

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("graphql-rewiter", flag.ContinueOnError)
	fs.SetOutput(stderr)

	queryPath := fs.String("query", "", "Path to GraphQL query file")
	decisionPath := fs.String("decision", "", "Path to policy decision JSON file")
	serve := fs.Bool("serve", false, "Run HTTP server")
	addr := fs.String("addr", ":8080", "HTTP listen address")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "flag error: %v\n", err)
		return 1
	}

	// Start web mode by default when no CLI file input is provided.
	if *serve || (*queryPath == "" && *decisionPath == "") {
		if err := serverRunner(*addr); err != nil {
			fmt.Fprintf(stderr, "server error: %v\n", err)
			return 1
		}
		return 0
	}

	query, decision, err := loadInputs(*queryPath, *decisionPath)
	if err != nil {
		fmt.Fprintf(stderr, "input error: %v\n", err)
		return 1
	}

	out, err := RewriteQuery(query, []byte(decision))
	if err != nil {
		fmt.Fprintf(stderr, "rewrite error: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintln(stdout, out)
	return 0
}

func loadInputs(queryPath, decisionPath string) (string, string, error) {
	if queryPath == "" && decisionPath == "" {
		return sampleQuery, sampleDecision, nil
	}

	if queryPath == "" || decisionPath == "" {
		return "", "", fmt.Errorf("both -query and -decision must be provided together")
	}

	queryBytes, err := os.ReadFile(queryPath)
	if err != nil {
		return "", "", fmt.Errorf("read query file: %w", err)
	}

	decisionBytes, err := os.ReadFile(decisionPath)
	if err != nil {
		return "", "", fmt.Errorf("read decision file: %w", err)
	}

	return string(queryBytes), string(decisionBytes), nil
}
