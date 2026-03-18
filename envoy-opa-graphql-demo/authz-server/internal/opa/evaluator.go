package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/types"

	"authz-server/internal/privilege"
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
	CurrentTime   string   `json:"current_time"` // ISO 8601 格式当前时间
	Privileges    string   `json:"privileges"`   // base64 编码的 Bloom Filter
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
		// 注册自定义内置函数 hasPrivilege(privileges_string, privilege_name)
		rego.Function2(
			&rego.Function{
				Name: "hasPrivilege",
				Decl: types.NewFunction(types.Args(types.S, types.S), types.B),
			},
			func(_ rego.BuiltinContext, a, b *ast.Term) (*ast.Term, error) {
				privStr, ok1 := a.Value.(ast.String)
				privName, ok2 := b.Value.(ast.String)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("hasPrivilege: expected two string arguments")
				}
				has, err := privilege.HasPrivilege(string(privStr), string(privName))
				if err != nil {
					return nil, err
				}
				return ast.BooleanTerm(has), nil
			},
		),
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
