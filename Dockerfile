# One image: the Go API serving the built SPA, plus the migrate and onboard commands.
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go
WORKDIR /src
COPY backend-go/go.mod backend-go/go.sum ./
RUN go mod download
COPY backend-go/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/migrate ./cmd/onboard

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/ /app/
COPY --from=web /web/dist /app/web
ENV HTTP_ADDR=:8080 STORAGE_DIR=/data WEB_DIR=/app/web TRUST_PROXY=true
EXPOSE 8080
ENTRYPOINT ["/app/api"]
