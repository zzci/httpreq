package httpreq

import "time"

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
