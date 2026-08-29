# Builds the single ledgerops binary (cmd/api) for the compose demo path.
# Two stages: compile with the full Go toolchain, ship on a minimal base.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ledgerops-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/ledgerops-api /usr/local/bin/ledgerops-api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ledgerops-api"]
