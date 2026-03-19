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

	"todo-server/graph"
	"todo-server/graph/generated"
)

func main() {
	publisher, err := graph.NewKafkaPublisherFromEnv()
	if err != nil {
		log.Fatalf("init event publisher: %v", err)
	}

	resolver := graph.NewResolver(publisher)
	defer func() {
		if err := resolver.Close(); err != nil {
			log.Printf("close publisher: %v", err)
		}
	}()

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
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
	mux.Handle("/", playground.Handler("Todo GraphQL Playground", "/query"))
	mux.Handle("/query", srv)

	log.Println("todo-server listening on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
