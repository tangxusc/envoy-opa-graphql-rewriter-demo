package main

import (
	"log"
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"

	"graphql-server/graph"
	"graphql-server/graph/generated"
)

func main() {
	resolver := graph.NewResolver()
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	// SSE 必须在 POST 之前注册，以便先匹配 Accept: text/event-stream
	srv.AddTransport(transport.SSE{})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	mux := http.NewServeMux()
	mux.Handle("/", playground.Handler("GraphQL Playground", "/query"))
	mux.Handle("/query", srv)

	log.Println("graphql-server listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
