---
title: Quickstart
description: Run ledgerops locally, move value between two accounts, and check the books.
---

Run ledgerops locally, move value between two accounts, and check the books.

## Before you start

You need Git, `make`, Docker with Compose, and `curl`. `make up` publishes
ports 8080, 5432, 9090 and 3000, so free them first. Go 1.25 is only needed for
`race-02` and `race-03` and for developing ledgerops; Node is only needed for
developing the console.

## 1. Get the code, build and start it

```bash
git clone https://github.com/gizawNahom/ledgerops
cd ledgerops
make up
```

This builds the service from source and starts PostgreSQL, the migrations, the
API on `:8080`, Prometheus and Grafana. The first build takes a while.

## 2. Run the first demo

```bash
make demo-01
```

Opens a `treasury` system account (the source of value), `alice-01` and
`bob-01`. Funds `alice-01` with 100.00 from `treasury`, then moves 50.00 to
`bob-01`. Each transfer is recorded as two legs, one per account, that sum to
zero.

## 3. Try the guarantees

The demo stack comes with two local keys. Set them once:

```bash
API=http://localhost:8080
TENANT="Authorization: Bearer demo-tenant-key"
OP="Authorization: Bearer demo-operator-key"
```

**Retry the transfer demo-01 posted.** Sending the same `Idempotency-Key`
again returns the original transaction instead of posting twice:

```bash
curl -s -w '\nHTTP %{http_code}\n' -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: demo-01-transfer' \
  -H 'Content-Type: application/json' -d '{"from":"alice-01","to":"bob-01","amount":"50.00"}'
```

Expect `HTTP 200` and the `transaction_id` that demo-01 printed.

**Overdraw an account.** The refusal names the shortfall:

```bash
curl -s -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: alice-overdraws' \
  -H 'Content-Type: application/json' -d '{"from":"alice-01","to":"bob-01","amount":"500.00"}'
# 422 {"account_id":"alice-01","available":"50.00","error":"insufficient_funds","requested":"500.00"}
```

**Read a balance and the entries behind it:**

```bash
curl -s $API/accounts/alice-01 -H "$TENANT"
# {"account_id":"alice-01","balance":"50.00","type":"wallet"}
curl -s $API/accounts/alice-01/entries -H "$TENANT"
```

Each entry comes with a running balance.

## 4. Check the books

The books check takes the operator key:

```bash
curl -s $API/health/trial-balance -H "$OP"
# {"verdict":"Books balance: YES","imbalance_minor":0,…}
```

With `?tenant_id=` it checks one tenant. Without it, it checks every tenant.

## 5. Stop it

```bash
make down
```

`make down` stops the stack **and deletes its data volume**. The keys and
passwords in `docker-compose.yml` are local demo values. Don't reuse them
anywhere real.

## Start from nothing instead

The steps above use the demo tenant. To provision your own tenant, open
accounts, fund them and move value, follow the
[tenant-from-scratch guide](https://github.com/gizawNahom/ledgerops/blob/main/docs/quickstart.md).
Run `make up` first.

## More demos

| Command | What it shows |
|---|---|
| `make demo-02` | An overdraft is refused with `422`, naming the shortfall |
| `make demo-03` | A retried request returns the same transaction |
| `make demo-04` | A fresh tenant goes from provisioned to first transfer |
| `make chaos-01` | The API is killed mid-write; the retry lands exactly once |
| `make race-02` | No balance goes negative under concurrent spenders |
| `make race-03` | Concurrent duplicate submissions store one transaction |

`demo-04` provisions a fixed tenant name, so a second run fails with
`tenant_already_exists`. Run `make down` first. `race-02` and `race-03` also
need Go.

## Reference

| Route | Credential |
|---|---|
| `POST /tenants` | operator |
| `POST /tenant-links`, `DELETE /tenant-links/{link_id}` | operator |
| `GET /health/trial-balance`, `GET /console/verdict` | operator |
| `POST /accounts`, `GET /accounts/{id}` | tenant |
| `POST /transfers` (a `counterparty_alias` body sends cross-tenant) | tenant |
| `POST /counterparties` | tenant |
| `GET /accounts/{id}/entries` | tenant, or operator (all tenants) |
| `GET /transfers/{transfer_id}` | either party's tenant key, or operator |
| `GET /metrics`, console static assets | none |

Credentials are sent as `Authorization: Bearer <key>`. The operator key is
refused on the routes that open accounts, read an account, post transfers and
register counterparties. A wrong or missing key gets
`401 {"error":"unidentified_caller"}`. There is no OpenAPI spec yet.

## Pull the image

A signed release-candidate image is published
(`ghcr.io/gizawnahom/ledgerops:0.2.0-rc.1`). A pull-and-run guide will be
added once it has been run end to end on a clean machine. Until then, build
from source as above.
