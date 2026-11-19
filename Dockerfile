# The single ledgerops binary (cmd/api: serve, migrate, version) on a minimal,
# non-root base. Every base is pinned by digest. The Go stage runs on the build
# platform and cross-compiles for the target, so a multi-arch build never
# emulates the compiler. The final stage has no RUN: it only copies.
FROM --platform=$BUILDPLATFORM node:24-slim@sha256:dc26ae92c9fb15cf6b1d63e20f0944b2efe0f115b20e00be48fb23fceb061c54 AS console
WORKDIR /src/web/console
COPY web/console/package.json web/console/package-lock.json ./
RUN npm ci
COPY web/console/ ./
RUN npm run build

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
# The service serves /console from web/console/dist relative to its working
# directory, so the built bundle goes there and the image runs from /.
COPY --from=console /src/web/console/dist /web/console/dist
WORKDIR /
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ledgerops"]
