package main

import (
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

const secret = "demo-secret"

type Claims struct {
	Roles []string `json:"roles"`
	gojwt.RegisteredClaims
}

func main() {
	adminToken, err := issueToken("admin-1", []string{"admin"}, 24*time.Hour)
	if err != nil {
		panic(err)
	}
	fmt.Println("=== Admin Token (role=admin) ===")
	fmt.Println(adminToken)
	fmt.Println()

	userToken, err := issueToken("user-1", []string{"user"}, 24*time.Hour)
	if err != nil {
		panic(err)
	}
	fmt.Println("=== User Token (role=user) ===")
	fmt.Println(userToken)
}

func issueToken(subject string, roles []string, ttl time.Duration) (string, error) {
	claims := Claims{
		Roles: roles,
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  gojwt.NewNumericDate(time.Now()),
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	return gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
