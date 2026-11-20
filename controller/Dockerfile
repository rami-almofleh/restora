# Build context: repository root.  docker build -f controller/Dockerfile .

# Kopia stores backups encrypted, deduplicated and incrementally. The data mover pods run this image.
FROM kopia/kopia:0.23.1 AS kopia

FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ internal/
COPY controller/ controller/
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X restora.local/restora/internal/version.Version=${VERSION}" \
    -o /out/restora-controller ./controller

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/restora-controller /restora-controller
COPY --from=kopia /usr/bin/kopia /usr/bin/kopia
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/restora-controller"]
