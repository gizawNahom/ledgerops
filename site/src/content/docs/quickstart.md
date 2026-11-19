---
title: Quickstart
description: Run ledgerops from its Docker image, move value between two accounts, and check the books.
---

Run ledgerops from its Docker image, move value between two accounts, and check the books.

## Before you start

You need Docker and `curl`. Port 8080 must be free. This guide runs PostgreSQL
16 in a container to keep it self-contained. In real use, point ledgerops at
your own PostgreSQL 16.

## 1. Pull the image and start PostgreSQL

```bash
export LEDGEROPS_IMAGE=ghcr.io/gizawnahom/ledgerops:0.2.0-rc.2
docker pull $LEDGEROPS_IMAGE

docker network create lo
docker run -d --name pg --network lo \
  -e POSTGRES_PASSWORD=admin -e POSTGRES_DB=ledgerops postgres:16

# wait until PostgreSQL accepts connections
until docker exec pg pg_isready -h 127.0.0.1 -U postgres -d ledgerops; do sleep 1; done
```

The image is a release candidate published with a cosign signature. There is
no `latest` tag, so you always name a version. The passwords and keys in this guide are throwaway
values. Choose your own for anything real.

## 2. Migrate the database

```bash
docker run --rm --network lo \
  -e LEDGEROPS_MIGRATE_DSN='postgres://postgres:admin@pg:5432/ledgerops?sslmode=disable' \
  -e LEDGEROPS_APP_PASSWORD=apppw \
  $LEDGEROPS_IMAGE migrate
```

Expect `schema at version 6` and `app role password set`. The privileged DSN
is only for this step. The service never holds it. Without a password,
`migrate` exits non-zero and names `LEDGEROPS_APP_PASSWORD` and
`LEDGEROPS_APP_PASSWORD_FILE`.

## 3. Start the service

```bash
docker run -d --name lo --network lo -p 8080:8080 \
  -e LEDGEROPS_APP_DSN='postgres://ledgerops_app:apppw@pg:5432/ledgerops?sslmode=disable' \
  -e LEDGEROPS_OPERATOR_KEY=opkey \
  $LEDGEROPS_IMAGE

until curl -sf localhost:8080/health; do sleep 1; done
# {"status":"ok"}
```

The app password in the DSN must match the one you gave `migrate`. If it
doesn't, the service refuses to start and says authentication failed.

## 4. Provision a tenant and move value

Set the keys once. Provisioning takes the operator key and returns the new
tenant's own key. Every account and transfer call after that uses the tenant
key.

```bash
API=http://localhost:8080
OP="Authorization: Bearer opkey"

RESP=$(curl -s -X POST $API/tenants -H "$OP" \
  -H 'Content-Type: application/json' -d '{"name":"acme"}')
TENANT_ID=$(echo "$RESP" | grep -o '"tenant_id":"[^"]*"' | cut -d'"' -f4)
TENANT="Authorization: Bearer $(echo "$RESP" | grep -o '"tenant_key":"[^"]*"' | cut -d'"' -f4)"
```

Value enters the ledger through a `system` account, which may go negative.
`wallet` accounts never can. Open one of each, fund `alice`, then pay `bob`:

```bash
for acct in '"treasury","type":"system"' '"alice","type":"wallet"' '"bob","type":"wallet"'; do
  curl -s -X POST $API/accounts -H "$TENANT" \
    -H 'Content-Type: application/json' -d "{\"account_id\":$acct}"; echo
done

curl -s -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: fund-alice' \
  -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice","amount":"100.00"}'
curl -s -w '\nHTTP %{http_code}\n' -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: alice-pays-bob' \
  -H 'Content-Type: application/json' -d '{"from":"alice","to":"bob","amount":"30.00"}'
# {"legs":[{"account":"alice","amount":"-30.00"},{"account":"bob","amount":"30.00"}],"transaction_id":"txn_…"}
# HTTP 201
```

Every transfer needs an `Idempotency-Key`. Amounts are decimal strings.

## 5. Try the guarantees

**Retry the transfer.** Sending the same `Idempotency-Key` again returns the
original transaction instead of posting twice:

```bash
curl -s -w '\nHTTP %{http_code}\n' -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: alice-pays-bob' \
  -H 'Content-Type: application/json' -d '{"from":"alice","to":"bob","amount":"30.00"}'
```

Expect `HTTP 200` and the same `transaction_id`.

**Overdraw an account.** The refusal names the shortfall:

```bash
curl -s -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: alice-overdraws' \
  -H 'Content-Type: application/json' -d '{"from":"alice","to":"bob","amount":"500.00"}'
# 422 {"account_id":"alice","available":"70.00","error":"insufficient_funds","requested":"500.00"}
```

**Read a balance and the entries behind it:**

```bash
curl -s $API/accounts/alice -H "$TENANT"
# {"account_id":"alice","balance":"70.00","type":"wallet"}
curl -s $API/accounts/alice/entries -H "$TENANT"
```

Each entry comes with a running balance.

## 6. Check the books

The books check takes the operator key:

```bash
curl -s "$API/health/trial-balance?tenant_id=$TENANT_ID" -H "$OP"
# {"verdict":"Books balance: YES","imbalance_minor":0,…}
```

With `tenant_id` it checks one tenant. Without it, it checks every tenant.
The operator console is at `http://localhost:8080/console`.

## 7. Stop it

```bash
docker rm -f lo pg && docker network rm lo
```

This deletes the database with the container. Tenant names are unique, so
running step 4 twice without cleaning up fails with `tenant_already_exists`.

## Build from source and run the demos

The demos use a stack that includes Prometheus and Grafana. It needs Git,
`make`, Docker with Compose, and `curl`, and it publishes ports 8080, 5432,
9090 and 3000. Go 1.25 is only needed for `race-02` and `race-03` and for
developing ledgerops. Node is only needed for developing the console.

```bash
git clone https://github.com/gizawNahom/ledgerops
cd ledgerops
make up
make demo-01
make down
```

`make up` builds the service from source and starts PostgreSQL, the
migrations, the API, Prometheus and Grafana. `make down` stops the stack
**and deletes its data volume**. The keys and passwords in
`docker-compose.yml` are local demo values. Don't reuse them anywhere real.

## More demos

These run against the source stack above, so run `make up` first.

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
