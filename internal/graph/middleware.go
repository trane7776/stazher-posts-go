package graph

import (
	"context"
	"net/http"
	"strings"
)

type userIDKey struct{}

func identifyUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-User-ID"))
		ctx := context.WithValue(r.Context(), userIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
