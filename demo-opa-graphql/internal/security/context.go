package security

import "context"

type principalKey struct{}

type Principal struct {
	Subject string
	Roles   []string
}

func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(*Principal)
	if !ok || principal == nil {
		return nil, false
	}
	return principal, true
}
