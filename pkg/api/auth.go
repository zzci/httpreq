package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpdns/pkg/httpdns"
	"golang.org/x/crypto/bcrypt"
)

type contextKey int

const userContextKey contextKey = 0

// JWTAuth middleware for /api/* endpoints. Requires a valid Bearer token.
func (a *API) JWTAuth(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("missing_token"))
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := a.parseToken(tokenStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("invalid_token"))
			return
		}
		user, err := a.DB.GetUserByID(claims.UserID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("user_not_found"))
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx), p)
	}
}

// BasicAuthHTTPreq middleware for /present and /cleanup endpoints.
// Authenticates using HTTP Basic Auth against database users.
func (a *API) BasicAuthHTTPreq(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpdns"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		user, err := a.DB.GetUserByUsername(username)
		if err != nil {
			// Constant-time comparison to prevent timing attacks
			bcrypt.CompareHashAndPassword([]byte("$2a$10$placeholder_hash_for_timing"), []byte(password))
			w.Header().Set("WWW-Authenticate", `Basic realm="httpdns"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpdns"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx), p)
	}
}

func getUserFromContext(r *http.Request) (httpdns.User, bool) {
	u, ok := r.Context().Value(userContextKey).(httpdns.User)
	return u, ok
}
