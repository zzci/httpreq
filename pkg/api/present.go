package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpdns/pkg/httpdns"
)

// webPresentPost handles POST /present (lego httpreq DNS provider).
// Authenticates via Basic Auth, looks up the user's nanoid subdomain for the domain,
// then stores the TXT record under <nanoid>.<baseDomain>.
func (a *API) webPresentPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var payload httpdns.HTTPReqPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.Logger.Errorw("Present: JSON decode error", "error", err.Error())
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

	// Look up the nanoid subdomain for this user+domain
	domain := httpdns.ExtractDomainFromFQDN(payload.FQDN)
	subdomain, err := a.DB.GetSubdomainByUserDomain(user.ID, domain)
	if err != nil {
		a.Logger.Errorw("Present: domain not authorized",
			"user", user.Username, "fqdn", payload.FQDN)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(jsonError("domain_not_authorized"))
		return
	}

	internalDomain := httpdns.InternalDomain(subdomain, a.Config.General.Domain)

	if err := a.DB.PresentTXT(internalDomain, payload.Value); err != nil {
		a.Logger.Errorw("Present: DB error", "error", err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}

	a.Logger.Infow("TXT record presented",
		"fqdn", payload.FQDN, "internal_domain", internalDomain)

	resp := httpdns.HTTPReqResponse{
		InternalDomain: internalDomain,
		CNAMETarget:    internalDomain,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
