package graph

import (
	"context"
	"errors"
	"fmt"

	"demo-opa-graphql/internal/security"
)

var errUnauthenticated = errors.New("unauthenticated")

func (r *Resolver) authorize(ctx context.Context, operation, field string, args map[string]interface{}) error {
	if r.Authorizer == nil {
		return fmt.Errorf("authorizer not configured")
	}

	principal, ok := security.PrincipalFromContext(ctx)
	input := security.PolicyInput{
		Operation: operation,
		Field:     field,
		Args:      args,
		User: security.PolicyUser{
			Authenticated: ok,
		},
	}
	if ok {
		input.User.Subject = principal.Subject
		input.User.Roles = append([]string(nil), principal.Roles...)
	}

	decision, err := r.Authorizer.Evaluate(ctx, input)
	if err != nil {
		return fmt.Errorf("authorize via opa: %w", err)
	}
	if decision.Allow {
		return nil
	}
	if decision.Reason == "requires authentication" {
		return errUnauthenticated
	}
	return fmt.Errorf("forbidden: %s", decision.Reason)
}

func unionRoles(base []string, extra []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+len(extra))
	for _, role := range base {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	for _, role := range extra {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	return out
}
