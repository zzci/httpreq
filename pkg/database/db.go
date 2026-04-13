package database

import (
	"database/sql"
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

	"github.com/zzci/httpdns/pkg/httpdns"
)

type acmednsdb struct {
	DB     *sql.DB
	Mutex  sync.Mutex
	Logger *zap.SugaredLogger
	Config *httpdns.Config
}

// DBVersion shows the database version this code uses.
var DBVersion = 3

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
		created_at INT NOT NULL
	);`

var usersTablePG = `
	CREATE TABLE IF NOT EXISTS users(
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
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

// getSQLiteStmt replaces all PostgreSQL prepared statement placeholders (eg. $1, $2) with SQLite variant "?"
func getSQLiteStmt(s string) string {
	re, _ := regexp.Compile(`\$[0-9]+`)
	return re.ReplaceAllString(s, "?")
}

func Init(config *httpdns.Config, logger *zap.SugaredLogger) (httpdns.DB, error) {
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
	} else {
		_, _ = d.DB.Exec(txtTablePG)
		_, _ = d.DB.Exec(usersTablePG)
		_, _ = d.DB.Exec(userDomainsTablePG)
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
	// Tables are created in Init, just bump version
	_, err := d.DB.Exec("UPDATE acmedns SET Value='3' WHERE Name='db_version'")
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

func (d *acmednsdb) ListTXTRecords() ([]httpdns.TXTRecord, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var records []httpdns.TXTRecord
	rows, err := d.DB.Query("SELECT Domain, Value, LastUpdate FROM txt ORDER BY LastUpdate DESC")
	if err != nil {
		return records, err
	}
	defer rows.Close()
	for rows.Next() {
		var r httpdns.TXTRecord
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

func (d *acmednsdb) ListTXTRecordsByDomains(domains []string) ([]httpdns.TXTRecord, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	if len(domains) == 0 {
		return nil, nil
	}
	var records []httpdns.TXTRecord
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
		var r httpdns.TXTRecord
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

func (d *acmednsdb) CreateUser(username, passwordHash string) (httpdns.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	now := time.Now().Unix()
	insSQL := `INSERT INTO users (username, password_hash, created_at) VALUES ($1, $2, $3)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	result, err := d.DB.Exec(insSQL, username, passwordHash, now)
	if err != nil {
		return httpdns.User{}, fmt.Errorf("failed to create user: %w", err)
	}
	id, _ := result.LastInsertId()
	return httpdns.User{ID: id, Username: username, PasswordHash: passwordHash, CreatedAt: now}, nil
}

func (d *acmednsdb) GetUserByUsername(username string) (httpdns.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var u httpdns.User
	getSQL := `SELECT id, username, password_hash, created_at FROM users WHERE username=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (d *acmednsdb) GetUserByID(id int64) (httpdns.User, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var u httpdns.User
	getSQL := `SELECT id, username, password_hash, created_at FROM users WHERE id=$1`
	if d.Config.Database.Engine == "sqlite" {
		getSQL = getSQLiteStmt(getSQL)
	}
	err := d.DB.QueryRow(getSQL, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

// --- Domain methods ---

func (d *acmednsdb) AddUserDomain(userID int64, domain string) (httpdns.UserDomain, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	domain = strings.ToLower(strings.TrimSpace(domain))
	subdomain := httpdns.GenerateSubdomain()
	insSQL := `INSERT INTO user_domains (user_id, domain, subdomain) VALUES ($1, $2, $3)`
	if d.Config.Database.Engine == "sqlite" {
		insSQL = getSQLiteStmt(insSQL)
	}
	_, err := d.DB.Exec(insSQL, userID, domain, subdomain)
	if err != nil {
		return httpdns.UserDomain{}, fmt.Errorf("failed to add domain: %w", err)
	}
	return httpdns.UserDomain{
		Domain:    domain,
		Subdomain: subdomain,
	}, nil
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

func (d *acmednsdb) GetUserDomains(userID int64) ([]httpdns.UserDomain, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	var domains []httpdns.UserDomain
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
		var ud httpdns.UserDomain
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
