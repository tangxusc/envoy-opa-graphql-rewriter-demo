package extauthz

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc/codes"

	"authz-server/internal/jwt"
	"authz-server/internal/opa"
	"authz-server/internal/privilege"
)

// mustEncodePrivileges is a test helper that encodes roles into a privileges string.
func mustEncodePrivileges(t *testing.T, roles []string) string {
	t.Helper()
	privStr, err := privilege.Encode(roles)
	if err != nil {
		t.Fatalf("privilege.Encode: %v", err)
	}
	return privStr
}

// makeCheckRequest builds a minimal Envoy CheckRequest for testing.
func makeCheckRequest(authHeader, body string) *authv3.CheckRequest {
	headers := map[string]string{}
	if authHeader != "" {
		headers["authorization"] = authHeader
	}
	return &authv3.CheckRequest{
		Attributes: &authv3.AttributeContext{
			Request: &authv3.AttributeContext_Request{
				Http: &authv3.AttributeContext_HttpRequest{
					Headers: headers,
					Body:    body,
				},
			},
		},
	}
}

// fakeEvaluator creates a real OPA evaluator with a test policy for integration-style tests.
func fakeEvaluator(t *testing.T) *opa.Evaluator {
	t.Helper()
	eval, err := opa.NewEvaluator(context.Background(), "../opa/testdata/test_policy.rego")
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return eval
}

func findHeader(headers []*corev3.HeaderValueOption, key string) (string, bool) {
	for _, h := range headers {
		hv := h.GetHeader()
		if hv.GetKey() == key {
			return hv.GetValue(), true
		}
	}
	return "", false
}

func TestNewServer(t *testing.T) {
	t.Parallel()
	eval := fakeEvaluator(t)
	s := NewServer(eval)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestCheck_Allowed(t *testing.T) {

	// Mock jwt parse to return an authenticated admin user
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{
			Subject:       "alice",
			Roles:         []string{"admin"},
			Privileges:    mustEncodePrivileges(t, []string{"admin"}),
			Authenticated: true,
		}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	body, _ := json.Marshal(map[string]string{"query": "{ users { name } }"})
	req := makeCheckRequest("Bearer fake-token", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Fatalf("expected %s header", headerUserID)
	}
	if userID != "alice" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "alice")
	}
}

func TestCheck_Denied_Unauthenticated(t *testing.T) {
	// Mock jwt parse to return unauthenticated user
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{Authenticated: false}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	body, _ := json.Marshal(map[string]string{"query": "{ users { name } }"})
	req := makeCheckRequest("", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() == int32(codes.OK) {
		t.Error("expected non-OK status for unauthenticated user")
	}
	for _, h := range resp.GetDeniedResponse().GetHeaders() {
		if h.GetHeader().GetKey() == headerUserID {
			t.Fatalf("did not expect denied response to include %s", headerUserID)
		}
	}
}

func TestCheck_MissingAuth_JWTError(t *testing.T) {
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return nil, fmt.Errorf("invalid token")
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	req := makeCheckRequest("Bearer bad", "")
	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.Unauthenticated) {
		t.Errorf("status code = %d, want Unauthenticated(%d)", resp.GetStatus().GetCode(), codes.Unauthenticated)
	}
}

func TestCheck_InvalidBody(t *testing.T) {
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{Authenticated: true, Subject: "alice", Roles: []string{"admin"}, Privileges: mustEncodePrivileges(t, []string{"admin"})}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	req := makeCheckRequest("Bearer fake", "not-json")
	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.InvalidArgument) {
		t.Errorf("status code = %d, want InvalidArgument(%d)", resp.GetStatus().GetCode(), codes.InvalidArgument)
	}
}

func TestCheck_RewrittenBody(t *testing.T) {
	origJWT := jwtParseFromHeader
	origRewrite := rewriteBody
	defer func() {
		jwtParseFromHeader = origJWT
		rewriteBody = origRewrite
	}()

	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{
			Subject:       "alice",
			Roles:         []string{"user"}, // non-admin -> salary denied
			Privileges:    mustEncodePrivileges(t, []string{"user"}),
			Authenticated: true,
		}, nil
	}

	rewriteBody = func(body []byte, deniedFields []string) ([]byte, error) {
		return []byte(`{"query":"{ users { name } }"}`), nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	multilineQuery := "{\n  users {\n    name\n    salary\n  }\n}"
	body, _ := json.Marshal(map[string]string{"query": multilineQuery})
	req := makeCheckRequest("Bearer fake", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}

	// Check that rewritten body header is present
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	rewritten, found := findHeader(okResp.GetHeaders(), headerRewrittenBody)
	if !found {
		t.Error("expected x-rewritten-body header")
	}
	if rewritten == "" {
		t.Error("expected x-rewritten-body to be non-empty")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Errorf("expected %s header", headerUserID)
	}
	if userID != "alice" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "alice")
	}
}

func TestCheck_RewriteError(t *testing.T) {
	origJWT := jwtParseFromHeader
	origRewrite := rewriteBody
	defer func() {
		jwtParseFromHeader = origJWT
		rewriteBody = origRewrite
	}()

	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{
			Subject:       "alice",
			Roles:         []string{"user"},
			Privileges:    mustEncodePrivileges(t, []string{"user"}),
			Authenticated: true,
		}, nil
	}

	rewriteBody = func(body []byte, deniedFields []string) ([]byte, error) {
		return nil, fmt.Errorf("rewrite failed")
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	multilineQuery := "{\n  users {\n    name\n    salary\n  }\n}"
	body, _ := json.Marshal(map[string]string{"query": multilineQuery})
	req := makeCheckRequest("Bearer fake", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.Internal) {
		t.Errorf("status code = %d, want Internal(%d)", resp.GetStatus().GetCode(), codes.Internal)
	}
}

func TestExtractTopFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		query  string
		expect []string
	}{
		{
			name:   "simple query",
			query:  "{\n  users {\n    name\n    salary\n  }\n}",
			expect: []string{"users", "name", "salary"},
		},
		{
			name:   "empty query",
			query:  "",
			expect: nil,
		},
		{
			name:   "no nested braces",
			query:  "{ users }",
			expect: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTopFields(tc.query)
			if len(got) != len(tc.expect) {
				t.Fatalf("extractTopFields = %v, want %v", got, tc.expect)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Errorf("field[%d] = %q, want %q", i, got[i], tc.expect[i])
				}
			}
		})
	}
}

func TestDenied_CodeMapping(t *testing.T) {
	t.Parallel()
	resp := denied(codes.Unauthenticated, "test")
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse")
	}
	// Unauthenticated should map to 401
	if deniedResp.GetStatus().GetCode() != 401 {
		t.Errorf("HTTP code = %d, want 401", deniedResp.GetStatus().GetCode())
	}

	resp2 := denied(codes.PermissionDenied, "test")
	deniedResp2 := resp2.GetDeniedResponse()
	// PermissionDenied should map to 403
	if deniedResp2.GetStatus().GetCode() != 403 {
		t.Errorf("HTTP code = %d, want 403", deniedResp2.GetStatus().GetCode())
	}
	for _, h := range deniedResp2.GetHeaders() {
		if h.GetHeader().GetKey() == headerUserID {
			t.Fatalf("did not expect denied response to include %s", headerUserID)
		}
	}
}

func TestAllowed(t *testing.T) {
	t.Parallel()
	resp := allowed()
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status = %d, want OK", resp.GetStatus().GetCode())
	}
}

func TestAllowedWithUserID(t *testing.T) {
	t.Parallel()
	resp := allowedWithUserID("user-1")
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Fatalf("expected %s header", headerUserID)
	}
	if userID != "user-1" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "user-1")
	}
}

func TestAllowedWithRewrittenBody(t *testing.T) {
	t.Parallel()
	resp := allowedWithRewrittenBody(`{"query":"{ name }"}`)
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	rewritten, found := findHeader(okResp.GetHeaders(), headerRewrittenBody)
	if !found {
		t.Error("expected x-rewritten-body header")
	}
	if rewritten != `{"query":"{ name }"}` {
		t.Errorf("header value = %q", rewritten)
	}
	if _, hasUserID := findHeader(okResp.GetHeaders(), headerUserID); hasUserID {
		t.Fatalf("did not expect %s header", headerUserID)
	}
}

// Verify that Check properly handles the HTTP code mapping for the denied response
func TestCheck_EmptyBody(t *testing.T) {
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{Authenticated: true, Subject: "alice", Roles: []string{"admin"}, Privileges: mustEncodePrivileges(t, []string{"admin"})}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	req := makeCheckRequest("Bearer fake", "")
	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Fatalf("expected %s header", headerUserID)
	}
	if userID != "alice" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "alice")
	}
}

// Verify that request without attributes doesn't panic
func TestCheck_NilAttributes(t *testing.T) {
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{Authenticated: true, Subject: "alice", Roles: []string{"admin"}, Privileges: mustEncodePrivileges(t, []string{"admin"})}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	req := &authv3.CheckRequest{}
	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// Empty request with authenticated user should be OK
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Fatalf("expected %s header", headerUserID)
	}
	if userID != "alice" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "alice")
	}
}

func TestExtractOperationType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		query  string
		expect string
	}{
		{name: "query implicit", query: "{ users { name } }", expect: "query"},
		{name: "query explicit", query: "query { users { name } }", expect: "query"},
		{name: "mutation", query: `mutation { updateEmployee(id: "emp-1", name: "Bob") { id name } }`, expect: "mutation"},
		{name: "subscription", query: "subscription { employeeUpdated { id name salary } }", expect: "subscription"},
		{name: "empty", query: "", expect: ""},
		{name: "invalid", query: "{{{ bad", expect: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractOperationType(tc.query)
			if got != tc.expect {
				t.Errorf("extractOperationType(%q) = %q, want %q", tc.query, got, tc.expect)
			}
		})
	}
}

func TestCheck_SubscriptionAllowed(t *testing.T) {
	origJWT := jwtParseFromHeader
	defer func() { jwtParseFromHeader = origJWT }()
	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{
			Subject:       "alice",
			Roles:         []string{"admin"},
			Privileges:    mustEncodePrivileges(t, []string{"admin"}),
			Authenticated: true,
		}, nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	body, _ := json.Marshal(map[string]string{"query": "subscription { employeeUpdated { id name salary } }"})
	req := makeCheckRequest("Bearer fake-token", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}
	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Fatalf("expected %s header", headerUserID)
	}
	if userID != "alice" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "alice")
	}
}

func TestCheck_SubscriptionRewritten(t *testing.T) {
	origJWT := jwtParseFromHeader
	origRewrite := rewriteBody
	defer func() {
		jwtParseFromHeader = origJWT
		rewriteBody = origRewrite
	}()

	jwtParseFromHeader = func(header string) (*jwt.UserInfo, error) {
		return &jwt.UserInfo{
			Subject:       "bob",
			Roles:         []string{"user"},
			Privileges:    mustEncodePrivileges(t, []string{"user"}),
			Authenticated: true,
		}, nil
	}

	rewriteBody = func(body []byte, deniedFields []string) ([]byte, error) {
		return []byte(`{"query":"subscription { employeeUpdated { id name } }"}`), nil
	}

	eval := fakeEvaluator(t)
	s := NewServer(eval)

	multilineQuery := "subscription {\n  employeeUpdated {\n    id\n    name\n    salary\n  }\n}"
	body, _ := json.Marshal(map[string]string{"query": multilineQuery})
	req := makeCheckRequest("Bearer fake", string(body))

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus().GetCode() != int32(codes.OK) {
		t.Errorf("status code = %d, want OK", resp.GetStatus().GetCode())
	}

	okResp := resp.GetOkResponse()
	if okResp == nil {
		t.Fatal("expected OkResponse")
	}
	if _, found := findHeader(okResp.GetHeaders(), headerRewrittenBody); !found {
		t.Error("expected x-rewritten-body header for subscription rewrite")
	}
	userID, found := findHeader(okResp.GetHeaders(), headerUserID)
	if !found {
		t.Errorf("expected %s header", headerUserID)
	}
	if userID != "bob" {
		t.Errorf("%s = %q, want %q", headerUserID, userID, "bob")
	}
}
