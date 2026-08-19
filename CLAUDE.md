# ledgerops

Double-entry ledger service. Go 1.23+ · PostgreSQL 16 · TypeScript SPA console.
Modular monolith, ports-and-adapters, pure domain core.

Architecture SSOT: `docs/product/architecture/brief.md`
Feature narrative: `docs/feature/ledger-core/feature-delta.md`

## nWave Wave Work

Wave skills (`nw-discuss`, `nw-design`, `nw-devops`, `nw-distill`,
`nw-deliver`) are domain knowledge for their agent, not instructions for the
session. Dispatch the owning agent — never author or edit wave artifacts
directly, including amendments to finished waves. Agent definitions carry
mandates the skills omit. Ask before making any exception.

## Development Paradigm

This project follows the **functional programming** paradigm. Use
@nw-functional-software-crafter for implementation.

Hexagonal DDD with a functional domain: pure core, effect shell, immutable
types (DDD-16, supersedes DDD-10). See
`docs/product/architecture/adr-007-functional-domain-core.md`.

## Mutation Testing Strategy

This project uses **nightly-delta** mutation testing. CI runs on files modified
each day. NOT run during feature delivery.
