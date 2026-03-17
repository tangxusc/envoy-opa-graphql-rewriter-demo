package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"

	"demo-opa-graphql/graph"
	"demo-opa-graphql/graph/generated"
	"demo-opa-graphql/internal/security"
)

func main() {
	addr := envOrDefault("ADDR", ":8080")
	secret := []byte(envOrDefault("JWT_SECRET", "demo-secret"))
	policyPath := envOrDefault("POLICY_PATH", "policy/authz.rego")

	authorizer, err := security.NewAuthorizer(policyPath)
	if err != nil {
		log.Fatalf("load authorizer: %v", err)
	}

	resolver := graph.NewResolver(authorizer)
	graphqlServer := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	mux := http.NewServeMux()
	mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.Handle("/query", security.JWTMiddleware(secret)(graphqlServer))

	log.Printf("listening on %s", addr)
	printDemoTokens(secret)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

func printDemoTokens(secret []byte) {
	userToken, err := security.IssueDemoToken(secret, "user-1", []string{"user"}, 24*time.Hour)
	if err != nil {
		log.Printf("issue user token: %v", err)
	} else {
		log.Printf("demo user token (role=user): %s", userToken)
	}

	adminToken, err := security.IssueDemoToken(secret, "admin-1", []string{"admin"}, 24*time.Hour)
	if err != nil {
		log.Printf("issue admin token: %v", err)
	} else {
		log.Printf("demo admin token (role=admin): %s", adminToken)
	}
}

func envOrDefault(key, value string) string {
	if got := os.Getenv(key); got != "" {
		return got
	}
	return value
}
