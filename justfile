set dotenv-load

# Build frontend
web-build:
    cd web && npx vite build

# Build Go binary (with embedded frontend)
build: web-build
    mkdir -p dist
    CGO_ENABLED=0 go build -o dist/httpreq .

# Run all tests
test:
    go test ./pkg/...

# Start dev server (backend + frontend via nsl)
dev: build
    #!/usr/bin/env bash
    nsl run -n httpreq dist/httpreq -c config.cfg &
    BACKEND_PID=$!
    cd web && nsl run -n httpreq-dev npx vite --port NSL_PORT &
    FRONTEND_PID=$!
    trap "kill $BACKEND_PID $FRONTEND_PID 2>/dev/null" EXIT
    wait

# Start backend only via nsl
serve: build
    nsl run -n httpreq dist/httpreq -c config.cfg

# Clean build artifacts and data
clean:
    rm -rf dist
    rm -rf web/dist

# Format and lint
lint:
    go vet ./...
    cd web && npx tsc --noEmit

# Tag and push a release
release version:
    git tag v{{version}}
    git push origin v{{version}}
