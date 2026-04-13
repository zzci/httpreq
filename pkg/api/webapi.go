package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/zzci/httpdns/pkg/httpdns"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

type addDomainRequest struct {
	Domain string `json:"domain"`
}

// POST /api/register
func (a *API) apiRegister(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 6 {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "username required, password min 6 chars"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	user, err := a.DB.CreateUser(req.Username, string(hash))
	if err != nil {
		a.Logger.Errorw("Register: DB error", "error", err.Error())
		jsonResp(w, http.StatusConflict, map[string]string{"error": "username_taken"})
		return
	}
	token, err := a.generateToken(user.ID, user.Username)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "token_error"})
		return
	}
	jsonResp(w, http.StatusCreated, loginResponse{Token: token, Username: user.Username})
}

// POST /api/login
func (a *API) apiLogin(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	user, err := a.DB.GetUserByUsername(strings.TrimSpace(req.Username))
	if err != nil {
		bcrypt.CompareHashAndPassword([]byte("$2a$10$placeholder_hash_for_timing"), []byte(req.Password))
		jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	token, err := a.generateToken(user.ID, user.Username)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "token_error"})
		return
	}
	jsonResp(w, http.StatusOK, loginResponse{Token: token, Username: user.Username})
}

// GET /api/domains
func (a *API) apiGetDomains(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	user, _ := getUserFromContext(r)
	domains, err := a.DB.GetUserDomains(user.ID)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	if domains == nil {
		domains = []httpdns.UserDomain{}
	}
	for i := range domains {
		domains[i].CNAMETarget = httpdns.InternalDomain(domains[i].Subdomain, a.Config.General.Domain)
	}
	jsonResp(w, http.StatusOK, domains)
}

// POST /api/domains
func (a *API) apiAddDomain(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	user, _ := getUserFromContext(r)
	var req addDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if domain == "" {
		jsonResp(w, http.StatusBadRequest, map[string]string{"error": "domain_required"})
		return
	}
	ud, err := a.DB.AddUserDomain(user.ID, domain)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			jsonResp(w, http.StatusConflict, map[string]string{"error": "domain_already_exists"})
			return
		}
		a.Logger.Errorw("AddDomain: DB error", "error", err.Error())
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	ud.CNAMETarget = httpdns.InternalDomain(ud.Subdomain, a.Config.General.Domain)
	jsonResp(w, http.StatusCreated, ud)
}

// DELETE /api/domains/:domain
func (a *API) apiRemoveDomain(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	user, _ := getUserFromContext(r)
	domain := strings.ToLower(strings.TrimSpace(p.ByName("domain")))
	if err := a.DB.RemoveUserDomain(user.ID, domain); err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/records
func (a *API) apiGetRecords(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	user, _ := getUserFromContext(r)
	domains, err := a.DB.GetUserDomains(user.ID)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	internalDomains := make([]string, len(domains))
	for i, d := range domains {
		internalDomains[i] = httpdns.InternalDomain(d.Subdomain, a.Config.General.Domain)
	}
	records, err := a.DB.ListTXTRecordsByDomains(internalDomains)
	if err != nil {
		jsonResp(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	if records == nil {
		records = []httpdns.TXTRecord{}
	}
	jsonResp(w, http.StatusOK, records)
}

// GET /api/info
func (a *API) apiInfo(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	apiDomain := a.Config.API.Domain
	if apiDomain == "" {
		apiDomain = a.Config.General.Domain
	}
	jsonResp(w, http.StatusOK, map[string]string{
		"base_domain": a.Config.General.Domain,
		"api_domain":  apiDomain,
	})
}

func jsonResp(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
