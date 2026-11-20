# Build context: repository root.  docker build -f api/Dockerfile .

# 1. Web interface
FROM node:24-alpine AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# 2. API binary with the web interface embedded
FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ internal/
COPY api/ api/
COPY ui/embed.go ui/embed.go
COPY --from=ui /src/ui/dist ui/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X restora.local/restora/internal/version.Version=${VERSION}" \
    -o /out/restora-api ./api

# 3. Runtime image
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/restora-api /restora-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/restora-api"]
