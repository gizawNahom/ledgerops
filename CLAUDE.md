# ledgerops

Double-entry ledger service. Go 1.23+ · PostgreSQL 16 · TypeScript SPA console.
Modular monolith, ports-and-adapters, pure domain core.

Architecture SSOT: `docs/product/architecture/brief.md`
Feature narrative: `docs/feature/ledger-core/feature-delta.md`

## Development Paradigm

This project follows the **object-oriented** paradigm. Use
@nw-software-crafter for implementation.

Go routing decision; the idiom is procedural-with-interfaces (DDD-10).

## Mutation Testing Strategy

This project uses **nightly-delta** mutation testing. CI runs on files modified
each day. NOT run during feature delivery.
