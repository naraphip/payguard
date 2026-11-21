package main

import (
	"context"
	"net/http"
	"slices"
)

// In production the outer session middleware has already authenticated all API requests.
func productionRoles(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if actor, ok := productionActorFromRequest(r); ok && !slices.Contains(roles, actor.Role) {
			problem(w, http.StatusForbidden, "This account does not have access to this operation.")
			return
		}
		next(w, r)
	}
}

func auditActor(ctx context.Context, sandboxActor string) string {
	actor, ok := productionActorFromContext(ctx)
	if ok {
		return actor.ID
	}
	return sandboxActor
}
