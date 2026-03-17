package jwt

import (
	"fmt"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"
)

const defaultSecret = "demo-secret"

type UserInfo struct {
	Subject       string
	Roles         []string
	Authenticated bool
}

type Claims struct {
	Roles []string `json:"roles"`
	gojwt.RegisteredClaims
}

// ParseFromHeader 从 Authorization: Bearer <token> 中解析 JWT。
// 如果 header 为空则返回未认证的 UserInfo（不返回 error）。
func ParseFromHeader(authHeader string) (*UserInfo, error) {
	if authHeader == "" {
		return &UserInfo{Authenticated: false}, nil
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	tokenString := strings.TrimSpace(parts[1])
	if tokenString == "" {
		return nil, fmt.Errorf("empty bearer token")
	}

	claims := &Claims{}
	token, err := gojwt.ParseWithClaims(tokenString, claims, func(token *gojwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*gojwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return []byte(defaultSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if claims.Subject == "" {
		return nil, fmt.Errorf("token subject is required")
	}

	return &UserInfo{
		Subject:       claims.Subject,
		Roles:         claims.Roles,
		Authenticated: true,
	}, nil
}
