package wallet

import (
	"context"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
)

// fakeAdapter embeds a nil chain.Adapter so it satisfies the interface via
// method promotion, then overrides only NonceAt — the only method the
// nonce manager calls.
type fakeAdapter struct {
	chain.Adapter
}

func (fakeAdapter) NonceAt(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}

func TestNonceManager_ConcurrentNextNeverCollides(t *testing.T) {
	m := NewNonceManager(fakeAdapter{})
	addr := common.HexToAddress("0x1")

	const n = 500
	seen := make(map[uint64]bool, n)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nonce, err := m.Next(context.Background(), addr)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[nonce] {
				t.Errorf("nonce %d issued twice", nonce)
			}
			seen[nonce] = true
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Fatalf("expected %d unique nonces, got %d", n, len(seen))
	}
}

func TestNonceManager_ResyncRefetchesFromChain(t *testing.T) {
	m := NewNonceManager(fakeAdapter{})
	addr := common.HexToAddress("0x2")

	first, err := m.Next(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if first != 0 {
		t.Fatalf("expected first nonce 0, got %d", first)
	}

	if err := m.Resync(context.Background(), addr); err != nil {
		t.Fatal(err)
	}
	after, err := m.Next(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("expected resync to reset to on-chain nonce 0, got %d", after)
	}
}
