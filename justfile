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

# Start dev server (frontend + backend on same domain via nsl path prefix)
dev: build
    #!/usr/bin/env bash
    cd web && nsl run -n httpreq npx vite &
    FRONTEND_PID=$!
    nsl run -n httpreq:/api dist/httpreq -c config.cfg &
    BACKEND_PID=$!
    trap "kill $FRONTEND_PID $BACKEND_PID 2>/dev/null" EXIT
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
