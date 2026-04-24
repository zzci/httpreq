package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/lib/pq"

	"go.uber.org/zap"

	"github.com/zzci/httpreq/pkg/httpreq"
)

type acmednsdb struct {
	DB     *sql.DB
	Mutex  sync.Mutex
	Logger *zap.SugaredLogger
	Config *httpreq.Config
}

// DBVersion shows the database version this code uses.
var DBVersion = 5

var acmeTable = `
	CREATE TABLE IF NOT EXISTS acmedns(
		Name TEXT,
		Value TEXT
	);`

var txtTable = `
	CREATE TABLE IF NOT EXISTS txt(
		Domain TEXT NOT NULL,
		Value TEXT NOT NULL DEFAULT '',
		LastUpdate INT
	);`

var txtTablePG = `
	CREATE TABLE IF NOT EXISTS txt(
		rowid SERIAL,
		Domain TEXT NOT NULL,
		Value TEXT NOT NULL DEFAULT '',
		LastUpdate INT
	);`

var usersTable = `
	CREATE TABLE IF NOT EXISTS users(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		api_key TEXT UNIQUE NOT NULL,
		created_at INT NOT NULL
	);`

var usersTablePG = `
	CREATE TABLE IF NOT EXISTS users(
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		api_key TEXT UNIQUE NOT NULL,
		created_at INT NOT NULL
	);`

var userDomainsTable = `
	CREATE TABLE IF NOT EXISTS user_domains(
		user_id INTEGER NOT NULL,
		domain TEXT NOT NULL,
		subdomain TEXT UNIQUE NOT NULL,
		UNIQUE(user_id, domain),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);`

var userDomainsTablePG = `
	CREATE TABLE IF NOT EXISTS user_domains(
		user_id INTEGER NOT NULL REFERENCES users(id),
		domain TEXT NOT NULL,
		subdomain TEXT UNIQUE NOT NULL,
		UNIQUE(user_id, domain)
	);`

var apiKeysTable = `
	CREATE TABLE IF NOT EXISTS api_keys(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		key_value TEXT UNIQUE NOT NULL,
		scope TEXT NOT NULL DEFAULT '["*"]',
		created_at INT NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);`

var apiKeysTablePG = `
	CREATE TABLE IF NOT EXISTS api_keys(
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		key_value TEXT UNIQUE NOT NULL,
		scope TEXT NOT NULL DEFAULT '["*"]',
		created_at INT NOT NULL
	);`

// getSQLiteStmt replaces all PostgreSQL prepared statement placeholders (eg. $1, $2) with SQLite variant "?"
func getSQLiteStmt(s string) string {
	re, _ := regexp.Compile(`\$[0-9]+`)
	return re.ReplaceAllString(s, "?")
}

func Init(config *httpreq.Config, logger *zap.SugaredLogger) (httpreq.DB, error) {
	var d = &acmednsdb{Config: config, Logger: logger}
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	// Ensure parent directory exists for sqlite files
	if config.Database.Engine == "sqlite" {
		if dir := filepath.Dir(config.Database.Connection); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
	}
	db, err := sql.Open(config.Database.Engine, config.Database.Connection)
	if err != nil {
		return d, err
	}
	d.DB = db
	var versionString string
	_ = d.DB.QueryRow("SELECT Value FROM acmedns WHERE Name='db_version'").Scan(&versionString)
	if versionString == "" {
		versionString = "0"
	}
	_, _ = d.DB.Exec(acmeTable)
	if config.Database.Engine == "sqlite" {
		_, _ = d.DB.Exec(txtTable)
		_, _ = d.DB.Exec(usersTable)
		_, _ = d.DB.Exec(userDomainsTable)
		_, _ = d.DB.Exec(apiKeysTable)
	} else {
		_, _ = d.DB.Exec(txtTablePG)
		_, _ = d.DB.Exec(usersTablePG)
		_, _ = d.DB.Exec(userDomainsTablePG)
		_, _ = d.DB.Exec(apiKeysTablePG)
	}
	if err == nil {
		err = d.checkDBUpgrades(versionString)
	}
	if err == nil {
		if versionString == "0" {
			insversion := fmt.Sprintf("INSERT INTO acmedns (Name, Value) values('db_version', '%d')", DBVersion)
			_, err = db.Exec(insversion)
		}
	}
	return d, err
}

func (d *acmednsdb) checkDBUpgrades(versionString string) error {
	version, err := strconv.Atoi(versionString)
	if err != nil {
		return err
	}
	if version != DBVersion {
		return d.handleDBUpgrades(version)
	}
	return nil
}

func (d *acmednsdb) handleDBUpgrades(version int) error {
	if version < 2 {
		if err := d.handleDBUpgradeTo2(); err != nil {
			return err
		}
	}
	if version < 3 {
		if err := d.handleDBUpgradeTo3(); err != nil {
			return err
		}
	}
	if version < 4 {
		if err := d.handleDBUpgradeTo4(); err != nil {
			return err
		}
	}
	if version < 5 {
		if err := d.handleDBUpgradeTo5(); err != nil {
			return err
		}
	}
	return nil
}

func (d *acmednsdb) handleDBUpgradeTo2() error {
	var hasOldColumn bool
	if d.Config.Database.Engine == "sqlite" {
		rows, err := d.DB.Query("PRAGMA table_info(txt)")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cid int
				var name, ctype string
				var notnull int
				var dfltValue *string
				var pk int
				if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
					if strings.EqualFold(name, "Subdomain") {
						hasOldColumn = true
					}
				}
			}
		}
	} else {
		var count int
		err := d.DB.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_name='txt' AND column_name='subdomain'").Scan(&count)
		if err == nil && count > 0 {
			hasOldColumn = true
		}
	}

	if hasOldColumn {
		if d.Config.Database.Engine == "sqlite" {
			_, _ = d.DB.Exec("ALTER TABLE txt RENAME TO txt_old")
			_, _ = d.DB.Exec(txtTable)
			_, _ = d.DB.Exec("INSERT INTO txt (Domain, Value, LastUpdate) SELECT Subdomain, Value, LastUpdate FROM txt_old")
			_, _ = d.DB.Exec("DROP TABLE txt_old")
		} else {
			_, _ = d.DB.Exec("ALTER TABLE txt RENAME COLUMN Subdomain TO Domain")
		}
	}

	_, err := d.DB.Exec("UPDATE acmedns SET Value='2' WHERE Name='db_version'")
	return err
}

func (d *acmednsdb) handleDBUpgradeTo3() error {
	_, err := d.DB.Exec("UPDATE acmedns SET Value='3' WHERE Name='db_version'")
	return err
}

func (d *acmednsdb) handleDBUpgradeTo4() error {
	// Add api_key column — SQLite doesn't support UNIQUE in ALTER TABLE ADD COLUMN
	if d.Config.Database.Engine == "sqlite" {
		_, _ = d.DB.Exec("ALTER TABLE users ADD COLUMN api_key TEXT DEFAULT ''")
		// Create unique index separately
		_, _ = d.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key) WHERE api_key != ''")
	} else {
		_, _ = d.DB.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS api_key TEXT DEFAULT ''")
		_, _ = d.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key) WHERE api_key != ''")
	}
	// Generate api_key for existing users that don't have one
	rows, err := d.DB.Query("SELECT id FROM users WHERE api_key = '' OR api_key IS NULL")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				key := httpreq.GenerateAPIKey()
				_, _ = d.DB.Exec("UPDATE users SET api_key = ? WHERE id = ?", key, id)
			}
		}
	}
	_, err = d.DB.Exec("UPDATE acmedns SET Value='4' WHERE Name='db_version'")
	return err
}

func (d *acmednsdb) handleDBUpgradeTo5() error {
	// api_keys table created in Init, just bump version
	// Migrate existing users.api_key to api_keys table as default global key
	rows, err := d.DB.Query("SELECT id, username, api_key FROM users WHERE api_key != '' AND api_key IS NOT NULL")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var userID int64
			var username, apiKey string
			if err := rows.Scan(&userID, &username, &apiKey); err == nil {
				now := time.Now().Unix()
				_, _ = d.DB.Exec("INSERT OR IGNORE INTO api_keys (user_id, name, key_value, scope, created_at) VALUES (?, ?, ?, ?, ?)",
					userID, "Default", apiKey, `["*"]`, now)
			}
		}
	}
	_, err = d.DB.Exec("UPDATE acmedns SET Value='5' WHERE Name='db_version'")
	return err
}

// --- API Key methods ---

func (d *acmednsdb) CreateAPIKey(userID int64, name string, scope []string) (httpreq.APIKey, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	keyValue := httpreq.GenerateAPIKey()
	scopeJSON, _ := json.Marshal(scope)
	now := time.Now().Unix()
	insSQL := `INSERT INTO api_keys (user_id, name, key_value, scope, created_at) VALUES ($1, $2, $3, $4, $5)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	result, err := d.DB.Exec(insSQL, userID, name, keyValue, string(scopeJSON), now)
	if err != nil {
		return httpreq.APIKey{}, fmt.Errorf("failed to create api key: %w", err)
	}
	id, _ := result.LastInsertId()
	return httpreq.APIKey{ID: id, UserID: userID, Name: name, Key: keyValue, Scope: scope, CreatedAt: now}, nil
}

func (d *acmednsdb) ListAPIKeys(userID int64) ([]httpreq.APIKey, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var keys []httpreq.APIKey
	getSQL := `SELECT id, user_id, name, key_value, scope, created_at FROM api_keys WHERE user_id=$1 ORDER BY created_at`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	rows, err := d.DB.Query(getSQL, userID)
	if err != nil {
		return keys, err
	}
	defer rows.Close()
	for rows.Next() {
		var k httpreq.APIKey
		var scopeJSON string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Key, &scopeJSON, &k.CreatedAt); err != nil {
			return keys, err
		}
		_ = json.Unmarshal([]byte(scopeJSON), &k.Scope)
		keys = append(keys, k)
	}
	return keys, nil
}

func (d *acmednsdb) DeleteAPIKey(userID, keyID int64) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	delSQL := `DELETE FROM api_keys WHERE id=$1 AND user_id=$2`
	if d.Config.Database.Engine == "sqlite" {
		delSQL = getSQLiteStmt(delSQL)
	}
	_, err := d.DB.Exec(delSQL, keyID, userID)
	return err
}

func (d *acmednsdb) GetAPIKeyByValue(keyValue string) (httpreq.APIKey, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var k httpreq.APIKey
	var scopeJSON string
	getSQL := `SELECT id, user_id, name, key_value, scope, created_at FROM api_keys WHERE key_value=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, keyValue).Scan(&k.ID, &k.UserID, &k.Name, &k.Key, &scopeJSON, &k.CreatedAt)
	if err != nil {
		return k, fmt.Errorf("api key not found: %w", err)
	}
	_ = json.Unmarshal([]byte(scopeJSON), &k.Scope)
	return k, nil
}

func (d *acmednsdb) UpdateAPIKeyScope(keyID int64, scope []string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	scopeJSON, _ := json.Marshal(scope)
	updSQL := `UPDATE api_keys SET scope=$1 WHERE id=$2`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}
	_, err := d.DB.Exec(updSQL, string(scopeJSON), keyID)
	return err
}

// --- TXT record methods ---

func (d *acmednsdb) PresentTXT(fqdn, value string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain := sanitizeFQDN(fqdn)
	timenow := time.Now().Unix()
	insSQL := `INSERT INTO txt (Domain, Value, LastUpdate) VALUES ($1, $2, $3)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	sm, err := d.DB.Prepare(insSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare present statement: %w", err)
	}
	defer sm.Close()
	_, err = sm.Exec(domain, value, timenow)
	return err
}

func (d *acmednsdb) CleanupTXT(fqdn, value string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain := sanitizeFQDN(fqdn)
	delSQL := `DELETE FROM txt WHERE Domain=$1 AND Value=$2`
	if d.Config.Database.Engine == "sqlite" {
		delSQL = getSQLiteStmt(delSQL)
	}
	sm, err := d.DB.Prepare(delSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare cleanup statement: %w", err)
	}
	defer sm.Close()
	_, err = sm.Exec(domain, value)
	return err
}

func (d *acmednsdb) GetTXTForDomain(domain string) ([]string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var txts []string
	getSQL := `SELECT Value FROM txt WHERE Domain=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	sm, err := d.DB.Prepare(getSQL)
	if err != nil {
		return txts, err
	}
	defer sm.Close()
	rows, err := sm.Query(domain)
	if err != nil {
		return txts, err
	}
	defer rows.Close()
	for rows.Next() {
		var rtxt string
		err = rows.Scan(&rtxt)
		if err != nil {
			return txts, err
		}
		txts = append(txts, rtxt)
	}
	return txts, nil
}

func (d *acmednsdb) ListTXTRecords() ([]httpreq.TXTRecord, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var records []httpreq.TXTRecord
	rows, err := d.DB.Query("SELECT Domain, Value, LastUpdate FROM txt ORDER BY LastUpdate DESC")
	if err != nil {
		return records, err
	}
	defer rows.Close()
	for rows.Next() {
		var r httpreq.TXTRecord
		var ts int64
		err = rows.Scan(&r.Domain, &r.Value, &ts)
		if err != nil {
			return records, err
		}
		r.LastUpdate = time.Unix(ts, 0)
		records = append(records, r)
	}
	return records, nil
}

func (d *acmednsdb) ListTXTRecordsByDomains(domains []string) ([]httpreq.TXTRecord, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	if len(domains) == 0 {
		return nil, nil
	}
	var records []httpreq.TXTRecord
	placeholders := make([]string, len(domains))
	args := make([]interface{}, len(domains))
	for i, dom := range domains {
		if d.Config.Database.Engine == "sqlite" {
			placeholders[i] = "?"
		} else {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		args[i] = sanitizeFQDN(dom)
	}
	query := fmt.Sprintf("SELECT Domain, Value, LastUpdate FROM txt WHERE Domain IN (%s) ORDER BY LastUpdate DESC",
		strings.Join(placeholders, ","))
	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return records, err
	}
	defer rows.Close()
	for rows.Next() {
		var r httpreq.TXTRecord
		var ts int64
		err = rows.Scan(&r.Domain, &r.Value, &ts)
		if err != nil {
			return records, err
		}
		r.LastUpdate = time.Unix(ts, 0)
		records = append(records, r)
	}
	return records, nil
}

// --- User methods ---

func (d *acmednsdb) CreateUser(username, passwordHash string) (httpreq.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	now := time.Now().Unix()
	apiKey := httpreq.GenerateAPIKey()
	insSQL := `INSERT INTO users (username, password_hash, api_key, created_at) VALUES ($1, $2, $3, $4)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	result, err := d.DB.Exec(insSQL, username, passwordHash, apiKey, now)
	if err != nil {
		return httpreq.User{}, fmt.Errorf("failed to create user: %w", err)
	}
	id, _ := result.LastInsertId()
	// Create default global key in api_keys table
	scopeJSON, _ := json.Marshal([]string{"*"})
	insKeySQL := `INSERT INTO api_keys (user_id, name, key_value, scope, created_at) VALUES ($1, $2, $3, $4, $5)`
	if d.Config.Database.Engine == "sqlite" {
		insKeySQL = getSQLiteStmt(insKeySQL)
	}
	_, _ = d.DB.Exec(insKeySQL, id, "Default", apiKey, string(scopeJSON), now)
	return httpreq.User{ID: id, Username: username, PasswordHash: passwordHash, APIKey: apiKey, CreatedAt: now}, nil
}

func (d *acmednsdb) GetUserByUsername(username string) (httpreq.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var u httpreq.User
	getSQL := `SELECT id, username, password_hash, api_key, created_at FROM users WHERE username=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (d *acmednsdb) GetUserByID(id int64) (httpreq.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var u httpreq.User
	getSQL := `SELECT id, username, password_hash, api_key, created_at FROM users WHERE id=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (d *acmednsdb) RegenerateAPIKey(userID int64) (string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	newKey := httpreq.GenerateAPIKey()
	updSQL := `UPDATE users SET api_key=$1 WHERE id=$2`
	if d.Config.Database.Engine == "sqlite" {
		updSQL = getSQLiteStmt(updSQL)
	}
	_, err := d.DB.Exec(updSQL, newKey, userID)
	if err != nil {
		return "", fmt.Errorf("failed to regenerate api key: %w", err)
	}
	return newKey, nil
}

// --- Domain methods ---

func (d *acmednsdb) AddUserDomain(userID int64, username, domain string) (httpreq.UserDomain, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	insSQL := `INSERT INTO user_domains (user_id, domain, subdomain) VALUES ($1, $2, $3)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	// Try with increasing salt on subdomain collision (max 10 attempts)
	for salt := 0; salt < 10; salt++ {
		subdomain := httpreq.GenerateSubdomain(username, domain, salt)
		_, err := d.DB.Exec(insSQL, userID, domain, subdomain)
		if err == nil {
			return httpreq.UserDomain{Domain: domain, Subdomain: subdomain}, nil
		}
		errStr := strings.ToLower(err.Error())
		// Only retry on subdomain uniqueness collision, not on user_id+domain duplicate
		if !strings.Contains(errStr, "unique") && !strings.Contains(errStr, "duplicate") {
			return httpreq.UserDomain{}, fmt.Errorf("failed to add domain: %w", err)
		}
		// Check if it's a user_id+domain duplicate (not a subdomain collision)
		if strings.Contains(errStr, "user_id") || strings.Contains(errStr, "user_domains.user_id") {
			return httpreq.UserDomain{}, fmt.Errorf("failed to add domain: %w", err)
		}
	}
	return httpreq.UserDomain{}, fmt.Errorf("failed to add domain: subdomain collision after retries")
}

func (d *acmednsdb) RemoveUserDomain(userID int64, domain string) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	delSQL := `DELETE FROM user_domains WHERE user_id=$1 AND domain=$2`
	if d.Config.Database.Engine == "sqlite" {
		delSQL = getSQLiteStmt(delSQL)
	}
	_, err := d.DB.Exec(delSQL, userID, domain)
	return err
}

func (d *acmednsdb) GetUserDomains(userID int64) ([]httpreq.UserDomain, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var domains []httpreq.UserDomain
	getSQL := `SELECT domain, subdomain FROM user_domains WHERE user_id=$1 ORDER BY domain`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	rows, err := d.DB.Query(getSQL, userID)
	if err != nil {
		return domains, err
	}
	defer rows.Close()
	for rows.Next() {
		var ud httpreq.UserDomain
		if err := rows.Scan(&ud.Domain, &ud.Subdomain); err != nil {
			return domains, err
		}
		domains = append(domains, ud)
	}
	return domains, nil
}

func (d *acmednsdb) GetSubdomainByUserDomain(userID int64, domain string) (string, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	var subdomain string
	getSQL := `SELECT subdomain FROM user_domains WHERE user_id=$1 AND domain=$2`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, userID, domain).Scan(&subdomain)
	if err != nil {
		return "", fmt.Errorf("domain not found: %w", err)
	}
	return subdomain, nil
}

func (d *acmednsdb) GetSubdomainOwner(subdomain string) (int64, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	subdomain = strings.ToLower(strings.TrimSpace(subdomain))
	var userID int64
	getSQL := `SELECT user_id FROM user_domains WHERE subdomain=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, subdomain).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("subdomain not found: %w", err)
	}
	return userID, nil
}

// --- Admin methods ---

func (d *acmednsdb) ListUsers() ([]httpreq.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var users []httpreq.User
	rows, err := d.DB.Query("SELECT id, username, password_hash, api_key, created_at FROM users ORDER BY id")
	if err != nil {
		return users, err
	}
	defer rows.Close()
	for rows.Next() {
		var u httpreq.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.APIKey, &u.CreatedAt); err != nil {
			return users, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *acmednsdb) DeleteUser(userID int64) error {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	delSQL := `DELETE FROM user_domains WHERE user_id=$1`
	if d.Config.Database.Engine == "sqlite" {
		delSQL = getSQLiteStmt(delSQL)
	}
	_, _ = d.DB.Exec(delSQL, userID)
	delSQL = `DELETE FROM users WHERE id=$1`
	if d.Config.Database.Engine == "sqlite" {
		delSQL = getSQLiteStmt(delSQL)
	}
	_, err := d.DB.Exec(delSQL, userID)
	return err
}

func (d *acmednsdb) ListAllDomains() ([]httpreq.UserDomain, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var domains []httpreq.UserDomain
	rows, err := d.DB.Query("SELECT ud.domain, ud.subdomain, u.username FROM user_domains ud JOIN users u ON ud.user_id = u.id ORDER BY ud.domain")
	if err != nil {
		return domains, err
	}
	defer rows.Close()
	for rows.Next() {
		var ud httpreq.UserDomain
		var owner string
		if err := rows.Scan(&ud.Domain, &ud.Subdomain, &owner); err != nil {
			return domains, err
		}
		ud.Owner = owner
		domains = append(domains, ud)
	}
	return domains, nil
}

func (d *acmednsdb) Close() {
	d.DB.Close()
}

func (d *acmednsdb) GetBackend() *sql.DB {
	return d.DB
}

func (d *acmednsdb) SetBackend(backend *sql.DB) {
	d.DB = backend
}

func sanitizeFQDN(fqdn string) string {
	fqdn = strings.ToLower(strings.TrimSpace(fqdn))
	fqdn = strings.TrimSuffix(fqdn, ".")
	return fqdn
}
