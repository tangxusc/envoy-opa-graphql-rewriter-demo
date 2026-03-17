package extauthz

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"

	"authz-server/internal/jwt"
	"authz-server/internal/opa"
	"authz-server/internal/rewriter"
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
	userInfo, err := jwt.ParseFromHeader(authHeader)
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

	// 4. 调用 OPA 评估
	evalInput := opa.EvalInput{
		User: opa.UserInput{
			Authenticated: userInfo.Authenticated,
			Subject:       userInfo.Subject,
			Roles:         userInfo.Roles,
		},
		Request: opa.RequestInput{
			Query:  gqlBody.Query,
			Fields: fields,
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

	// 5. 如有 denied_fields，改写 body
	if len(decision.DeniedFields) > 0 && body != "" {
		rewrittenBody, err := rewriter.RewriteBody([]byte(body), decision.DeniedFields)
		if err != nil {
			log.Printf("rewrite error: %v", err)
			return denied(codes.Internal, "query rewrite failed"), nil
		}
		log.Printf("rewritten body: %s", string(rewrittenBody))
		return allowedWithRewrittenBody(string(rewrittenBody)), nil
	}

	return allowed(), nil
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
	return &authv3.CheckResponse{
		Status: &status.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   "x-rewritten-body",
							Value: body,
						},
					},
				},
			},
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
