package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpreq/pkg/httpreq"
)

// webCleanupPost handles POST /cleanup (lego httpreq DNS provider).
// Authenticates via Basic Auth, looks up the user's nanoid subdomain, then removes the TXT record.
func (a *API) webCleanupPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var payload httpreq.HTTPReqPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.Logger.Errorw("Cleanup: JSON decode error", "error", err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_json"))
		return
	}
	if payload.FQDN == "" || payload.Value == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("missing_fqdn_or_value"))
		return
	}

	user, ok := getUserFromContext(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(jsonError("unauthorized"))
		return
	}

	subdomain, err := a.resolveSubdomain(user.ID, payload.FQDN)
	if err != nil {
		a.Logger.Errorw("Cleanup: domain not authorized",
			"user", user.Username, "fqdn", payload.FQDN)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(jsonError("domain_not_authorized"))
		return
	}

	internalDomain := httpreq.InternalDomain(subdomain, a.Config.General.Domain)

	if err := a.DB.CleanupTXT(internalDomain, payload.Value); err != nil {
		a.Logger.Errorw("Cleanup: DB error", "error", err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}

	a.Logger.Infow("TXT record cleaned up",
		"fqdn", payload.FQDN, "internal_domain", internalDomain)
	w.WriteHeader(http.StatusOK)
}
