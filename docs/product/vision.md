# Product Vision — ledgerops

## What this is

A double-entry ledger service that moves value between accounts correctly, and can prove
it did. API-first, with an internal operations console for oversight.

Value is generic: money, points, credits. The ledger does not care what a unit represents,
only that units are conserved.

## Who it is for

Systems that need to hold and move balances but should not implement a ledger themselves —
digital wallets, freelance marketplaces, credit and loyalty systems. The consumer is the
engineer integrating it, not an end user.

## The bet

Most teams that need balances start with a `balance` column and two UPDATE statements.
That works until it doesn't, and when it fails it fails silently: money is created or
destroyed, and nobody can reconstruct what happened. Retrofitting a real ledger onto
production data that is already wrong is far harder than starting with one.

ledgerops is the primitive you reach for before that happens.

## What it is not

- Not an accounting package. No chart of accounts templates, tax reporting, or invoicing.
- Not a payments processor. It records movement; it does not touch real payment rails.
- Not a customer-facing product. The console is for operators.

## Success looks like

An engineer can post a transfer through the API, see the resulting balances, and verify
that the entries behind them sum to zero — without reading the source code to believe it.

## Status

Greenfield. First feature is `ledger-core`. This vision was bootstrapped during the
DISCUSS wave for that feature and is founder-stated, not validated against users —
no DISCOVER wave has run.
