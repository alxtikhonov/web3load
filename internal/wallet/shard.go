package wallet

// ShardWallets splits wallets into shardCount contiguous, non-overlapping
// slices and returns the one at shardIndex, sized as evenly as possible
// (earliest shards absorb any remainder). This is the invariant distributed
// mode depends on: no two workers may ever hold the same wallet, or their
// independently-tracked nonces for it would collide and corrupt each
// other's transactions.
func ShardWallets(wallets []Wallet, shardIndex, shardCount int) []Wallet {
	if shardCount <= 1 {
		return wallets
	}
	start := 0
	for i := 0; i < shardIndex; i++ {
		start += chunkSize(len(wallets), i, shardCount)
	}
	if start > len(wallets) {
		start = len(wallets)
	}
	end := start + chunkSize(len(wallets), shardIndex, shardCount)
	if end > len(wallets) {
		end = len(wallets)
	}
	return wallets[start:end]
}

func chunkSize(total, index, count int) int {
	base := total / count
	if index < total%count {
		base++
	}
	return base
}
