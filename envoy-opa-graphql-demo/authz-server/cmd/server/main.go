package main

import (
	"context"
	"log"
	"net"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc"

	"authz-server/internal/extauthz"
	"authz-server/internal/opa"
)

func main() {
	ctx := context.Background()

	evaluator, err := opa.NewEvaluator(ctx, "policy/graphql_authz.rego")
	if err != nil {
		log.Fatalf("create opa evaluator: %v", err)
	}

	srv := grpc.NewServer()
	authServer := extauthz.NewServer(evaluator)
	authv3.RegisterAuthorizationServer(srv, authServer)

	lis, err := net.Listen("tcp", ":9001")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	log.Println("authz-server listening on :9001")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("grpc serve: %v", err)
	}
}
