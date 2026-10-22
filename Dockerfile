# The single ledgerops binary (cmd/api: serve, migrate, version) on a minimal,
# non-root base. Every base is pinned by digest. The Go stage runs on the build
# platform and cross-compiles for the target, so a multi-arch build never
# emulates the compiler. The final stage has no RUN: it only copies.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# console_embed is inert until the console bundle is embedded (slice 05).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -tags console_embed -o /out/ledgerops ./cmd/api

FROM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/ledgerops /usr/local/bin/ledgerops
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ledgerops"]
