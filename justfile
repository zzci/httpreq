set dotenv-load

# Build frontend
web-build:
    cd web && npx vite build

# Build Go binary (with embedded frontend)
build: web-build
    CGO_ENABLED=0 go build -o httpdns .

# Run all tests
test:
    go test ./pkg/...

# Start dev server (backend + frontend via nsl)
dev:
    #!/usr/bin/env bash
    # Start backend
    nsl run -n httpdns -p 3000 ./httpdns -c config.cfg &
    BACKEND_PID=$!
    # Start frontend dev server
    cd web && nsl run -n httpdns-dev npx vite --port NSL_PORT &
    FRONTEND_PID=$!
    trap "kill $BACKEND_PID $FRONTEND_PID 2>/dev/null" EXIT
    wait

# Start backend only via nsl
serve:
    nsl run -n httpdns -p 3000 ./httpdns -c config.cfg

# Clean build artifacts and data
clean:
    rm -f httpdns
    rm -rf web/dist

# Format and lint
lint:
    go vet ./...
    cd web && npx tsc --noEmit

# Tag and push a release
release version:
    git tag v{{version}}
    git push origin v{{version}}
