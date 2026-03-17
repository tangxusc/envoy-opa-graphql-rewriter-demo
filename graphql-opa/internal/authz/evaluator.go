package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
)

const (
	decisionQuery      = "data.graphqlapi.authz.decision"
	canReadSalaryQuery = "data.graphqlapi.authz.can_read_salary"
)

type User struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type GraphQLRequest struct {
	Schema    string         `json:"schema"`
	Query     string         `json:"query"`
	User      User           `json:"user"`
	Variables map[string]any `json:"variables"`
}

type Input struct {
	Input GraphQLRequest `json:"input"`
}

type Decision struct {
	Allow         bool     `json:"allow"`
	QueryValid    bool     `json:"query_valid"`
	Reasons       []string `json:"reasons"`
	FilteredQuery string   `json:"filtered_query,omitempty"`
	RemovedFields []string `json:"removed_fields,omitempty"`
}

type Evaluator struct {
	decisionPrepared      rego.PreparedEvalQuery
	canReadSalaryPrepared rego.PreparedEvalQuery
}

func NewEvaluator(ctx context.Context, policyPath string) (*Evaluator, error) {
	module, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("read policy file %q: %w", policyPath, err)
	}

	decisionPrepared, err := rego.New(
		rego.Query(decisionQuery),
		rego.Module(policyPath, string(module)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare decision query: %w", err)
	}

	canReadSalaryPrepared, err := rego.New(
		rego.Query(canReadSalaryQuery),
		rego.Module(policyPath, string(module)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare salary query: %w", err)
	}
	//不为空
	if canReadSalaryPrepared != (rego.PreparedEvalQuery{}) {
		fmt.Println("canReadSalaryPrepared is not nil")
	}

	return &Evaluator{
		decisionPrepared:      decisionPrepared,
		canReadSalaryPrepared: canReadSalaryPrepared,
	}, nil
}

func (e *Evaluator) Evaluate(ctx context.Context, input Input) (Decision, error) {
	decision, err := e.evaluateDecision(ctx, input)
	if err != nil {
		return Decision{}, err
	}

	if !decision.QueryValid {
		decision.Allow = false
		decision.FilteredQuery = ""
		decision.RemovedFields = []string{}
		decision.Reasons = uniqueSortedStrings(decision.Reasons)
		return decision, nil
	}

	filteredQuery, removedFields, hasAllowedFields, err := filterGraphQLQuery(
		ctx,
		input.Input.Query,
		input.Input.User,
		input.Input.Variables,
		e.canReadSalary,
	)
	if err != nil {
		return Decision{}, fmt.Errorf("filter graphql query: %w", err)
	}

	decision.FilteredQuery = filteredQuery
	decision.RemovedFields = removedFields
	decision.Allow = hasAllowedFields

	if len(removedFields) > 0 {
		decision.Reasons = append(
			decision.Reasons,
			fmt.Sprintf("removed unauthorized fields: %s", strings.Join(removedFields, ", ")),
		)
	}
	if !hasAllowedFields {
		decision.Reasons = append(decision.Reasons, "no fields left after authorization filtering")
	}

	decision.Reasons = uniqueSortedStrings(decision.Reasons)
	return decision, nil
}

func (e *Evaluator) evaluateDecision(ctx context.Context, input Input) (Decision, error) {
	results, err := e.decisionPrepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return Decision{}, fmt.Errorf("evaluate decision query: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return Decision{}, fmt.Errorf("decision query returned no result")
	}

	return decodeDecision(results[0].Expressions[0].Value)
}

func (e *Evaluator) canReadSalary(ctx context.Context, user User, targetID string) (bool, error) {
	checkInput := map[string]any{
		"user": map[string]any{
			"id":   user.ID,
			"role": user.Role,
		},
		"target_id": targetID,
	}

	results, err := e.canReadSalaryPrepared.Eval(ctx, rego.EvalInput(checkInput))
	if err != nil {
		return false, fmt.Errorf("evaluate salary query: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return false, nil
	}

	allowed, ok := results[0].Expressions[0].Value.(bool)
	if !ok {
		return false, fmt.Errorf("salary query returned non-bool result")
	}

	return allowed, nil
}

func decodeDecision(raw any) (Decision, error) {
	bytes, err := json.Marshal(raw)
	if err != nil {
		return Decision{}, fmt.Errorf("marshal decision: %w", err)
	}

	var decision Decision
	if err := json.Unmarshal(bytes, &decision); err != nil {
		return Decision{}, fmt.Errorf("unmarshal decision: %w", err)
	}

	if decision.Reasons == nil {
		decision.Reasons = []string{}
	}
	if decision.RemovedFields == nil {
		decision.RemovedFields = []string{}
	}

	return decision, nil
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	dedup := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		dedup[value] = struct{}{}
	}

	out := make([]string, 0, len(dedup))
	for value := range dedup {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
