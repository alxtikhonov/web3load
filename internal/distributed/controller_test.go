package distributed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/report"
	"github.com/web3load/web3load/internal/scenario"
	"github.com/web3load/web3load/internal/wallet"
)

func makeTestWallets(n int) []wallet.Wallet {
	ws := make([]wallet.Wallet, n)
	for i := range ws {
		var addr common.Address
		addr[19] = byte(i)
		ws[i] = wallet.Wallet{Address: addr, PrivateKey: "0x01"}
	}
	return ws
}

func testScenario() *scenario.Scenario {
	return &scenario.Scenario{
		Version: "0.1",
		Info:    scenario.Info{Name: "dist_test"},
		Target:  scenario.Target{RPCURL: "http://127.0.0.1:8545", ChainID: 31337, Environment: "local"},
		Load:    scenario.Load{Type: "constant", VUs: 9, Duration: scenario.Duration(time.Second)},
		Wallets: scenario.Wallets{Count: 9},
		Steps:   []scenario.Step{{Action: "get_balance"}},
	}
}

func doRegister(t *testing.T, baseURL, workerID string) (Assignment, error) {
	t.Helper()
	body, err := json.Marshal(RegisterRequest{WorkerID: workerID})
	if err != nil {
		return Assignment{}, err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/register", bytes.NewReader(body))
	if err != nil {
		return Assignment{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Assignment{}, err
	}
	defer resp.Body.Close()
	var a Assignment
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return Assignment{}, err
	}
	return a, nil
}

// TestController_RegisterBlocksUntilCohortComplete is the key correctness
// test for the whole distributed package: it proves the registration
// barrier actually blocks (not just "eventually returns something"), and
// that the resulting shards partition the wallet set exactly — the
// invariant a nonce collision across two worker processes would silently
// violate.
func TestController_RegisterBlocksUntilCohortComplete(t *testing.T) {
	ctrl := NewController(testScenario(), makeTestWallets(9), 3)
	srv := httptest.NewServer(ctrl.Handler())
	defer srv.Close()

	type outcome struct {
		assignment Assignment
		err        error
	}
	results := make(chan outcome, 3)
	var wg sync.WaitGroup

	register := func(id string) {
		defer wg.Done()
		a, err := doRegister(t, srv.URL, id)
		results <- outcome{assignment: a, err: err}
	}

	wg.Add(2)
	go register("w1")
	go register("w2")

	select {
	case <-results:
		t.Fatal("expected registration to block until all 3 workers had joined")
	case <-time.After(150 * time.Millisecond):
	}

	wg.Add(1)
	go register("w3")
	wg.Wait()
	close(results)

	var assignments []Assignment
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		assignments = append(assignments, r.assignment)
	}
	if len(assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(assignments))
	}

	seenIdx := map[int]bool{}
	seenWallets := map[common.Address]int{}
	for _, a := range assignments {
		if a.ShardCount != 3 {
			t.Errorf("expected shard_count 3, got %d", a.ShardCount)
		}
		if seenIdx[a.ShardIndex] {
			t.Errorf("shard index %d assigned twice", a.ShardIndex)
		}
		seenIdx[a.ShardIndex] = true
		if a.ScenarioYAML == "" {
			t.Error("expected a non-empty scenario YAML")
		}
		for _, w := range a.Wallets {
			seenWallets[w.Address]++
		}
	}
	if len(seenIdx) != 3 {
		t.Fatalf("expected shard indices {0,1,2} all used, got %v", seenIdx)
	}
	if len(seenWallets) != 9 {
		t.Fatalf("expected all 9 wallets distributed exactly once, got %d unique", len(seenWallets))
	}
	for addr, count := range seenWallets {
		if count != 1 {
			t.Errorf("wallet %s assigned to %d shards, want exactly 1", addr.Hex(), count)
		}
	}
}

func TestController_Aggregate_SumsAndWeights(t *testing.T) {
	ctrl := NewController(testScenario(), makeTestWallets(3), 2)
	ctrl.results["w1"] = report.Result{
		TotalTransactions: 100, Throughput: 10, SuccessRate: 100,
		RPCLatency: report.Percentiles{P95: 50 * time.Millisecond},
	}
	ctrl.results["w2"] = report.Result{
		TotalTransactions: 300, Throughput: 30, SuccessRate: 50,
		RPCLatency: report.Percentiles{P95: 80 * time.Millisecond},
	}

	agg := ctrl.Aggregate()
	if agg.TotalTransactions != 400 {
		t.Errorf("expected summed total transactions 400, got %d", agg.TotalTransactions)
	}
	if agg.Throughput != 40 {
		t.Errorf("expected summed throughput 40, got %v", agg.Throughput)
	}
	// weighted: (100*100 + 300*50) / 400 = 25000/400 = 62.5
	if agg.SuccessRate != 62.5 {
		t.Errorf("expected weighted success rate 62.5, got %v", agg.SuccessRate)
	}
	if agg.RPCLatency.P95 != 80*time.Millisecond {
		t.Errorf("expected max p95 80ms across workers, got %v", agg.RPCLatency.P95)
	}
}

func TestController_WaitForCompletion(t *testing.T) {
	ctrl := NewController(testScenario(), makeTestWallets(2), 2)
	srv := httptest.NewServer(ctrl.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- ctrl.WaitForCompletion(ctx) }()

	for _, id := range []string{"w1", "w2"} {
		body, _ := json.Marshal(MetricsReport{WorkerID: id, Done: true})
		resp, err := http.Post(srv.URL+"/metrics", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected WaitForCompletion to succeed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForCompletion did not return after both workers reported done")
	}
}

func TestController_WaitForCompletion_TimesOutIfAWorkerNeverReports(t *testing.T) {
	ctrl := NewController(testScenario(), makeTestWallets(2), 2)
	srv := httptest.NewServer(ctrl.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	body, _ := json.Marshal(MetricsReport{WorkerID: "w1", Done: true})
	resp, err := http.Post(srv.URL+"/metrics", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if err := ctrl.WaitForCompletion(ctx); err == nil {
		t.Fatal("expected WaitForCompletion to time out when only 1 of 2 workers reported done")
	}
}
