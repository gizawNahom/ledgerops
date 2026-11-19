---
title: Compare
description: When to choose ledgerops, when not to, and which open-source ledgers suit other needs better.
---

Several open-source ledgers make different trade-offs. This page says where
ledgerops fits and where another one may suit you better.

## Choose ledgerops if

- You already run PostgreSQL.
- You want the ledger as a separate service that your applications call over
  HTTP.
- You care most about being able to check that the books balance, and about the
  service's own database role being unable to rewrite entries.
- Your use case is wallets, credits or loyalty points, where a transfer moves
  value between two accounts.

## It is not

- **An accounting package.** There are no chart-of-accounts templates, tax
  reporting or invoicing.
- **A payments processor.** It records movement and never touches real payment
  rails.
- **A hosted service.** You run it yourself.

## If you need something else

| If you need… | Consider | Why |
|---|---|---|
| Very high throughput | [TigerBeetle](https://github.com/tigerbeetle/tigerbeetle) | A purpose-built financial transactions database with very high throughput. It's a database rather than a service, so you add the API layer. |
| Ledger writes in the same database transaction as your application's data | [pgledger](https://github.com/pgr0ss/pgledger) | A ledger made of SQL functions inside your own PostgreSQL, with no separate service. |
| Programmable transaction logic and a full fintech platform | [Formance Ledger](https://github.com/formancehq/ledger) | Its own scripting language (Numscript), and it runs as a replicated cluster. |
| Holds, multi-currency, reconciliation and a hosted option today | [Blnk](https://github.com/blnkfinance/blnk) | The closest in shape to ledgerops (a Go and PostgreSQL service), with a wider feature set today. |

This comparison is based on each project's public documentation as of October
2026. Check their current state before you decide.

## What ledgerops emphasises

- **A built-in books check.** The trial-balance verdict names any account whose
  stored balance has drifted from its entries.
- **Append-only history enforced by database permissions** for the service's
  role, not by convention.
- **An operator console** for checking the books.
- **Tests that exercise the guarantees** under concurrent requests and a killed
  process.

## What ledgerops lacks today

Most of the "Consider" rows above exist because ledgerops doesn't do that yet:
one currency, two-leg transfers only, no holds, no webhooks, a single very busy
account as the first performance limit, and not yet run in production. The
full list is on
[Status, limits and operating](../status/).
