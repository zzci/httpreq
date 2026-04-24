package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpreq/pkg/httpreq"
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
// Accepts username + api_key (preferred) or username + password via HTTP Basic Auth.
func (a *API) BasicAuthHTTPreq(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		username, secret, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpdns"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		user, err := a.DB.GetUserByUsername(username)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpdns"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		// Basic Auth only accepts api_key, not password
		authenticated := user.APIKey != "" && subtle.ConstantTimeCompare([]byte(secret), []byte(user.APIKey)) == 1
		if !authenticated {
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

func getUserFromContext(r *http.Request) (httpreq.User, bool) {
	u, ok := r.Context().Value(userContextKey).(httpreq.User)
	return u, ok
}
