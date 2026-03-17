package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/open-policy-agent/opa/rego"
)

type PolicyUser struct {
	Authenticated bool     `json:"authenticated"`
	Subject       string   `json:"subject,omitempty"`
	Roles         []string `json:"roles,omitempty"`
}

type PolicyInput struct {
	User      PolicyUser             `json:"user"`
	Operation string                 `json:"operation"`
	Field     string                 `json:"field"`
	Args      map[string]interface{} `json:"args,omitempty"`
}

type Decision struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason"`
}

type Authorizer struct {
	query rego.PreparedEvalQuery
}

func NewAuthorizer(policyPath string) (*Authorizer, error) {
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}

	prepared, err := rego.New(
		rego.Query("data.graphql.authz.decision"),
		rego.Module("policy/authz.rego", string(policyBytes)),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("prepare policy: %w", err)
	}

	return &Authorizer{query: prepared}, nil
}

func (a *Authorizer) Evaluate(ctx context.Context, input PolicyInput) (Decision, error) {
	results, err := a.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return Decision{}, fmt.Errorf("evaluate policy: %w", err)
	}
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return Decision{}, fmt.Errorf("evaluate policy: empty result")
	}

	raw := results[0].Expressions[0].Value
	payload, err := json.Marshal(raw)
	if err != nil {
		return Decision{}, fmt.Errorf("marshal policy result: %w", err)
	}

	var decision Decision
	if err := json.Unmarshal(payload, &decision); err != nil {
		return Decision{}, fmt.Errorf("unmarshal policy result: %w", err)
	}
	if decision.Reason == "" {
		decision.Reason = "forbidden"
	}

	return decision, nil
}
