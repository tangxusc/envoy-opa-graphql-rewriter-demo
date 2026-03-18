package jwt

import (
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func signTestToken(t *testing.T, claims *Claims) string {
	t.Helper()
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte("demo-secret"))
	if err != nil {
		t.Fatalf("signTestToken: %v", err)
	}
	return s
}

func TestParseFromHeader_EmptyHeader(t *testing.T) {
	t.Parallel()
	info, err := ParseFromHeader("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Authenticated {
		t.Error("expected Authenticated=false for empty header")
	}
}

func TestParseFromHeader_ValidToken(t *testing.T) {
	t.Parallel()
	tok := signTestToken(t, &Claims{
		Roles: []string{"admin", "viewer"},
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "alice",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	info, err := ParseFromHeader("Bearer " + tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Authenticated {
		t.Error("expected Authenticated=true")
	}
	if info.Subject != "alice" {
		t.Errorf("Subject = %q, want %q", info.Subject, "alice")
	}
	if len(info.Roles) != 2 || info.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin viewer]", info.Roles)
	}
}

func TestParseFromHeader_PrivilegesField(t *testing.T) {
	t.Parallel()
	tok := signTestToken(t, &Claims{
		Roles:      []string{"admin"},
		Privileges: "dGVzdC1wcml2aWxlZ2Vz",
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "alice",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	info, err := ParseFromHeader("Bearer " + tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Privileges != "dGVzdC1wcml2aWxlZ2Vz" {
		t.Errorf("Privileges = %q, want %q", info.Privileges, "dGVzdC1wcml2aWxlZ2Vz")
	}
}

func TestParseFromHeader_ExpiredToken(t *testing.T) {
	t.Parallel()
	tok := signTestToken(t, &Claims{
		Roles: []string{"admin"},
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "bob",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	})

	_, err := ParseFromHeader("Bearer " + tok)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseFromHeader_InvalidFormat(t *testing.T) {
	t.Parallel()
	_, err := ParseFromHeader("Basic abc123")
	if err == nil {
		t.Fatal("expected error for non-Bearer scheme")
	}
}

func TestParseFromHeader_EmptyBearerToken(t *testing.T) {
	t.Parallel()
	_, err := ParseFromHeader("Bearer ")
	if err == nil {
		t.Fatal("expected error for empty bearer token")
	}
}

func TestParseFromHeader_MissingSubject(t *testing.T) {
	t.Parallel()
	tok := signTestToken(t, &Claims{
		Roles: []string{"admin"},
		RegisteredClaims: gojwt.RegisteredClaims{
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	_, err := ParseFromHeader("Bearer " + tok)
	if err == nil {
		t.Fatal("expected error for missing subject")
	}
}

func TestParseFromHeader_InvalidSignature(t *testing.T) {
	t.Parallel()
	claims := &Claims{
		Roles: []string{"admin"},
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "alice",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("signTestToken: %v", err)
	}

	_, err = ParseFromHeader("Bearer " + s)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestParseFromHeader_BearerCaseInsensitive(t *testing.T) {
	t.Parallel()
	tok := signTestToken(t, &Claims{
		Roles: []string{"viewer"},
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "carol",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	info, err := ParseFromHeader("bearer " + tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Subject != "carol" {
		t.Errorf("Subject = %q, want %q", info.Subject, "carol")
	}
}

func TestParseFromHeader_MalformedHeader(t *testing.T) {
	t.Parallel()
	_, err := ParseFromHeader("justAtoken")
	if err == nil {
		t.Fatal("expected error for malformed header (no space)")
	}
}
