package graph_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"

	"demo-opa-graphql/graph"
	"demo-opa-graphql/graph/generated"
	"demo-opa-graphql/internal/security"
)

type graphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func TestGraphQLAuthFlow(t *testing.T) {
	secret := []byte("integration-secret")
	handler := newTestHandler(t, secret)

	t.Run("anonymous publicInfo is allowed", func(t *testing.T) {
		resp := doGraphQL(t, handler, `{ publicInfo }`, "")
		if len(resp.Errors) > 0 {
			t.Fatalf("unexpected errors: %+v", resp.Errors)
		}

		var value string
		if err := json.Unmarshal(resp.Data["publicInfo"], &value); err != nil {
			t.Fatalf("unmarshal publicInfo: %v", err)
		}
		if value == "" {
			t.Fatalf("publicInfo should not be empty")
		}
	})

	t.Run("anonymous me is unauthenticated", func(t *testing.T) {
		resp := doGraphQL(t, handler, `{ me { id name roles } }`, "")
		if len(resp.Errors) == 0 {
			t.Fatalf("expected errors")
		}
		if !strings.Contains(resp.Errors[0].Message, "unauthenticated") {
			t.Fatalf("error = %q, want contains %q", resp.Errors[0].Message, "unauthenticated")
		}
	})

	t.Run("mixed query keeps allowed fields and nulls unauthorized field", func(t *testing.T) {
		resp := doGraphQL(t, handler, `{ publicInfo me { id name roles } }`, "")
		if len(resp.Errors) == 0 {
			t.Fatalf("expected errors")
		}

		foundUnauthenticated := false
		for _, err := range resp.Errors {
			if strings.Contains(err.Message, "unauthenticated") {
				foundUnauthenticated = true
				break
			}
		}
		if !foundUnauthenticated {
			t.Fatalf("errors = %+v, want contains %q", resp.Errors, "unauthenticated")
		}

		var publicInfo string
		if err := json.Unmarshal(resp.Data["publicInfo"], &publicInfo); err != nil {
			t.Fatalf("unmarshal publicInfo: %v", err)
		}
		if publicInfo == "" {
			t.Fatalf("publicInfo should not be empty")
		}

		var me interface{}
		if err := json.Unmarshal(resp.Data["me"], &me); err != nil {
			t.Fatalf("unmarshal me: %v", err)
		}
		if me != nil {
			t.Fatalf("me = %#v, want nil", me)
		}
	})

	t.Run("user cannot createPost", func(t *testing.T) {
		token, err := security.IssueDemoToken(secret, "user-1", []string{"user"}, time.Hour)
		if err != nil {
			t.Fatalf("IssueDemoToken user: %v", err)
		}

		resp := doGraphQL(t, handler, `mutation { createPost(title: "hello") { id title authorID } }`, token)
		if len(resp.Errors) == 0 {
			t.Fatalf("expected errors")
		}
		if !strings.Contains(resp.Errors[0].Message, "insufficient role") {
			t.Fatalf("error = %q, want contains %q", resp.Errors[0].Message, "insufficient role")
		}
	})

	t.Run("admin can createPost", func(t *testing.T) {
		token, err := security.IssueDemoToken(secret, "admin-1", []string{"admin"}, time.Hour)
		if err != nil {
			t.Fatalf("IssueDemoToken admin: %v", err)
		}

		resp := doGraphQL(t, handler, `mutation { createPost(title: "hello") { id title authorID } }`, token)
		if len(resp.Errors) > 0 {
			t.Fatalf("unexpected errors: %+v", resp.Errors)
		}

		var post struct {
			AuthorID string `json:"authorID"`
		}
		if err := json.Unmarshal(resp.Data["createPost"], &post); err != nil {
			t.Fatalf("unmarshal createPost: %v", err)
		}
		if post.AuthorID != "admin-1" {
			t.Fatalf("authorID = %q, want %q", post.AuthorID, "admin-1")
		}
	})
}

func newTestHandler(t *testing.T, secret []byte) http.Handler {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	policyPath := filepath.Join(filepath.Dir(thisFile), "..", "policy", "authz.rego")

	authorizer, err := security.NewAuthorizer(policyPath)
	if err != nil {
		t.Fatalf("NewAuthorizer() error = %v", err)
	}

	resolver := graph.NewResolver(authorizer)
	graphqlServer := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	mux := http.NewServeMux()
	mux.Handle("/query", security.JWTMiddleware(secret)(graphqlServer))

	return mux
}

func doGraphQL(t *testing.T, handler http.Handler, query string, token string) graphQLResponse {
	t.Helper()

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "http://example.local/query", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	var output graphQLResponse
	if err := json.NewDecoder(recorder.Body).Decode(&output); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return output
}
