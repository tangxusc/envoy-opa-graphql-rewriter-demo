package security

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTMiddleware(t *testing.T) {
	secret := []byte("test-secret")

	t.Run("valid token injects principal", func(t *testing.T) {
		token, err := IssueDemoToken(secret, "user-1", []string{"user"}, time.Hour)
		if err != nil {
			t.Fatalf("IssueDemoToken() error = %v", err)
		}

		h := JWTMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				t.Fatalf("principal missing from context")
			}
			_, _ = w.Write([]byte(fmt.Sprintf("%s:%s", principal.Subject, principal.Roles[0])))
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
		}
		if got := resp.Body.String(); got != "user-1:user" {
			t.Fatalf("body = %q, want %q", got, "user-1:user")
		}
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		h := JWTMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			t.Fatalf("expected non-empty error body")
		}
	})
}
