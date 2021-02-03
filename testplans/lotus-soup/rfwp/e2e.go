package rfwp

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"
	"golang.org/x/sync/errgroup"
)

func RecoveryFromFailedWindowedPoStE2E(t *testkit.TestEnvironment) error {
	switch t.Role {
	case "bootstrapper":
		return testkit.HandleDefaultRole(t)
	case "client":
		return handleClient(t)
	case "miner":
		return handleMiner(t)
	case "miner-full-slash":
		return handleMinerFullSlash(t)
	case "miner-partial-slash":
		return handleMinerPartialSlash(t)
	}

	return fmt.Errorf("unknown role: %s", t.Role)
}

func handleMiner(t *testkit.TestEnvironment) error {
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	t.RecordMessage("running miner: %s", myActorAddr)

	if t.GroupSeq == 1 {
		go FetchChainState(t, m)
	}

	go UpdateChainState(t, m)

	minersToBeSlashed := 2
	ch := make(chan testkit.SlashedMinerMsg)
	sub := t.SyncClient.MustSubscribe(ctx, testkit.SlashedMinerTopic, ch)
	var eg errgroup.Group

	for i := 0; i < minersToBeSlashed; i++ {
		select {
		case slashedMiner := <-ch:
			// wait for slash
			eg.Go(func() error {
				select {
				case <-waitForSlash(t, slashedMiner):
				case err = <-t.SyncClient.MustBarrier(ctx, testkit.StateAbortTest, 1).C:
					if err != nil {
						return err
					}
					return errors.New("got abort signal, exitting")
				}
				return nil
			})
		case err := <-sub.Done():
			return fmt.Errorf("got error while waiting for slashed miners: %w", err)
		case err := <-t.SyncClient.MustBarrier(ctx, testkit.StateAbortTest, 1).C:
			if err != nil {
				return err
			}
			return errors.New("got abort signal, exitting")
		}
	}

	errc := make(chan error)
	go func() {
		errc <- eg.Wait()
	}()

	select {
	case err := <-errc:
		if err != nil {
			return err
		}
	case err := <-t.SyncClient.MustBarrier(ctx, testkit.StateAbortTest, 1).C:
		if err != nil {
			return err
		}
		return errors.New("got abort signal, exitting")
	}

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

func waitForSlash(t *testkit.TestEnvironment, msg testkit.SlashedMinerMsg) chan error {
	// assert that balance got reduced with that much 5 times (sector fee)
	// assert that balance got reduced with that much 2 times (termination fee)
	// assert that balance got increased with that much 10 times (block reward)
	// assert that power got increased with that much 1 times (after sector is sealed)
	// assert that power got reduced with that much 1 times (after sector is announced faulty)
	slashedMiner := msg.MinerActorAddr

	errc := make(chan error)
	go func() {
		foundSlashConditions := false
		for range time.Tick(10 * time.Second) {
			if foundSlashConditions {
				close(errc)
				return
			}
			t.RecordMessage("wait for slashing, tick")
			func() {
				cs.Lock()
				defer cs.Unlock()

				negativeAmounts := []big.Int{}
				negativeDiffs := make(map[big.Int][]abi.ChainEpoch)

				for am, heights := range cs.DiffCmp[slashedMiner.String()]["LockedFunds"] {
					amount, err := big.FromString(am)
					if err != nil {
						errc <- fmt.Errorf("cannot parse LockedFunds amount: %w:", err)
						return
					}

					// amount is negative => slash condition
					if big.Cmp(amount, big.Zero()) < 0 {
						negativeDiffs[amount] = heights
						negativeAmounts = append(negativeAmounts, amount)
					}
				}

				t.RecordMessage("negative diffs: %d", len(negativeDiffs))
				if len(negativeDiffs) < 3 {
					return
				}

				sort.Slice(negativeAmounts, func(i, j int) bool { return big.Cmp(negativeAmounts[i], negativeAmounts[j]) > 0 })

				// TODO: confirm the largest is > 18 filecoin
				// TODO: confirm the next largest is > 9 filecoin
				foundSlashConditions = true
			}()
		}
	}()

	return errc
}

func handleMinerFullSlash(t *testkit.TestEnvironment) error {
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	t.RecordMessage("running miner, full slash: %s", myActorAddr)

	// TODO: wait until we have sealed a deal for a client
	time.Sleep(240 * time.Second)

	t.RecordMessage("shutting down miner, full slash: %s", myActorAddr)

	ctxt, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = m.StopFn(ctxt)
	if err != nil {
		//return err
		t.RecordMessage("err from StopFn: %s", err.Error()) // TODO: expect this to be fixed on Lotus
	}

	t.RecordMessage("shutdown miner, full slash: %s", myActorAddr)

	t.SyncClient.MustPublish(ctx, testkit.SlashedMinerTopic, testkit.SlashedMinerMsg{
		MinerActorAddr: myActorAddr,
	})

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

func handleMinerPartialSlash(t *testkit.TestEnvironment) error {
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	t.RecordMessage("running miner, partial slash: %s", myActorAddr)

	// TODO: wait until we have sealed a deal for a client
	time.Sleep(185 * time.Second)

	t.RecordMessage("shutting down miner, partial slash: %s", myActorAddr)

	ctxt, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = m.StopFn(ctxt)
	if err != nil {
		//return err
		t.RecordMessage("err from StopFn: %s", err.Error()) // TODO: expect this to be fixed on Lotus
	}

	t.RecordMessage("shutdown miner, partial slash: %s", myActorAddr)

	t.SyncClient.MustPublish(ctx, testkit.SlashedMinerTopic, testkit.SlashedMinerMsg{
		MinerActorAddr: myActorAddr,
	})

	time.Sleep(300 * time.Second)

	rm, err := testkit.RestoreMiner(t, m)
	if err != nil {
		t.RecordMessage("got err: %s", err.Error())
		return err
	}

	myActorAddr, err = rm.MinerApi.ActorAddress(ctx)
	if err != nil {
		t.RecordMessage("got err: %s", err.Error())
		return err
	}

	t.RecordMessage("running miner again, partial slash: %s", myActorAddr)

	time.Sleep(3600 * time.Second)

	//t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

func handleClient(t *testkit.TestEnvironment) error {
	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	// This is a client role
	t.RecordMessage("running client")

	ctx := context.Background()
	client := cl.FullApi

	time.Sleep(10 * time.Second)

	// select a miner based on our GroupSeq (client 1 -> miner 1 ; client 2 -> miner 2)
	// this assumes that all miner instances receive the same sorted MinerAddrs slice
	minerAddr := cl.MinerAddrs[t.InitContext.GroupSeq-1]
	if err := client.NetConnect(ctx, minerAddr.MinerNetAddrs); err != nil {
		return err
	}
	t.D().Counter(fmt.Sprintf("send-data-to,miner=%s", minerAddr.MinerActorAddr)).Inc(1)

	t.RecordMessage("selected %s as the miner", minerAddr.MinerActorAddr)

	time.Sleep(2 * time.Second)

	// generate 1800 bytes of random data
	data := make([]byte, 1800)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)

	file, err := ioutil.TempFile("/tmp", "data")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	fcid, err := client.ClientImport(ctx, api.FileRef{Path: file.Name(), IsCAR: false})
	if err != nil {
		return err
	}
	t.RecordMessage("file cid: %s", fcid)

	// start deal
	t1 := time.Now()
	fastRetrieval := false
	deal := testkit.StartDeal(ctx, minerAddr.MinerActorAddr, client, fcid.Root, fastRetrieval)
	t.RecordMessage("started deal: %s", deal)

	// this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(2 * time.Second)

	t.RecordMessage("waiting for deal to be sealed")
	testkit.WaitDealSealed(t, ctx, client, deal)
	t.D().ResettingHistogram("deal.sealed").Update(int64(time.Since(t1)))

	// TODO: wait to stop miner (ideally get a signal, rather than sleep)
	time.Sleep(180 * time.Second)

	t.RecordMessage("trying to retrieve %s", fcid)
	info, err := client.ClientGetDealInfo(ctx, *deal)
	if err != nil {
		return err
	}

	carExport := true
	err = testkit.RetrieveData(t, ctx, client, fcid.Root, &info.PieceCID, carExport, data)
	if err != nil && strings.Contains(err.Error(), "cannot make retrieval deal for zero bytes") {
		t.D().Counter("deal.expect-slashing").Inc(1)
	} else if err != nil {
		// unknown error => fail test
		t.RecordFailure(err)

		// send signal to abort test
		t.SyncClient.MustSignalEntry(ctx, testkit.StateAbortTest)

		t.D().ResettingHistogram("deal.retrieved.err").Update(int64(time.Since(t1)))
		time.Sleep(10 * time.Second) // wait for metrics to be emitted

		return nil
	}

	t.D().ResettingHistogram("deal.retrieved").Update(int64(time.Since(t1)))
	time.Sleep(10 * time.Second) // wait for metrics to be emitted

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount) // TODO: not sure about this
	return nil
}
