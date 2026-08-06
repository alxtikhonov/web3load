package wallet

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func makeWallets(n int) []Wallet {
	ws := make([]Wallet, n)
	for i := range ws {
		var addr common.Address
		addr[19] = byte(i)
		ws[i] = Wallet{Address: addr}
	}
	return ws
}

func TestShardWallets_EvenSplit(t *testing.T) {
	wallets := makeWallets(9)
	for shard := 0; shard < 3; shard++ {
		got := ShardWallets(wallets, shard, 3)
		if len(got) != 3 {
			t.Fatalf("shard %d: expected 3 wallets, got %d", shard, len(got))
		}
	}
}

func TestShardWallets_NoOverlapAndCoversAll(t *testing.T) {
	wallets := makeWallets(10)
	seen := make(map[common.Address]int)
	for shard := 0; shard < 3; shard++ {
		for _, w := range ShardWallets(wallets, shard, 3) {
			seen[w.Address]++
		}
	}
	if len(seen) != len(wallets) {
		t.Fatalf("expected all %d wallets covered exactly once, got %d unique", len(wallets), len(seen))
	}
	for addr, count := range seen {
		if count != 1 {
			t.Errorf("wallet %s assigned to %d shards, want exactly 1", addr.Hex(), count)
		}
	}
}

func TestShardWallets_RemainderGoesToEarliestShards(t *testing.T) {
	wallets := makeWallets(10) // 10 / 3 = 3 remainder 1 -> sizes 4,3,3
	sizes := []int{
		len(ShardWallets(wallets, 0, 3)),
		len(ShardWallets(wallets, 1, 3)),
		len(ShardWallets(wallets, 2, 3)),
	}
	want := []int{4, 3, 3}
	for i := range want {
		if sizes[i] != want[i] {
			t.Errorf("shard %d: got size %d, want %d", i, sizes[i], want[i])
		}
	}
}

func TestShardWallets_SingleShardReturnsAll(t *testing.T) {
	wallets := makeWallets(5)
	got := ShardWallets(wallets, 0, 1)
	if len(got) != 5 {
		t.Fatalf("expected all 5 wallets for a single shard, got %d", len(got))
	}
}
