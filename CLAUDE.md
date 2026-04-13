# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

acme-dns is a simplified DNS server with a RESTful HTTP API to handle ACME DNS-01 challenges. It allows delegating `_acme-challenge` TXT records via CNAME so that ACME certificates can be issued without giving the CA client access to the primary DNS zone. Written in Go, it supports SQLite and PostgreSQL backends.

## Build & Run

```bash
# Build
CGO_ENABLED=0 go build

# Run (uses ./config.cfg by default, or specify with -c)
./acme-dns -c /path/to/config.cfg
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run a single package's tests
go test ./pkg/api/
go test ./pkg/database/
go test ./pkg/nameserver/
go test ./pkg/acmedns/

# Run a specific test
go test -run TestApiRegister ./pkg/api/

# Coverage
go test -cover ./...
```

Note: `TestFileCheckPermissionDenied` in `pkg/acmedns` fails when running as root (e.g., in containers).

## Architecture

### Startup Flow (main.go)

1. Read TOML config (`config.cfg`) via `acmedns.ReadConfig`
2. Initialize database via `database.Init`
3. Initialize HTTP API via `api.Init`
4. Initialize and start DNS server(s) via `nameserver.InitAndStart`
5. Start API server in a goroutine; block on error channel

### Package Structure (`pkg/`)

- **acmedns** — Core types (`AcmeDnsConfig`, `ACMETxt`, `Cidrslice`), interfaces (`AcmednsDB`, `AcmednsNS`), config parsing, logging setup, and utility functions. This is the shared foundation that other packages depend on.
- **database** — Implements `AcmednsDB` interface. Handles SQLite/PostgreSQL via `database/sql`. Uses mutex for all DB operations. Manages schema creation and versioned upgrades. PostgreSQL uses `$1` placeholders; SQLite uses `?` (converted via `getSQLiteStmt`).
- **api** — HTTP API using `httprouter`. Three endpoints: `POST /register`, `POST /update` (auth-protected), `GET /health`. Auth via `X-Api-User` (UUID) and `X-Api-Key` headers with bcrypt password verification. Supports TLS via Let's Encrypt (certmagic), custom certs, or none. IP-based access control via `AllowFrom` CIDR lists.
- **nameserver** — DNS server using `miekg/dns`. Handles DNS queries, serves static records from config, and dynamically resolves TXT records from the database. Supports UDP/TCP with IPv4/IPv6. Also handles its own ACME challenges (`_acme-challenge.<domain>`) for API TLS.

### Key Interfaces (pkg/acmedns/interfaces.go)

- `AcmednsDB` — Database operations: `Register`, `GetByUsername`, `GetTXTForDomain`, `Update`
- `AcmednsNS` — DNS server lifecycle: `Start`, `SetOwnAuthKey`, `ParseRecords`

### Configuration

TOML format (`config.cfg`) with sections: `[general]` (DNS listen/protocol/domain), `[database]` (engine/connection), `[api]` (HTTP listen/TLS/CORS), `[logconfig]` (level/format/output).

### Database Schema

Three tables: `acmedns` (key-value metadata including db_version), `records` (user accounts with UUID username, bcrypt password, subdomain, AllowFrom CIDR JSON), `txt` (TXT record values per subdomain, two rows per registration for round-robin updates).

## Dependencies

- `miekg/dns` — DNS protocol library
- `julienschmidt/httprouter` — HTTP router
- `caddyserver/certmagic` — Automatic HTTPS via ACME
- `glebarez/go-sqlite` — Pure Go SQLite driver (CGO_ENABLED=0 compatible)
- `lib/pq` — PostgreSQL driver
- `BurntSushi/toml` — Config parsing
- `rs/cors` — CORS middleware
- `go.uber.org/zap` — Structured logging
- `gavv/httpexpect` — HTTP testing (test only)
- `DATA-DOG/go-sqlmock` — SQL mocking (test only)
