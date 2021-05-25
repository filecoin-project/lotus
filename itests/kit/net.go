package kit

//
// func StartTwoNodesOneMiner(ctx context.Context, t *testing.T, blocktime time.Duration) ([]TestFullNode, []address.Address) {
// 	n, sn := MinerRPCMockMinerBuilder(t, TwoFull, OneMiner)
//
// 	fullNode1 := n[0]
// 	fullNode2 := n[1]
// 	miner := sn[0]
//
// 	// Get everyone connected
// 	addrs, err := fullNode1.NetAddrsListen(ctx)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	if err := fullNode2.NetConnect(ctx, addrs); err != nil {
// 		t.Fatal(err)
// 	}
//
// 	// Start mining blocks
// 	bm := NewBlockMiner(t, miner)
// 	bm.MineBlocks(ctx, blocktime)
// 	t.Cleanup(bm.Stop)
//
// 	// Send some funds to register the second node
// 	fullNodeAddr2, err := fullNode2.WalletNew(ctx, types.KTSecp256k1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	SendFunds(ctx, t, fullNode1, fullNodeAddr2, abi.NewTokenAmount(1e18))
//
// 	// Get the first node's address
// 	fullNodeAddr1, err := fullNode1.WalletDefaultAddress(ctx)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	// Create mock CLI
// 	return n, []address.Address{fullNodeAddr1, fullNodeAddr2}
// }
