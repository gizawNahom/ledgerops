# Quickstart: a tenant from scratch

The [README quickstart](../README.md#quickstart) uses the demo stack's
pre-seeded tenant. This walkthrough starts from nothing instead: provision a
tenant, open accounts, fund one, and move value — the same steps an
integrating service takes.

Start the stack first:

```bash
make up
```

## 1. Provision a tenant

Provisioning needs the operator key and returns the new tenant's id and its
own key. Every account and transfer call after this uses the tenant key; the
operator key is never accepted on those routes.

```bash
API=http://localhost:8080
OP="Authorization: Bearer demo-operator-key"

RESP=$(curl -s -X POST $API/tenants -H "$OP" \
  -H 'Content-Type: application/json' -d '{"name":"acme"}')
TENANT_ID=$(echo "$RESP" | grep -o '"tenant_id":"[^"]*"' | cut -d'"' -f4)
TENANT="Authorization: Bearer $(echo "$RESP" | grep -o '"tenant_key":"[^"]*"' | cut -d'"' -f4)"
```

Tenant names are unique, so running this twice fails with
`tenant_already_exists`. Use a different name, or `make down` to start over.

## 2. Open accounts

There is no "mint" endpoint: value enters the ledger through a `system`
account, which may go negative. `wallet` accounts never can.

```bash
for acct in '"treasury","type":"system"' '"alice","type":"wallet"' '"bob","type":"wallet"'; do
  curl -s -X POST $API/accounts -H "$TENANT" \
    -H 'Content-Type: application/json' -d "{\"account_id\":$acct}"; echo
done
```

## 3. Fund a wallet, then move value

Every transfer needs an `Idempotency-Key`. Amounts are decimal strings.

```bash
curl -s -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: fund-alice' \
  -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice","amount":"100.00"}'
curl -s -X POST $API/transfers -H "$TENANT" -H 'Idempotency-Key: alice-pays-bob' \
  -H 'Content-Type: application/json' -d '{"from":"alice","to":"bob","amount":"30.00"}'
# → 201 {"legs":[{"account":"alice","amount":"-30.00"},{"account":"bob","amount":"30.00"}],"transaction_id":"txn_…"}
```

## 4. Check the result

```bash
curl -s $API/accounts/alice -H "$TENANT"          # {"account_id":"alice","balance":"70.00","type":"wallet"}
curl -s $API/accounts/alice/entries -H "$TENANT"  # each entry, with a running balance
curl -s "$API/health/trial-balance?tenant_id=$TENANT_ID" -H "$OP"
# {"verdict":"Books balance: YES","imbalance_minor":0,…}
```

The books check takes the operator key. With `tenant_id` it checks that
tenant's accounts against its entries; without it, it checks every tenant's.

`make down` stops the stack and deletes its data volume.
