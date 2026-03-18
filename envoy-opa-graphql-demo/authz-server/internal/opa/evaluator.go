package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/open-policy-agent/opa/v1/rego"
)

const decisionQuery = "data.graphqlapi.authz.decision"

// Decision 表示 OPA 策略评估的结果。
type Decision struct {
	Allow        bool     `json:"allow"`
	DeniedFields []string `json:"denied_fields"`
	Reason       string   `json:"reason"`
}

// UserInput 传递给 OPA 策略的用户信息。
type UserInput struct {
	Authenticated bool     `json:"authenticated"`
	Subject       string   `json:"subject"`
	Roles         []string `json:"roles"`
}

// RequestInput 传递给 OPA 策略的请求信息。
type RequestInput struct {
	Query         string   `json:"query"`
	Fields        []string `json:"fields"`
	OperationType string   `json:"operation_type"`
}

// EvalInput 是 OPA 评估的顶层输入。
type EvalInput struct {
	User    UserInput    `json:"user"`
	Request RequestInput `json:"request"`
}

// Evaluator 封装 OPA 策略的编译与评估。
type Evaluator struct {
	prepared rego.PreparedEvalQuery
}

// NewEvaluator 从 rego 文件创建评估器。
func NewEvaluator(ctx context.Context, policyPath string) (*Evaluator, error) {
	module, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("read policy file %q: %w", policyPath, err)
	}

	prepared, err := rego.New(
		rego.Query(decisionQuery),
		rego.Module(policyPath, string(module)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare decision query: %w", err)
	}

	return &Evaluator{prepared: prepared}, nil
}

// Evaluate 对输入执行 OPA 策略评估，返回 Decision。
func (e *Evaluator) Evaluate(ctx context.Context, input EvalInput) (*Decision, error) {
	results, err := e.prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("evaluate decision: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("decision query returned no result")
	}

	raw := results[0].Expressions[0].Value
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal decision: %w", err)
	}

	var decision Decision
	if err := json.Unmarshal(bytes, &decision); err != nil {
		return nil, fmt.Errorf("unmarshal decision: %w", err)
	}

	return &decision, nil
}
