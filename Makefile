SHELL := /bin/bash

.PHONY: up down wait-app demo-01 demo-02 demo-03 chaos-01 race-02 race-03

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
AUTH := Authorization: Bearer demo-operator-key

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
