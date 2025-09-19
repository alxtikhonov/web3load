package wallet

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/web3load/web3load/internal/chain"
)

// NonceManager hands out strictly increasing, collision-free nonces per
// address even when many virtual users race to send transactions from the
// same wallet concurrently. This is the load-testing-specific correctness
// problem generic HTTP tools never have to solve: get a nonce wrong and
// every later transaction from that wallet fails until it's fixed.
type NonceManager struct {
	adapter chain.Adapter
	mu      sync.Mutex
	next    map[common.Address]uint64
}

func NewNonceManager(adapter chain.Adapter) *NonceManager {
	return &NonceManager{adapter: adapter, next: make(map[common.Address]uint64)}
}

// Next allocates the next nonce for addr, fetching the on-chain pending
// nonce on first use.
func (m *NonceManager) Next(ctx context.Context, addr common.Address) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.next[addr]
	if !ok {
		onChain, err := m.adapter.NonceAt(ctx, addr)
		if err != nil {
			return 0, fmt.Errorf("nonce: fetch initial nonce for %s: %w", addr.Hex(), err)
		}
		n = onChain
	}
	m.next[addr] = n + 1
	return n, nil
}

// Release returns a nonce to the pool after its transaction failed before
// ever reaching the mempool (signing error, RPC rejection). It only rewinds
// when nonce is the most recently issued one for addr — an
// out-of-order release (an older nonce failing after a newer one already
// succeeded) is a known v0.1 limitation and leaves that gap for Resync to
// fix instead.
func (m *NonceManager) Release(addr common.Address, nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.next[addr]; ok && nonce+1 == current {
		m.next[addr] = nonce
	}
}

// Resync discards the locally tracked nonce for addr and re-fetches it from
// the chain. Call this after an RPC error like "nonce too low"/"nonce too
// high" that indicates local state has drifted from the chain's.
func (m *NonceManager) Resync(ctx context.Context, addr common.Address) error {
	onChain, err := m.adapter.NonceAt(ctx, addr)
	if err != nil {
		return fmt.Errorf("nonce: resync %s: %w", addr.Hex(), err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next[addr] = onChain
	return nil
}
