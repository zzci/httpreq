FROM node:22-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npx vite build

FROM golang:1.25-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o httpdns .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=backend /build/httpdns .
VOLUME ["/data"]
ENTRYPOINT ["./httpdns"]
EXPOSE 53 53/udp 3000
