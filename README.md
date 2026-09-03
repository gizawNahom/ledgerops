# ledgerops

Double-entry ledger service. Go · PostgreSQL 16 · TypeScript SPA console.

Modular monolith, ports-and-adapters, with a pure functional domain core
(no I/O in `internal/domain`). See `docs/product/architecture/brief.md` for
the full architecture SSOT.

## Stack

- **API**: Go, `net/http` + chi (`cmd/api`)
- **Domain**: pure functional core — `Money`, `Entry`, `Transaction`, posting rules (`internal/domain`)
- **Storage**: PostgreSQL 16, migrations in `internal/adapters/postgres/migrations`
- **Console**: TypeScript SPA (`web/console`) for verifying books and inspecting entries

## Running locally

```bash
make up         # builds and starts postgres, migrate, app (http://localhost:8080)
make down       # tears the stack down
```

`make up` runs `docker compose up -d --build`; migrations apply automatically
before the app starts. The app authenticates with a Bearer operator key
(`demo-operator-key` in the compose default).

Optional observability sidecars (Prometheus on `:9090`, Grafana on `:3000`)
are included in `docker-compose.yml` for demoing metrics.

## Project layout

```
cmd/api/              entrypoint
internal/domain/      pure domain core (no I/O)
internal/app/         use cases (PostTransfer, VerifyBooks, ...)
internal/adapters/    http + postgres adapters
web/console/          operator console SPA
docs/                 architecture, feature, and product docs (nWave-managed)
```

## Development

This project follows the nWave methodology — see `CLAUDE.md` for wave
conventions. Domain core changes should stay pure; effects belong in
`internal/adapters`.
