package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/parser"
)

// Decision models the authorization decision payload.
type Decision struct {
	Allow         bool     `json:"allow"`
	RemovedFields []string `json:"removed_fields"`
}

// RewriteQuery rewrites a GraphQL query by removing fields listed in decision.removed_fields.
func RewriteQuery(query string, decisionJSON []byte) (string, error) {
	var decision Decision
	if err := json.Unmarshal(decisionJSON, &decision); err != nil {
		return "", fmt.Errorf("unmarshal decision: %w", err)
	}

	if !decision.Allow {
		return "", fmt.Errorf("request denied by policy")
	}

	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return "", fmt.Errorf("parse graphql query: %w", err)
	}

	removedSet := make(map[string]struct{}, len(decision.RemovedFields))
	for _, path := range decision.RemovedFields {
		normalized := normalizePath(path)
		if normalized == "" {
			continue
		}
		removedSet[normalized] = struct{}{}
	}

	fragments := make(map[string]*ast.FragmentDefinition, len(doc.Fragments))
	for _, frag := range doc.Fragments {
		fragments[frag.Name] = frag
	}

	for _, op := range doc.Operations {
		op.SelectionSet = rewriteSelectionSet(op.SelectionSet, nil, removedSet, fragments)
	}

	// Fragment spreads are inlined during rewrite; drop fragment definitions in output.
	doc.Fragments = nil

	var b strings.Builder
	f := formatter.NewFormatter(&b)
	f.FormatQueryDocument(doc)
	return b.String(), nil
}

func rewriteSelectionSet(
	selectionSet ast.SelectionSet,
	parentPath []string,
	removedSet map[string]struct{},
	fragments map[string]*ast.FragmentDefinition,
) ast.SelectionSet {
	out := make(ast.SelectionSet, 0, len(selectionSet))

	for _, sel := range selectionSet {
		switch s := sel.(type) {
		case *ast.Field:
			fieldPath := appendPath(parentPath, s.Name)
			if shouldRemove(fieldPath, removedSet) {
				continue
			}

			if len(s.SelectionSet) > 0 {
				s.SelectionSet = rewriteSelectionSet(s.SelectionSet, fieldPath, removedSet, fragments)
				if len(s.SelectionSet) == 0 {
					// Object field without sub-selections is invalid; drop it.
					continue
				}
			}
			out = append(out, s)

		case *ast.FragmentSpread:
			frag := fragments[s.Name]
			if frag == nil {
				continue
			}
			inlined := rewriteSelectionSet(frag.SelectionSet, parentPath, removedSet, fragments)
			out = append(out, inlined...)

		case *ast.InlineFragment:
			s.SelectionSet = rewriteSelectionSet(s.SelectionSet, parentPath, removedSet, fragments)
			if len(s.SelectionSet) == 0 {
				continue
			}
			out = append(out, s)
		}
	}

	return out
}

func shouldRemove(path []string, removedSet map[string]struct{}) bool {
	_, ok := removedSet[strings.Join(path, ".")]
	return ok
}

func appendPath(parent []string, segment string) []string {
	child := make([]string, 0, len(parent)+1)
	child = append(child, parent...)
	child = append(child, segment)
	return child
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	parts := strings.Split(path, ".")
	cleaned := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return strings.Join(cleaned, ".")
}
