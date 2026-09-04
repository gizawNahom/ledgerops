SHELL := /bin/bash

.PHONY: up down wait-app demo-01 demo-02 demo-03 demo-04 chaos-01 race-02 race-03

# --- stack lifecycle -------------------------------------------------------

# `docker compose up` is itself idempotent against an already-running stack
# (compose reconciles instead of recreating unchanged services), so this
# target adds nothing beyond that except -d/--wait so callers get a shell
# back once the containers are up.
up:
	docker compose up -d --build

down:
	docker compose down -v

APP_URL := http://localhost:8080
# OPS-10/OPS-11 (step 02-05): DDD-23 Option C moved POST /accounts,
# POST /transfers, and GET /accounts/{id} -- every route demo-01..03 and
# chaos-01 exercise -- to a requireTenantKey-ONLY group; OperatorKey is never
# accepted there anymore. AUTH is repointed to the seeded demo tenant
# credential (docker-compose.yml's LEDGEROPS_DEMO_TENANT_KEY, resolved to
# tnt_legacy_seed) so every recipe body below stays byte-for-byte unchanged --
# only this variable's value changed. OPERATOR_AUTH keeps an explicit path to
# the unscoped platform credential for provisioning (POST /tenants,
# admin-only) and any console-compatibility scenario that still needs it.
AUTH := Authorization: Bearer demo-tenant-key
OPERATOR_AUTH := Authorization: Bearer demo-operator-key

# Polls the real driving port (not a container healthcheck) until the app
# answers, so the wait proves the same thing a caller's first request proves.
#
# Checks curl's own exit status, not just "did %{http_code} print digits":
# on connection-refused (e.g. mid-restart after chaos-01's SIGKILL) curl
# reports http_code "000" and exits non-zero, and "000" alone matches a
# bare digit-regex — so a digit-only check falsely declares the app
# answering during the exact window it isn't.
wait-app: up
	@echo "waiting for the app to accept requests..."
	@for i in $$(seq 1 60); do \
		code="$$(curl -s -o /dev/null -w '%{http_code}' -H "$(AUTH)" $(APP_URL)/accounts/__probe__)"; \
		status=$$?; \
		if [ "$$status" -eq 0 ] && [ "$$code" != "000" ]; then \
			echo "app is answering (HTTP $$code)"; exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "app never answered after 60s"; docker compose logs app; exit 1

# --- demo-01: US-1, post a transfer ----------------------------------------

demo-01: wait-app
	@echo "== demo-01: post a transfer =="
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"treasury","type":"system"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"alice-01","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"bob-01","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-01-fund' -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice-01","amount":"100.00"}'
	@echo "-- transferring 50.00 from alice-01 to bob-01, expect 201 with legs --"
	curl -s -w '\nHTTP %{http_code}\n' -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-01-transfer' -H 'Content-Type: application/json' -d '{"from":"alice-01","to":"bob-01","amount":"50.00"}'

# --- demo-02: US-2, reject insufficient funds -------------------------------

demo-02: wait-app
	@echo "== demo-02: reject insufficient funds =="
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"treasury","type":"system"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"alice-02","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"bob-02","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-02-fund' -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice-02","amount":"10.00"}'
	@echo "-- requesting 50.00 from a 10.00 balance, expect 422 insufficient_funds naming the shortfall --"
	curl -s -w '\nHTTP %{http_code}\n' -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-02-transfer' -H 'Content-Type: application/json' -d '{"from":"alice-02","to":"bob-02","amount":"50.00"}'

# --- demo-03: US-3, retry safely --------------------------------------------

demo-03: wait-app
	@echo "== demo-03: retry safely =="
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"treasury","type":"system"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"alice-03","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"bob-03","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-03-fund' -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice-03","amount":"100.00"}'
	@echo "-- first submission with key demo-03-retry --"
	curl -s -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-03-retry' -H 'Content-Type: application/json' -d '{"from":"alice-03","to":"bob-03","amount":"50.00"}' | tee /tmp/demo-03-first.json
	@echo ""
	@echo "-- identical retry with the same key, expect the identical transaction_id --"
	curl -s -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: demo-03-retry' -H 'Content-Type: application/json' -d '{"from":"alice-03","to":"bob-03","amount":"50.00"}' | tee /tmp/demo-03-second.json
	@echo ""
	@diff <(grep -o '"transaction_id":"[^"]*"' /tmp/demo-03-first.json) <(grep -o '"transaction_id":"[^"]*"' /tmp/demo-03-second.json) \
		&& echo "PASS: both answers name the same transaction" \
		|| (echo "FAIL: retry returned a different transaction"; exit 1)

# --- demo-04: US-4, provision a tenant, then post its first transfer -------
#
# Unlike demo-01..03 (which reuse the pre-seeded tnt_legacy_seed tenant via
# $(AUTH)), demo-04's whole point is proving a FRESH tenant's onboarding
# end-to-end: provision a brand-new tenant via POST /tenants using
# $(OPERATOR_AUTH) (admin-only route), capture the returned tenant_key from
# the response JSON, open an account and post that tenant's first transfer
# using THAT freshly-minted key (never $(AUTH)). Emits
# tenant_onboard_transfer_seconds on stdout in the same parseable
# `name=value` form KPI-5's demo_first_green_seconds uses (date +%s
# before/after wrapped around the whole provision-to-first-transfer
# sequence), so the CI job step can capture it the same way.

demo-04: wait-app
	@echo "== demo-04: provision a tenant, then post its first transfer =="
	@START=$$(date +%s); \
	curl -s -X POST $(APP_URL)/tenants -H "$(OPERATOR_AUTH)" -H 'Content-Type: application/json' -d '{"name":"demo-04-tenant"}' | tee /tmp/demo-04-tenant.json; \
	echo ""; \
	TENANT_KEY="$$(grep -o '"tenant_key":"[^"]*"' /tmp/demo-04-tenant.json | cut -d'"' -f4)"; \
	if [ -z "$$TENANT_KEY" ]; then echo "FAIL: no tenant_key returned by POST /tenants"; exit 1; fi; \
	echo "-- provisioned fresh tenant, tenant_key=$$TENANT_KEY --"; \
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "Authorization: Bearer $$TENANT_KEY" -H 'Content-Type: application/json' -d '{"account_id":"treasury-04","type":"system"}'; \
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "Authorization: Bearer $$TENANT_KEY" -H 'Content-Type: application/json' -d '{"account_id":"alice-04","type":"wallet"}'; \
	curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "Authorization: Bearer $$TENANT_KEY" -H 'Idempotency-Key: demo-04-fund' -H 'Content-Type: application/json' -d '{"from":"treasury-04","to":"alice-04","amount":"100.00"}'; \
	echo "-- posting the fresh tenant's first transfer, expect 201 --"; \
	RESPONSE="$$(curl -s -w '\nHTTP_CODE:%{http_code}' -X POST $(APP_URL)/transfers -H "Authorization: Bearer $$TENANT_KEY" -H 'Idempotency-Key: demo-04-first-transfer' -H 'Content-Type: application/json' -d '{"from":"treasury-04","to":"alice-04","amount":"10.00"}')"; \
	echo "$$RESPONSE"; \
	HTTP_CODE="$$(echo "$$RESPONSE" | grep -o 'HTTP_CODE:[0-9]*' | cut -d: -f2)"; \
	if [ "$$HTTP_CODE" != "201" ]; then echo "FAIL: fresh tenant's first transfer returned HTTP $$HTTP_CODE"; exit 1; fi; \
	END=$$(date +%s); \
	DURATION=$$((END - START)); \
	echo "tenant_onboard_transfer_seconds=$$DURATION"

# --- chaos-01: kill mid-write, verify no half-applied movement -------------

chaos-01: wait-app
	@echo "== chaos-01: kill the app mid-write, verify no half-applied movement =="
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"treasury","type":"system"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"alice-chaos","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/accounts -H "$(AUTH)" -H 'Content-Type: application/json' -d '{"account_id":"bob-chaos","type":"wallet"}'
	curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: chaos-01-fund' -H 'Content-Type: application/json' -d '{"from":"treasury","to":"alice-chaos","amount":"100.00"}'
	@echo "-- firing the transfer, then killing the app service mid-flight --"
	( curl -s -o /dev/null -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: chaos-01-kill' -H 'Content-Type: application/json' -d '{"from":"alice-chaos","to":"bob-chaos","amount":"25.00"}' & )
	sleep 0.05
	docker compose kill -s SIGKILL app
	docker compose start app
	@$(MAKE) --no-print-directory wait-app
	@echo "-- resubmitting the same key after restart, must land exactly once --"
	curl -s -w '\nHTTP %{http_code}\n' -X POST $(APP_URL)/transfers -H "$(AUTH)" -H 'Idempotency-Key: chaos-01-kill' -H 'Content-Type: application/json' -d '{"from":"alice-chaos","to":"bob-chaos","amount":"25.00"}'
	@echo "-- reading alice-chaos back: must be 75.00 (one 25.00 movement, not zero and not two) --"
	curl -s $(APP_URL)/accounts/alice-chaos -H "$(AUTH)"
	@echo ""

# --- race-02 / race-03: operational proof of I4 / I7 under contention ------
#
# scripts/race drives the SAME guarantee the acceptance suite's in-process
# RaceSpenders/RaceSubmitters harness proves under `go test`, but here against
# the real docker-compose stack, through the driving HTTP port only. It prints
# the KPI denominators (race-02: negative_balance_observations, iterations;
# race-03: distinct_transaction_ids, entry_pairs_stored, submissions) on
# stdout -- this isn't wired to a dashboard yet, so stdout is the contract.

race-02: wait-app
	go run ./scripts/race --scenario=kpi2

race-03: wait-app
	go run ./scripts/race --scenario=kpi3
