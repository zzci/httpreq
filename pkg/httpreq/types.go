package httpreq

import (
	"strings"
	"time"
)

// Config holds the config structure
type Config struct {
	General   general
	Database  dbsettings
	API       httpapi
	Logconfig logconfig
}

// Config file general section
type general struct {
	Listen        string
	Proto         string `toml:"protocol"`
	Domain        string
	Nsname        string
	Nsadmin       string
	Debug         bool
	StaticRecords []string `toml:"records"`
}

type dbsettings struct {
	Engine     string
	Connection string
}

// User represents a registered user stored in the database
type User struct {
	ID           int64    `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"-"`
	APIKey       string   `json:"api_key,omitempty"`
	CreatedAt    int64    `json:"created_at"`
	Domains      []string `json:"domains,omitempty"`
}

// UserDomain represents a domain owned by a user with its unique subdomain
type UserDomain struct {
	Domain      string `json:"domain"`
	Subdomain   string `json:"subdomain"`
	CNAMETarget string `json:"cname_target"`
	Owner       string `json:"owner,omitempty"`
}

// API config
type httpapi struct {
	Domain            string `toml:"api_domain"`
	IP                string
	AutocertPort      string `toml:"autocert_port"`
	Port              string `toml:"port"`
	TLS               string
	TLSCertPrivkey    string `toml:"tls_cert_privkey"`
	TLSCertFullchain  string `toml:"tls_cert_fullchain"`
	ACMECacheDir      string `toml:"acme_cache_dir"`
	NotificationEmail string `toml:"notification_email"`
	CorsOrigins       []string
	UseHeader         bool   `toml:"use_header"`
	HeaderName        string `toml:"header_name"`
	// JWT secret for API token signing (auto-generated if empty)
	JWTSecret string `toml:"jwt_secret"`
	// Admin API key for managing all users and domains
	AdminKey string `toml:"admin_key"`
}

// Logging config
type logconfig struct {
	Level   string `toml:"loglevel"`
	Logtype string `toml:"logtype"`
	File    string `toml:"logfile"`
	Format  string `toml:"logformat"`
}

// APIKey represents a user's API key with domain scope
type APIKey struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"-"`
	Name      string   `json:"name"`
	Key       string   `json:"key"`
	Scope     []string `json:"scope"`
	CreatedAt int64    `json:"created_at"`
}

// IsGlobal returns true if this key has global scope
func (k *APIKey) IsGlobal() bool {
	for _, s := range k.Scope {
		if s == "*" {
			return true
		}
	}
	return false
}

// HasDomainAccess checks if a key can operate on a domain.
// Supports exact match ("example.com") and wildcard ("*.example.com").
// Wildcard matches the root domain and all subdomains.
func (k *APIKey) HasDomainAccess(domain string) bool {
	if k.IsGlobal() {
		return true
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	for _, s := range k.Scope {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == domain {
			return true
		}
		if strings.HasPrefix(s, "*.") {
			root := s[2:] // "*.example.com" → "example.com"
			if domain == root || strings.HasSuffix(domain, "."+root) {
				return true
			}
		}
	}
	return false
}

// HTTPReqPayload is the request body for lego httpreq /present and /cleanup
type HTTPReqPayload struct {
	FQDN  string `json:"fqdn"`
	Value string `json:"value"`
}

// HTTPReqResponse is the response body for /present, informing the user of the CNAME target
type HTTPReqResponse struct {
	InternalDomain string `json:"internal_domain"`
	CNAMETarget    string `json:"cname_target"`
}

// TXTRecord represents a TXT record stored in the database
type TXTRecord struct {
	Domain     string    `json:"domain"`
	Value      string    `json:"value"`
	LastUpdate time.Time `json:"last_update"`
}
