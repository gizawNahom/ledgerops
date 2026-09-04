// Command race is the operational counterpart to the acceptance suite's
// in-process RaceSpenders/RaceSubmitters harness (tests/acceptance/ledgercore).
// It drives the SAME guarantee (I4 for kpi2, I7 for kpi3) against a running
// docker-compose stack, entirely through the driving HTTP port -- no import
// of internal/ or tests/acceptance/, by design (step 05-01 boundary rule).
//
// It exists because the KPI contract only asks for the denominators
// (negative_balance_observations/iterations for kpi2;
// distinct_transaction_ids/entry_pairs_stored/submissions for kpi3) to be
// visible on stdout for the operational demo -- the in-process suite proves
// correctness under go test, this proves the same shape holds against the
// real compose stack a clean clone actually runs.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	scenario := flag.String("scenario", "", "kpi2 or kpi3")
	flag.Parse()

	baseURL := envOr("RACE_APP_URL", "http://localhost:8080")
	// RACE_OPERATOR_KEY is retained only for symmetry with RACE_TENANT_KEY --
	// no scenario in this file currently needs an unscoped OperatorKey call.
	operatorKey := envOr("RACE_OPERATOR_KEY", "demo-operator-key")
	// OPS-10/OPS-11 (step 02-05): DDD-23 Option C moved POST /accounts,
	// POST /transfers, and GET /accounts/{id} to a requireTenantKey-ONLY
	// group -- kpi2/kpi3's account creation, transfers, and balance polling
	// all land on those three routes, so they authenticate with the tenant
	// credential instead. Defaults to LEDGEROPS_DEMO_TENANT_KEY's own value
	// (docker-compose.yml), the same seeded credential the Makefile's AUTH
	// now uses, so `go run ./scripts/race` needs no extra wiring against the
	// compose stack.
	tenantKey := envOr("RACE_TENANT_KEY", envOr("LEDGEROPS_DEMO_TENANT_KEY", "demo-tenant-key"))
	client := &httpClient{base: baseURL, key: tenantKey, operatorKey: operatorKey, http: &http.Client{Timeout: 10 * time.Second}}

	switch *scenario {
	case "kpi2":
		runKPI2(client)
	case "kpi3":
		runKPI3(client)
	default:
		fmt.Fprintln(os.Stderr, "usage: race --scenario=kpi2|kpi3")
		os.Exit(2)
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// --- HTTP driving-port client, mirroring the curl shapes in the Makefile ---

type httpClient struct {
	base string
	// key authenticates the tenant-scoped calls (account creation, transfer,
	// balance polling) this script drives. operatorKey is carried for
	// symmetry with RACE_TENANT_KEY only -- no scenario in this file
	// currently issues an unscoped OperatorKey call.
	key         string
	operatorKey string
	http        *http.Client
}

func (c *httpClient) postJSON(path string, body map[string]any, headers map[string]string) (int, map[string]any) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	return resp.StatusCode, decodeJSON(resp.Body)
}

func (c *httpClient) get(path string) (int, map[string]any) {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	return resp.StatusCode, decodeJSON(resp.Body)
}

func decodeJSON(r io.Reader) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r).Decode(&body)
	return body
}

func (c *httpClient) createAccount(id, kind string) {
	status, body := c.postJSON("/accounts", map[string]any{"account_id": id, "type": kind}, nil)
	if status != http.StatusCreated && status != http.StatusConflict {
		fmt.Fprintf(os.Stderr, "unexpected status %d creating account %q: %v\n", status, id, body)
		os.Exit(1)
	}
}

func (c *httpClient) transfer(from, to, amount, key string) (int, map[string]any) {
	return c.postJSON("/transfers", map[string]any{"from": from, "to": to, "amount": amount},
		map[string]string{"Idempotency-Key": key})
}

// --- kpi2: I4 holds under contention ---------------------------------------
//
// Mirrors "A thousand contended spends never drive a wallet below zero"
// (milestone-02-sufficient-funds.feature, @kpi-2): fund one wallet with
// 500.00, fire 1000 concurrent 1.00 spends against it, and poll the balance
// throughout for any observation below zero.
func runKPI2(c *httpClient) {
	const funded = 500.00
	const spendAmount = "1.00"
	const iterations = 1000

	fmt.Println("== race-02: KPI-2, I4 holds under contention ==")

	c.createAccount("treasury-kpi2", "system")
	c.createAccount("alice-kpi2", "wallet")
	c.createAccount("bob-kpi2", "wallet")
	if status, body := c.transfer("treasury-kpi2", "alice-kpi2", fmt.Sprintf("%.2f", funded), "kpi2-fund"); status != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "funding alice-kpi2 failed: %d %v\n", status, body)
		os.Exit(1)
	}

	stopPolling := make(chan struct{})
	var negativeBalanceObservations int
	var pollerWG sync.WaitGroup
	pollerWG.Add(1)
	go func() {
		defer pollerWG.Done()
		for {
			select {
			case <-stopPolling:
				return
			default:
			}
			if status, body := c.get("/accounts/alice-kpi2"); status == http.StatusOK {
				if balanceIsNegative(body["balance"]) {
					negativeBalanceObservations++
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.transfer("alice-kpi2", "bob-kpi2", spendAmount, fmt.Sprintf("kpi2-spend-%d", i))
		}(i)
	}
	wg.Wait()
	close(stopPolling)
	pollerWG.Wait()

	fmt.Printf("iterations=%d\n", iterations)
	fmt.Printf("negative_balance_observations=%d\n", negativeBalanceObservations)
	if negativeBalanceObservations == 0 {
		fmt.Println("PASS: no wallet balance was ever observed below zero")
	} else {
		fmt.Println("FAIL: a wallet balance was observed below zero")
		os.Exit(1)
	}
}

func balanceIsNegative(balance any) bool {
	text, ok := balance.(string)
	if !ok || text == "" {
		return false
	}
	return text[0] == '-'
}

// --- kpi3: I7 holds under contention ----------------------------------------
//
// Mirrors "Fifty simultaneous submissions of one key move value once"
// (milestone-03-idempotent-retry.feature, @kpi-3): fire 50 concurrent
// submissions of the same transfer under the same idempotency key and prove
// they collapse onto one transaction and one stored entry pair.
func runKPI3(c *httpClient) {
	const submissions = 50
	const key = "kpi3-race-key"

	fmt.Println("== race-03: KPI-3, I7 holds under contention ==")

	c.createAccount("treasury-kpi3", "system")
	c.createAccount("alice-kpi3", "wallet")
	c.createAccount("bob-kpi3", "wallet")
	if status, body := c.transfer("treasury-kpi3", "alice-kpi3", "100.00", "kpi3-fund"); status != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "funding alice-kpi3 failed: %d %v\n", status, body)
		os.Exit(1)
	}

	var mu sync.Mutex
	transactionIDs := map[string]int{}
	var wg sync.WaitGroup
	for i := 0; i < submissions; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := c.transfer("alice-kpi3", "bob-kpi3", "50.00", key)
			if status != http.StatusCreated && status != http.StatusOK {
				fmt.Fprintf(os.Stderr, "unexpected status %d submitting under shared key: %v\n", status, body)
				return
			}
			txnID, _ := body["transaction_id"].(string)
			mu.Lock()
			transactionIDs[txnID]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	var winningTxnID string
	for id := range transactionIDs {
		winningTxnID = id
	}

	entryPairsStored := countEntryPairs(c, "alice-kpi3", winningTxnID) + countEntryPairs(c, "bob-kpi3", winningTxnID)
	// Two accounts, one leg recorded on each -- a settled pair is 2 legs total.
	entryPairsStored /= 2

	fmt.Printf("submissions=%d\n", submissions)
	fmt.Printf("distinct_transaction_ids=%d\n", len(transactionIDs))
	fmt.Printf("entry_pairs_stored=%d\n", entryPairsStored)
	if len(transactionIDs) == 1 && entryPairsStored == 1 {
		fmt.Println("PASS: all submissions collapsed onto one transaction, one stored entry pair")
	} else {
		fmt.Println("FAIL: submissions did not collapse onto a single transaction/entry pair")
		os.Exit(1)
	}
}

func countEntryPairs(c *httpClient, accountID, transactionID string) int {
	status, body := c.get("/accounts/" + accountID + "/entries")
	if status != http.StatusOK {
		return 0
	}
	entries, _ := body["entries"].([]any)
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := entry["transaction_id"].(string); id == transactionID {
			count++
		}
	}
	return count
}
