// Package auth defines Soro's authentication-neutral request principal context.
package auth

import (
	"context"

	"github.com/google/uuid"
)

type Principal interface {
	ID() uuid.UUID
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

func ActorID(ctx context.Context) *uuid.UUID {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal == nil {
		return nil
	}
	id := principal.ID()
	if id == uuid.Nil {
		return nil
	}
	return &id
}
