package extauthz

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"

	"authz-server/internal/jwt"
	"authz-server/internal/opa"
	"authz-server/internal/rewriter"
)

// Function pointer vars for dependency injection / testing.
var (
	jwtParseFromHeader = jwt.ParseFromHeader
	rewriteBody        = rewriter.RewriteBody
)

const (
	headerRewrittenBody = "x-rewritten-body"
	headerUserID        = "x-user-id"
)

// Server 实现 Envoy ext_authz gRPC v3 接口。
type Server struct {
	evaluator *opa.Evaluator
}

// NewServer 创建 ext_authz 服务实例。
func NewServer(evaluator *opa.Evaluator) *Server {
	return &Server{evaluator: evaluator}
}

// Check 实现 envoy.service.auth.v3.Authorization.Check。
func (s *Server) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	httpReq := req.GetAttributes().GetRequest().GetHttp()

	// 1. 解析 JWT
	authHeader := httpReq.GetHeaders()["authorization"]
	userInfo, err := jwtParseFromHeader(authHeader)
	if err != nil {
		log.Printf("jwt parse error: %v", err)
		return denied(codes.Unauthenticated, "invalid token"), nil
	}

	// 2. 解析 GraphQL body
	body := httpReq.GetBody()
	var gqlBody struct {
		Query string `json:"query"`
	}
	if body != "" {
		if err := json.Unmarshal([]byte(body), &gqlBody); err != nil {
			log.Printf("body parse error: %v", err)
			return denied(codes.InvalidArgument, "invalid request body"), nil
		}
	}

	// 3. 提取 query 中的字段名
	fields := extractTopFields(gqlBody.Query)

	// 3.5 提取 operation type
	opType := extractOperationType(gqlBody.Query)

	// 4. 调用 OPA 评估
	evalInput := opa.EvalInput{
		User: opa.UserInput{
			Authenticated: userInfo.Authenticated,
			Subject:       userInfo.Subject,
			Roles:         userInfo.Roles,
			CurrentTime:   time.Now().UTC().Format(time.RFC3339),
			Privileges:    userInfo.Privileges,
		},
		Request: opa.RequestInput{
			Query:         gqlBody.Query,
			Fields:        fields,
			OperationType: opType,
		},
	}

	decision, err := s.evaluator.Evaluate(ctx, evalInput)
	if err != nil {
		log.Printf("opa evaluate error: %v", err)
		return denied(codes.Internal, "policy evaluation failed"), nil
	}

	if !decision.Allow {
		reason := decision.Reason
		if reason == "" {
			reason = "request denied by policy"
		}
		return denied(codes.PermissionDenied, reason), nil
	}

	// 5. 如有 denied_fields，检查请求类型并改写 body
	if len(decision.DeniedFields) > 0 {
		method := strings.ToUpper(httpReq.GetMethod())
		if method == "GET" {
			return denied(codes.PermissionDenied, "GET requests not supported with field-level restrictions"), nil
		}
		contentType := httpReq.GetHeaders()["content-type"]
		if strings.HasPrefix(contentType, "multipart/") {
			return denied(codes.PermissionDenied, "multipart requests not supported with field-level restrictions"), nil
		}
		if body == "" || gqlBody.Query == "" {
			return denied(codes.PermissionDenied, "cannot enforce field restrictions without query body"), nil
		}
		rewrittenBody, err := rewriteBody([]byte(body), decision.DeniedFields)
		if err != nil {
			log.Printf("rewrite error: %v", err)
			return denied(codes.PermissionDenied, "query rewrite failed: all requested fields are denied"), nil
		}
		log.Printf("rewritten body: %s", string(rewrittenBody))
		return allowedWithRewrittenBodyAndUserID(string(rewrittenBody), userInfo.Subject), nil
	}

	return allowedWithUserID(userInfo.Subject), nil
}

func allowed() *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &status.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	}
}

func allowedWithRewrittenBody(body string) *authv3.CheckResponse {
	return allowedWithRewrittenBodyAndUserID(body, "")
}

func allowedWithUserID(userID string) *authv3.CheckResponse {
	headers := make([]*corev3.HeaderValueOption, 0, 1)
	if userID != "" {
		headers = append(headers, headerOption(headerUserID, userID))
	}
	return &authv3.CheckResponse{
		Status: &status.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: headers,
			},
		},
	}
}

func allowedWithRewrittenBodyAndUserID(body, userID string) *authv3.CheckResponse {
	headers := make([]*corev3.HeaderValueOption, 0, 2)
	if userID != "" {
		headers = append(headers, headerOption(headerUserID, userID))
	}
	headers = append(headers, headerOption(headerRewrittenBody, body))
	return &authv3.CheckResponse{
		Status: &status.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: headers,
			},
		},
	}
}

func headerOption(key, value string) *corev3.HeaderValueOption {
	return &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:   key,
			Value: value,
		},
	}
}

func denied(code codes.Code, msg string) *authv3.CheckResponse {
	httpCode := typev3.StatusCode_Forbidden
	if code == codes.Unauthenticated {
		httpCode = typev3.StatusCode_Unauthorized
	}
	return &authv3.CheckResponse{
		Status: &status.Status{Code: int32(code), Message: msg},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: httpCode},
				Body:   `{"error":"` + msg + `"}`,
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   "content-type",
							Value: "application/json",
						},
					},
				},
			},
		},
	}
}

// extractOperationType 使用 gqlparser 解析 GraphQL query 的 operation type。
// 返回 "query"、"mutation"、"subscription" 或空字符串。
func extractOperationType(query string) string {
	if query == "" {
		return ""
	}
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return ""
	}
	if len(doc.Operations) == 0 {
		return ""
	}
	switch doc.Operations[0].Operation {
	case ast.Query:
		return "query"
	case ast.Mutation:
		return "mutation"
	case ast.Subscription:
		return "subscription"
	default:
		return ""
	}
}

// extractTopFields 简易提取 GraphQL query 中的字段名列表。
func extractTopFields(query string) []string {
	var fields []string
	// 简单解析：找 { } 中的字段名
	inBraces := 0
	for _, line := range strings.Split(query, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "{") {
			inBraces++
		}
		if inBraces >= 2 {
			// 提取字段名
			field := strings.TrimSpace(strings.Split(trimmed, "{")[0])
			field = strings.TrimSpace(strings.Split(field, "(")[0])
			field = strings.TrimSpace(strings.Split(field, "}")[0])
			if field != "" && !strings.HasPrefix(field, "#") && !strings.HasPrefix(field, "...") {
				fields = append(fields, field)
			}
		}
		if strings.Contains(trimmed, "}") {
			inBraces--
		}
	}
	return fields
}
