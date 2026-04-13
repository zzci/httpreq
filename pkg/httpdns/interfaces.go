package httpdns

import (
	"database/sql"
)

type DB interface {
	// TXT record management
	PresentTXT(fqdn, value string) error
	CleanupTXT(fqdn, value string) error
	GetTXTForDomain(string) ([]string, error)
	ListTXTRecords() ([]TXTRecord, error)
	ListTXTRecordsByDomains(domains []string) ([]TXTRecord, error)

	// User management
	CreateUser(username, passwordHash string) (User, error)
	GetUserByUsername(username string) (User, error)
	GetUserByID(id int64) (User, error)

	// Domain management
	AddUserDomain(userID int64, domain string) (UserDomain, error)
	RemoveUserDomain(userID int64, domain string) error
	GetUserDomains(userID int64) ([]UserDomain, error)
	GetSubdomainByUserDomain(userID int64, domain string) (string, error)

	GetBackend() *sql.DB
	SetBackend(*sql.DB)
	Close()
}

type NS interface {
	Start(errorChannel chan error)
	SetOwnAuthKey(key string)
	SetNotifyStartedFunc(func())
	ParseRecords()
}
