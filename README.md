# httpreq

DNS TXT record management server for ACME DNS-01 challenges, compatible with [lego httpreq](https://go-acme.github.io/lego/dns/httpreq/) provider.

**Online Service:** [https://dnsall.com](https://dnsall.com) — free to use, no deployment required.

## Features

- **CNAME delegation** — delegate `_acme-challenge` records via CNAME, no need to give CA clients access to your primary DNS
- **Multi-user** — each user gets isolated domains with unique nanoid subdomains
- **API Key auth** — separate API keys for httpreq Basic Auth (not your login password)
- **Web dashboard** — manage domains, view CNAME records, check active TXT records
- **Embedded SPA** — single binary includes the web UI, no separate frontend deployment
- **SQLite / PostgreSQL** — lightweight by default, scale when needed
- **Multi-arch Docker** — `linux/amd64` + `linux/arm64`

## How It Works

```
┌─────────────┐     CNAME      ┌──────────────────────┐
│ _acme-challenge.example.com ──────▶ r0hc4bc6.s.dnsall.com │
└─────────────┘                └──────────┬───────────┘
                                          │
┌─────────┐  POST /present   ┌───────────▼───────────┐
│  lego   │ ────────────────▶ │       httpreq         │
│ httpreq │  Basic Auth       │  stores TXT record    │
└─────────┘  (user:api_key)   │  serves DNS queries   │
                              └───────────────────────┘
```

1. User registers, adds domain `example.com` in the dashboard
2. System assigns a nanoid subdomain (e.g. `r0hc4bc6`) and shows the CNAME record to configure
3. User sets CNAME: `_acme-challenge.example.com → r0hc4bc6.s.dnsall.com`
4. lego calls `/present` with the challenge token via httpreq
5. httpreq stores the TXT record, DNS server responds to CA queries
6. Certificate issued, lego calls `/cleanup`

## Quick Start

### Docker Compose

```yaml
services:
  httpreq:
    image: zzci/httpreq:latest
    restart: unless-stopped
    ports:
      - "53:53"
      - "53:53/udp"
      - "3000:3000"
    volumes:
      - ./data:/app/data
```

Create `data/config.cfg`:

```toml
[general]
listen = "0.0.0.0:53"
protocol = "both"
domain = "s.dnsall.com"
nsname = "s.dnsall.com"
nsadmin = "admin.dnsall.com"
records = [
    "s.dnsall.com. A 1.2.3.4",
    "s.dnsall.com. NS s.dnsall.com.",
]

[database]
engine = "sqlite"
connection = "data/db/httpreq.db"

[api]
api_domain = "api.dnsall.com"
ip = "0.0.0.0"
port = "3000"
tls = "none"
jwt_secret = "change-me-to-a-random-string"
admin_key = "change-me-to-a-secret-key"

[logconfig]
loglevel = "info"
logtype = "stdout"
logformat = "json"
```

```bash
docker compose up -d
```

### Binary

```bash
CGO_ENABLED=0 go build -o httpreq .
./httpreq -c data/config.cfg
```

## Usage with lego

```bash
LEGO_DISABLE_CNAME_SUPPORT=true \
HTTPREQ_ENDPOINT=https://api.dnsall.com \
HTTPREQ_USERNAME=myuser \
HTTPREQ_PASSWORD=<api_key> \
lego --dns httpreq \
  --dns.propagation-disable-ans \
  --domains example.com \
  --domains "*.example.com" \
  --email admin@example.com \
  --accept-tos run
```

### Traefik

```yaml
# traefik.yml
certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@example.com
      storage: /data/ssl/acme.json
      dnsChallenge:
        provider: httpreq
        propagation:
          disableChecks: true
```

```yaml
# docker-compose.yml
services:
  traefik:
    environment:
      LEGO_DISABLE_CNAME_SUPPORT: "true"
      HTTPREQ_ENDPOINT: "https://api.dnsall.com"
      HTTPREQ_USERNAME: "myuser"
      HTTPREQ_PASSWORD: "<api_key>"
```

## API

### Public

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/register` | Register `{username, password}` → `{token, api_key}` |
| POST | `/api/login` | Login `{username, password}` → `{token, api_key}` |
| GET | `/api/info` | Server info `{base_domain, api_domain}` |

### User (JWT Bearer)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/profile` | Get username and api_key |
| POST | `/api/profile/regenerate-key` | Regenerate API key |
| GET | `/api/domains` | List user's domains |
| POST | `/api/domains` | Add domain `{domain}` |
| DELETE | `/api/domains/:domain` | Remove domain |
| GET | `/api/records` | List user's active TXT records |

### httpreq (Basic Auth: username + api_key)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/present` | Store TXT record `{fqdn, value}` |
| POST | `/cleanup` | Remove TXT record `{fqdn, value}` |

### Admin (X-Admin-Key header)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/users` | List all users |
| POST | `/admin/users` | Create user |
| DELETE | `/admin/users/:id` | Delete user and domains |
| GET | `/admin/domains` | List all domains |
| POST | `/admin/domains` | Add domain for user |
| DELETE | `/admin/domains/:domain` | Remove domain |
| GET | `/admin/records` | List all TXT records |

## Configuration

| Section | Key | Description | Default |
|---------|-----|-------------|---------|
| general | domain | DNS base domain for nanoid subdomains | required |
| general | nsname | NS record name | required |
| general | listen | DNS listen address | `127.0.0.1:53` |
| general | protocol | `both`, `udp`, `tcp` | `both` |
| database | engine | `sqlite` or `postgres` | `sqlite` |
| database | connection | DB path or connection string | `data/db/httpreq.db` |
| api | api_domain | API domain (for display) | falls back to general.domain |
| api | port | HTTP listen port | `443` |
| api | tls | `none`, `cert`, `letsencrypt` | `none` |
| api | jwt_secret | JWT signing key | auto-generated |
| api | admin_key | Admin API key | empty (disabled) |

## Development

```bash
# Backend
go test ./pkg/...
go build -o httpreq .

# Frontend
cd web
npm install
npm run dev    # dev server with proxy to localhost:3000

# Build frontend + embed into binary
cd web && npx vite build && cd ..
CGO_ENABLED=0 go build -o httpreq .
```

## License

MIT
