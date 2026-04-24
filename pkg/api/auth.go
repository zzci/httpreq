package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpreq/pkg/httpreq"
)

type contextKey int

const (
	userContextKey   contextKey = 0
	apiKeyContextKey contextKey = 1
)

// JWTOrKeyAuth middleware for /api/* endpoints.
// Accepts JWT Bearer token OR API key as Bearer token.
func (a *API) JWTOrKeyAuth(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("missing_token"))
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// Try JWT first
		if claims, err := a.parseToken(tokenStr); err == nil {
			if user, err := a.DB.GetUserByID(claims.UserID); err == nil {
				ctx := context.WithValue(r.Context(), userContextKey, user)
				next(w, r.WithContext(ctx), p)
				return
			}
		}

		// Try API key
		if apiKey, err := a.DB.GetAPIKeyByValue(tokenStr); err == nil {
			if user, err := a.DB.GetUserByID(apiKey.UserID); err == nil {
				ctx := context.WithValue(r.Context(), userContextKey, user)
				ctx = context.WithValue(ctx, apiKeyContextKey, apiKey)
				next(w, r.WithContext(ctx), p)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(jsonError("invalid_token"))
	}
}

// BasicAuthHTTPreq middleware for /present and /cleanup endpoints.
// Looks up API key from api_keys table, falls back to users.api_key.
func (a *API) BasicAuthHTTPreq(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		username, secret, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpreq"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		user, err := a.DB.GetUserByUsername(username)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="httpreq"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}

		// Try api_keys table first
		if apiKey, err := a.DB.GetAPIKeyByValue(secret); err == nil && apiKey.UserID == user.ID {
			ctx := context.WithValue(r.Context(), userContextKey, user)
			ctx = context.WithValue(ctx, apiKeyContextKey, apiKey)
			next(w, r.WithContext(ctx), p)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="httpreq"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(jsonError("unauthorized"))
	}
}

func getUserFromContext(r *http.Request) (httpreq.User, bool) {
	u, ok := r.Context().Value(userContextKey).(httpreq.User)
	return u, ok
}

func getAPIKeyFromContext(r *http.Request) (httpreq.APIKey, bool) {
	k, ok := r.Context().Value(apiKeyContextKey).(httpreq.APIKey)
	return k, ok
}
