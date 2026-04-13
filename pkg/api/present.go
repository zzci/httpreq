package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpdns/pkg/httpdns"
)

// resolveSubdomain resolves the nanoid subdomain from the FQDN.
// lego may send either the original FQDN (_acme-challenge.pvv.cc.)
// or the CNAME-resolved FQDN (r0hc4bc6.s.dnsall.com.).
func (a *API) resolveSubdomain(userID int64, fqdn string) (string, error) {
	// Case 1: FQDN is under our base domain (CNAME-resolved), e.g. r0hc4bc6.s.dnsall.com
	if sub, ok := httpdns.ExtractSubdomainFromFQDN(fqdn, a.Config.General.Domain); ok {
		// Verify this subdomain belongs to the authenticated user
		ownerID, err := a.DB.GetSubdomainOwner(sub)
		if err == nil && ownerID == userID {
			return sub, nil
		}
	}

	// Case 2: Original FQDN, e.g. _acme-challenge.pvv.cc
	domain := httpdns.ExtractDomainFromFQDN(fqdn)
	return a.DB.GetSubdomainByUserDomain(userID, domain)
}

// webPresentPost handles POST /present (lego httpreq DNS provider).
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

	subdomain, err := a.resolveSubdomain(user.ID, payload.FQDN)
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
