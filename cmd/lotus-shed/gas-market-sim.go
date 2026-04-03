package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
)

//go:embed gas_limit_dist.csv
var defaultGasLimitDistCSV []byte

// The width of each bucket in gas.
// Custom distributions supplied via --gas-limit-dist must use the same bucket size.
const gasLimitBucketSize = 20_000_000

// Transactions above this gas limit are classified as "large" and tracked under nb20.
const largeGasTxThreshold = 8_000_000_000

// gasLimitDist samples gas limits from a bucket distribution.
// Within a bucket, the sample is drawn uniformly.
type gasLimitDist struct {
	cdf   []int64 // cdf[i] = sum of counts for buckets 0..i (inclusive)
	total int64
}

// The CSV format is two columns (bucket, count).
// Bucket i covers [i*gasLimitBucketSize, (i+1)*gasLimitBucketSize).
func loadGasLimitDist(r io.Reader) (*gasLimitDist, error) {
	cr := csv.NewReader(r)
	cr.Comment = '#'
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("gas limit dist CSV has no data rows")
	}
	// find max bucket index to size the CDF slice
	maxBucket := 0
	for _, row := range rows[1:] {
		b, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, fmt.Errorf("invalid bucket %q: %w", row[0], err)
		}
		if b > maxBucket {
			maxBucket = b
		}
	}
	counts := make([]int64, maxBucket+1)
	for _, row := range rows[1:] {
		b, _ := strconv.Atoi(row[0])
		c, err := strconv.ParseInt(row[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid count %q: %w", row[1], err)
		}
		counts[b] += c
	}
	cdf := make([]int64, len(counts))
	var running int64
	for i, c := range counts {
		running += c
		cdf[i] = running
	}
	if running == 0 {
		return nil, fmt.Errorf("gas limit dist has zero total weight")
	}
	return &gasLimitDist{cdf: cdf, total: running}, nil
}

// sample draws one gas limit from the distribution.
func (d *gasLimitDist) sample(rng *rand.Rand) int64 {
	r := rng.Int63n(d.total)
	// binary search for the first bucket whose cdf > r
	lo, hi := 0, len(d.cdf)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if d.cdf[mid] <= r {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	base := int64(lo) * gasLimitBucketSize
	offset := rng.Int63n(gasLimitBucketSize)
	return base + offset
}

// historicTx holds the gas parameters for one transaction from a mempool history CSV.
type historicTx struct {
	arrivalMs  int64
	gasFeeCap  abi.TokenAmount
	gasPremium abi.TokenAmount
	gasLimit   int64
}

// loadMempoolHistory reads a mempool history CSV and returns transactions grouped by epoch.
// The CSV must have at minimum the columns: arrival_timestamp_ms, epoch, next_base_fee, gas_fee_cap, gas_premium, gas_limit.
// Also returns the (min, max) epoch seen and the next_base_fee from the earliest epoch (used as a
// default starting base fee for the simulation).
func loadMempoolHistory(path string) (txsByEpoch map[int64][]historicTx, minEpoch, maxEpoch int64, firstBaseFee abi.TokenAmount, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, big.Zero(), err
	}
	defer f.Close() //nolint:errcheck

	cr := csv.NewReader(f)
	header, err := cr.Read()
	if err != nil {
		return nil, 0, 0, big.Zero(), fmt.Errorf("reading CSV header: %w", err)
	}
	col := make(map[string]int)
	for i, h := range header {
		col[h] = i
	}
	for _, required := range []string{"arrival_timestamp_ms", "epoch", "next_base_fee", "gas_fee_cap", "gas_premium", "gas_limit"} {
		if _, ok := col[required]; !ok {
			return nil, 0, 0, big.Zero(), fmt.Errorf("mempool CSV missing required column %q", required)
		}
	}

	txsByEpoch = make(map[int64][]historicTx)
	minEpoch = math.MaxInt64
	maxEpoch = math.MinInt64

	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, 0, big.Zero(), err
		}
		arrivalMs, err := strconv.ParseInt(row[col["arrival_timestamp_ms"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing arrival_timestamp_ms: %w", err)
		}
		epoch, err := strconv.ParseInt(row[col["epoch"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing epoch: %w", err)
		}
		nextBaseFeeI, err := strconv.ParseInt(row[col["next_base_fee"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing next_base_fee: %w", err)
		}
		feeCapI, err := strconv.ParseInt(row[col["gas_fee_cap"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing gas_fee_cap: %w", err)
		}
		premiumI, err := strconv.ParseInt(row[col["gas_premium"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing gas_premium: %w", err)
		}
		gasLimitI, err := strconv.ParseInt(row[col["gas_limit"]], 10, 64)
		if err != nil {
			return nil, 0, 0, big.Zero(), fmt.Errorf("parsing gas_limit: %w", err)
		}
		if epoch < minEpoch {
			minEpoch = epoch
			firstBaseFee = big.NewInt(nextBaseFeeI)
		}
		if epoch > maxEpoch {
			maxEpoch = epoch
		}
		txsByEpoch[epoch] = append(txsByEpoch[epoch], historicTx{
			arrivalMs:  arrivalMs,
			gasFeeCap:  big.NewInt(feeCapI),
			gasPremium: big.NewInt(premiumI),
			gasLimit:   gasLimitI,
		})
	}
	if minEpoch > maxEpoch {
		return nil, 0, 0, big.Zero(), fmt.Errorf("mempool CSV has no data rows")
	}
	return txsByEpoch, minEpoch, maxEpoch, firstBaseFee, nil
}

var gasMktSimCmd = &cli.Command{
	Name:  "gas-market-sim",
	Usage: "simulate FIP-0115 gas market dynamics over many epochs",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "epochs",
			Usage: "number of epochs to simulate",
			Value: 3600,
		},
		&cli.Int64Flag{
			Name:  "seed",
			Usage: "random seed for reproducibility",
			Value: 42,
		},
		&cli.BoolFlag{
			Name:  "epoch-remaining-zero",
			Usage: "fix epochFractionRemaining to 0 for all transactions (disables time-based scaling)",
		},
		&cli.Float64Flag{
			Name:  "gas-per-epoch",
			Usage: "average gas submitted per epoch as a multiple of BlockGasLimit",
			Value: 3.0,
		},
		&cli.BoolFlag{
			Name:  "dynamic-demand",
			Usage: "at each epoch, probabilistically increase or decrease the gas-per-epoch",
		},
		&cli.Int64Flag{
			Name:  "base-fee",
			Usage: "starting base fee in attoFIL",
			Value: buildconstants.MinimumBaseFee,
		},
		&cli.BoolFlag{
			Name:  "flat-premium",
			Usage: "bypass mempool estimator; use baseFee*R+noise as premium for all transactions",
		},
		&cli.BoolFlag{
			Name:  "legacy-premium",
			Usage: "use pre-FIP-0115 gas premium estimate based on historical included-message premiums",
		},
		&cli.StringFlag{
			Name:  "base-fee-update",
			Usage: "base fee update mechanism: 'premium' (FIP-0115 percentile) or 'utilization' (legacy)",
			Value: "premium",
		},
		&cli.StringFlag{
			Name:  "gas-limit-dist",
			Usage: "path to a CSV file (bucket,count) defining the gas limit distribution; defaults to the embedded empirical distribution",
		},
		&cli.StringFlag{
			Name:  "replay",
			Usage: "path to a mempool history CSV (columns: arrival_timestamp_ms,epoch,next_base_fee,...,gas_fee_cap,gas_premium,gas_limit); the simulation starts one epoch before the first epoch in the file",
		},
		&cli.BoolFlag{
			Name:  "keep-historic-prices",
			Usage: "when using --replay, use gas_fee_cap and gas_premium from the CSV as-is instead of re-estimating them",
		},
		&cli.Int64Flag{
			Name:  "genesis-timestamp",
			Usage: "Unix timestamp (seconds) of chain epoch 0, used to compute epoch start times from arrival timestamps in --replay mode",
			Value: MAINNET_GENESIS_TIME,
		},
	},
	Action: runGasMktSim,
}

// simChainStore implements gasutils.ChainStoreAPI backed by a single in-memory TipSet.
type simChainStore struct{ ts *types.TipSet }

func (s *simChainStore) GetHeaviestTipSet() *types.TipSet { return s.ts }
func (s *simChainStore) GetTipSetFromKey(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return s.ts, nil
}
func (s *simChainStore) LoadTipSet(_ context.Context, _ types.TipSetKey) (*types.TipSet, error) {
	return s.ts, nil
}

// simEpochData holds the gas metas of messages included in one simulated epoch.
type simEpochData struct {
	premiums   []abi.TokenAmount
	limits     []int64
	nProposers int
}

// simMpool implements gasutils.MessagePoolAPI backed by an in-memory message slice.
type simMpool struct {
	msgs []*types.SignedMessage
	ts   *types.TipSet
}

func (m *simMpool) PendingFor(_ context.Context, _ address.Address) ([]*types.SignedMessage, *types.TipSet) {
	return nil, m.ts
}
func (m *simMpool) Pending(_ context.Context) ([]*types.SignedMessage, *types.TipSet) {
	return m.msgs, m.ts
}

// buildSimTipSet constructs a single-block TipSet with the given baseFee and timestamp.
func buildSimTipSet(baseFee abi.TokenAmount, timestamp uint64) *types.TipSet {
	miner, err := address.NewIDAddress(1)
	if err != nil {
		panic(err)
	}
	dummyCid, err := cid.Decode("bafyreicmaj5hhoy5mgqvamfhgexxyergw7hdeshizghodwkjg6qmpoco7i")
	if err != nil {
		panic(err)
	}
	blk := &types.BlockHeader{
		Miner:                 miner,
		Ticket:                &types.Ticket{VRFProof: []byte{0}},
		ElectionProof:         &types.ElectionProof{VRFProof: []byte{0}},
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte{0}},
		ParentStateRoot:       dummyCid,
		ParentMessageReceipts: dummyCid,
		Messages:              dummyCid,
		ParentBaseFee:         baseFee,
		Timestamp:             timestamp,
	}
	ts, err := types.NewTipSet([]*types.BlockHeader{blk})
	if err != nil {
		panic(err)
	}
	return ts
}

// simSampleNProposers samples the number of proposers in a tipset from a
// Poisson distribution with mean 5, matching Filecoin's Expected Consensus.
func simSampleNProposers(rng *rand.Rand) int {
	const lambda = 5.0
	L := math.Exp(-lambda)
	k := 0
	p := 1.0
	for p > L {
		k++
		p *= rng.Float64()
	}
	return k - 1
}

// simEffectivePremium returns the effective gas premium clamped to max(0, feeCap-baseFee).
func simEffectivePremium(feeCap, premium, baseFee abi.TokenAmount) abi.TokenAmount {
	available := big.Sub(feeCap, baseFee)
	if available.LessThan(big.Zero()) {
		return big.Zero()
	}
	if big.Cmp(premium, available) <= 0 {
		return premium
	}
	return available
}

// simSelectMessages fills one block for a proposer with the given ticket quality,
// selecting independently from the full mempool without knowledge of other proposers.
func simSelectMessages(msgs []*types.SignedMessage, effPremiums []abi.TokenAmount, tq float64, bgl int64) []bool {
	n := len(msgs)
	chosen := make([]bool, n)
	if n == 0 {
		return chosen
	}

	gasPerfs := make([]float64, n)
	for i := range msgs {
		gasPerfs[i] = float64(effPremiums[i].Int64()) * float64(bgl)
	}

	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}
	sort.Slice(idxs, func(a, b int) bool {
		return gasPerfs[idxs[a]] > gasPerfs[idxs[b]]
	})

	if tq > 0.84 {
		var blockGas int64
		var msgCount int
		for _, idx := range idxs {
			if gasPerfs[idx] < 0 || msgCount >= buildconstants.BlockMessageLimit {
				break
			}
			if blockGas+msgs[idx].Message.GasLimit <= bgl {
				chosen[idx] = true
				blockGas += msgs[idx].Message.GasLimit
				msgCount++
			}
		}
		return chosen
	}

	blockProb := messagepool.BlockProbabilities(tq)

	partitions := make([]int, n)
	for i := range partitions {
		partitions[i] = len(blockProb)
	}
	part := 0
	var partGas int64
	for _, idx := range idxs {
		if gasPerfs[idx] < 0 {
			break
		}
		if part >= len(blockProb) {
			break
		}
		if partGas+msgs[idx].Message.GasLimit > bgl {
			part++
			partGas = 0
			if part >= len(blockProb) {
				break
			}
		}
		partitions[idx] = part
		partGas += msgs[idx].Message.GasLimit
	}

	effPerfs := make([]float64, n)
	for i := range effPerfs {
		if partitions[i] < len(blockProb) {
			effPerfs[i] = gasPerfs[i] * blockProb[partitions[i]]
		} else {
			effPerfs[i] = -1
		}
	}

	sort.Slice(idxs, func(a, b int) bool {
		return effPerfs[idxs[a]] > effPerfs[idxs[b]]
	})
	var blockGas int64
	var msgCount int
	for _, idx := range idxs {
		if effPerfs[idx] < 0 || msgCount >= buildconstants.BlockMessageLimit {
			break
		}
		if blockGas+msgs[idx].Message.GasLimit <= bgl {
			chosen[idx] = true
			blockGas += msgs[idx].Message.GasLimit
			msgCount++
		}
	}
	return chosen
}

// legacyGasPremiumEstimate replicates the pre-FIP-0115 GasEstimateGasPremium logic:
// it looks back nblocksincl*2 epochs of included-message history and returns
// the gas-weighted 55th-percentile premium (plus noise), matching the original
// medianGasPremium implementation.
// history is a circular buffer of length cap; epoch is the index of the most
// recently written slot (epoch % cap).
func legacyGasPremiumEstimate(history []simEpochData, epoch int, nblocksincl uint64, rng *rand.Rand) abi.TokenAmount {
	cap := len(history)
	lookback := int(nblocksincl * 2)
	if lookback > epoch+1 {
		lookback = epoch + 1
	}
	if lookback > cap {
		lookback = cap
	}

	type gasMeta struct {
		price abi.TokenAmount
		limit int64
	}
	var metas []gasMeta
	var totalBlocks int
	for i := 0; i < lookback; i++ {
		// epoch-i wraps around the circular buffer; add cap before modulo to avoid negative.
		e := history[(epoch-i+cap)%cap]
		totalBlocks += e.nProposers
		for j, p := range e.premiums {
			metas = append(metas, gasMeta{p, e.limits[j]})
		}
	}

	// Sort descending by price.
	sort.Slice(metas, func(i, j int) bool {
		return big.Cmp(metas[i].price, metas[j].price) > 0
	})

	// Find gas-weighted 55th percentile (50th + 5% further toward high end).
	at := buildconstants.BlockGasTarget * int64(totalBlocks) / 2
	at += buildconstants.BlockGasTarget * int64(totalBlocks) / (2 * 20)
	prev1, prev2 := big.Zero(), big.Zero()
	for _, m := range metas {
		prev1, prev2 = m.price, prev1
		at -= m.limit
		if at < 0 {
			break
		}
	}
	premium := prev1
	if prev2.Sign() != 0 {
		premium = big.Div(big.Add(prev1, prev2), big.NewInt(2))
	}

	// Apply minimum premium.
	minP := big.NewInt(int64(gasutils.MinGasPremium))
	if big.Cmp(premium, minP) < 0 {
		switch nblocksincl {
		case 1:
			premium = big.NewInt(2 * int64(gasutils.MinGasPremium))
		case 2:
			premium = big.NewInt(150000) // 1.5 * MinGasPremium
		default:
			premium = minP
		}
	}

	// Add noise: mean 1, stddev 0.005 (same as original).
	const precision = 32
	noiseFactor := 1 + rng.NormFloat64()*0.005
	noiseInt := int64(noiseFactor*float64(int64(1)<<precision)) + 1
	premium = big.Div(big.Mul(premium, big.NewInt(noiseInt)), big.NewInt(int64(1)<<precision))
	if premium.Sign() <= 0 {
		premium = big.NewInt(1)
	}
	return premium
}

// runGasMktSim runs the gas market simulation.
// Every epoch:
//   - txsPerEpoch transactions are inserted with premiums estimated via GasEstimateGasPremiumFromMempool
//     (nblocksincl bimodal: p(1)=0.1, p(2)=0.2, p(10)=0.7), gas limits drawn from a
//     empirical distribution (default: embedded gas_limit_dist.csv, 500 buckets of 20M)
//   - Each tx has a deadline = submission epoch + nblocksincl; late txs are counted
//   - 5 independent proposers each select messages with a random tq, unaware of each other;
//     the tipset is the union of their selections
//   - BaseFee is updated per FIP-0115 via NextBaseFeeFromPremium
func runGasMktSim(cctx *cli.Context) error {
	epochs := cctx.Int("epochs")
	seed := cctx.Int64("seed")
	epochRemainingZero := cctx.Bool("epoch-remaining-zero")
	gasPerEpoch := cctx.Float64("gas-per-epoch")
	dynamicDemand := cctx.Bool("dynamic-demand")
	startBaseFee := big.NewInt(cctx.Int64("base-fee"))
	flatPremium := cctx.Bool("flat-premium")
	legacyPremium := cctx.Bool("legacy-premium")
	baseFeeUpdate := cctx.String("base-fee-update")
	mempoolHistoryPath := cctx.String("replay")
	keepHistoricPrices := cctx.Bool("keep-historic-prices")
	genesisTimestamp := cctx.Int64("genesis-timestamp")

	// Load mempool history if provided.
	var (
		txsByEpoch    map[int64][]historicTx
		useHistory    bool
		startEpochNum int64
	)
	if mempoolHistoryPath != "" {
		useHistory = true
		var histMinEpoch, histMaxEpoch int64
		var firstBaseFee abi.TokenAmount
		var err error
		txsByEpoch, histMinEpoch, histMaxEpoch, firstBaseFee, err = loadMempoolHistory(mempoolHistoryPath)
		if err != nil {
			return fmt.Errorf("loading mempool history: %w", err)
		}
		startEpochNum = histMinEpoch - 1
		if !cctx.IsSet("epochs") {
			// one setup epoch + one epoch per chain epoch in the CSV
			epochs = int(histMaxEpoch-histMinEpoch) + 2
		}
		if !cctx.IsSet("base-fee") {
			startBaseFee = firstBaseFee
		}
	}

	bgl := buildconstants.BlockGasLimit

	// Load gas limit distribution.
	var glDistReader io.Reader
	if distPath := cctx.String("gas-limit-dist"); distPath != "" {
		f, err := os.Open(distPath)
		if err != nil {
			return fmt.Errorf("opening gas-limit-dist: %w", err)
		}
		defer f.Close() //nolint:errcheck
		glDistReader = f
	} else {
		glDistReader = bytes.NewReader(defaultGasLimitDistCSV)
	}
	glDist, err := loadGasLimitDist(glDistReader)
	if err != nil {
		return fmt.Errorf("loading gas limit distribution: %w", err)
	}

	// txsPerEpoch: scale so average submitted gas ≈ gasPerEpoch * BlockGasLimit.
	// Mean gas per tx from the empirical distribution (mean bucket ≈ 10, midpoint ≈ 205M).
	var distMeanGas float64
	prev := int64(0)
	for i, c := range glDist.cdf {
		bucketCount := c - prev
		prev = c
		midpoint := float64(int64(i)*gasLimitBucketSize) + gasLimitBucketSize/2
		distMeanGas += midpoint * float64(bucketCount)
	}
	distMeanGas /= float64(glDist.total)
	txsPerEpoch := int(math.Round(gasPerEpoch * float64(bgl) / distMeanGas))
	if txsPerEpoch < 0 {
		txsPerEpoch = 0
	}
	// this seed is used to generate the simulation inputs (tq, gaslimit, nlblocksincl)
	rng := rand.New(rand.NewSource(seed))
	// this seed is used by premium estimation logic
	noiseRng := rand.New(rand.NewSource(seed))
	ctx := context.Background()

	baseFee := startBaseFee
	mempool := make([]*types.SignedMessage, 0, 5000)
	deadlines := make([]int, 0, 5000)       // deadline[i] = epoch by which mempool[i] must be included
	nblocksincls := make([]uint64, 0, 5000) // nblocksincls[i] = nblocksincl used when submitting mempool[i]
	var nextSender uint64 = 2
	var totalGasUsed, totalGasLimit int64
	var submitted1, submitted2, submitted10, submitted20 int
	var everLate1, everLate2, everLate10, everLate20 int
	// gas-weighted sum of effective priority fee at confirmation time, per nblocksincl cohort
	var confirmedPremiumGas1, confirmedPremiumGas2, confirmedPremiumGas10, confirmedPremiumGas20 float64
	var confirmedGas1, confirmedGas2, confirmedGas10, confirmedGas20 float64
	epochHistory := make([]simEpochData, 20) // circular buffer; slot = epoch % 20

	for epoch := 0; epoch < epochs; epoch++ {
		currentEpochNum := startEpochNum + int64(epoch)
		ts := buildSimTipSet(baseFee, 0)
		now := time.Now().Unix()
		newTxsThisEpoch := 0

		if useHistory {
			// Epoch start time derived from the genesis timestamp and epoch number,
			// so each tx's offset within the epoch reflects its actual arrival time.
			epochStartMs := (genesisTimestamp + currentEpochNum*int64(buildconstants.BlockDelaySecs)) * 1000
			epochTxs := txsByEpoch[currentEpochNum]

			for _, htx := range epochTxs {
				var nblocksincl uint64
				if htx.gasLimit > largeGasTxThreshold {
					nblocksincl = 20
				} else {
					switch r := rng.Float64(); {
					case r < 0.1:
						nblocksincl = 1
					case r < 0.3:
						nblocksincl = 2
					default:
						nblocksincl = 10
					}
				}

				var premium, feeCap abi.TokenAmount
				if keepHistoricPrices {
					premium = htx.gasPremium
					feeCap = htx.gasFeeCap
				} else {
					var csTs *types.TipSet
					if epochRemainingZero {
						csTs = buildSimTipSet(baseFee, 0)
					} else {
						// Use the tx's actual position within the epoch to compute
						// a tipset timestamp that yields the correct epochFractionRemaining.
						offsetSec := (htx.arrivalMs - epochStartMs) / 1000
						if offsetSec < 0 {
							offsetSec = 0
						}
						if offsetSec >= int64(buildconstants.BlockDelaySecs) {
							offsetSec = int64(buildconstants.BlockDelaySecs) - 1
						}
						csTs = buildSimTipSet(baseFee, uint64(now-offsetSec))
					}
					cs := &simChainStore{ts: csTs}
					mp := &simMpool{msgs: mempool, ts: ts}

					switch {
					case flatPremium:
						base := big.Div(baseFee, big.NewInt(buildconstants.BaseFeeMaxChangeDenom))
						noised := int64(math.Ceil(float64(base.Int64()) * (1 + noiseRng.NormFloat64()*0.005)))
						if noised < 1 {
							noised = 1
						}
						premium = big.NewInt(noised)
					case legacyPremium:
						premium = legacyGasPremiumEstimate(epochHistory, epoch-1, nblocksincl, noiseRng)
					default:
						var err error
						premium, err = gasutils.GasEstimateGasPremiumFromMempool(ctx, cs, mp, nblocksincl, htx.gasLimit, types.EmptyTSK)
						if err != nil {
							return fmt.Errorf("epoch %d: %w", epoch, err)
						}
					}
					partialMsg := &types.Message{GasLimit: htx.gasLimit, GasPremium: premium}
					var err error
					feeCap, err = gasutils.GasEstimateFeeCap(ctx, cs, partialMsg, int64(nblocksincl), types.EmptyTSK)
					if err != nil {
						return fmt.Errorf("epoch %d fee cap: %w", epoch, err)
					}
				}

				addr, _ := address.NewIDAddress(nextSender)
				mempool = append(mempool, &types.SignedMessage{
					Message: types.Message{
						From:       addr,
						To:         addr,
						Nonce:      0,
						GasLimit:   htx.gasLimit,
						GasPremium: premium,
						GasFeeCap:  feeCap,
					},
				})
				deadlines = append(deadlines, epoch+int(nblocksincl))
				nblocksincls = append(nblocksincls, nblocksincl)
				switch nblocksincl {
				case 1:
					submitted1++
				case 2:
					submitted2++
				case 20:
					submitted20++
				default:
					submitted10++
				}
				nextSender++
				newTxsThisEpoch++
			}
		} else {
			for i := range txsPerEpoch {
				var csTs *types.TipSet
				if epochRemainingZero {
					csTs = buildSimTipSet(baseFee, 0)
				} else {
					// Descending timestamps so epochFractionRemaining returns (30-i)/30:
					// the first tx sees a near-full epoch remaining, the last sees 1/30.
					csTs = buildSimTipSet(baseFee, uint64(now-int64(i)))
				}
				cs := &simChainStore{ts: csTs}
				mp := &simMpool{msgs: mempool, ts: ts}

				gl := glDist.sample(rng)
				var nblocksincl uint64
				if gl > largeGasTxThreshold {
					nblocksincl = 20
				} else {
					switch r := rng.Float64(); {
					case r < 0.1:
						nblocksincl = 1
					case r < 0.3:
						nblocksincl = 2
					default:
						nblocksincl = 10
					}
				}
				var premium abi.TokenAmount
				switch {
				case flatPremium:
					// baseFee * R where R = 1/BaseFeeMaxChangeDenom, plus small noise.
					base := big.Div(baseFee, big.NewInt(buildconstants.BaseFeeMaxChangeDenom))
					noised := int64(math.Ceil(float64(base.Int64()) * (1 + noiseRng.NormFloat64()*0.005)))
					if noised < 1 {
						noised = 1
					}
					premium = big.NewInt(noised)
				case legacyPremium:
					premium = legacyGasPremiumEstimate(epochHistory, epoch-1, nblocksincl, noiseRng)
				default:
					var err error
					premium, err = gasutils.GasEstimateGasPremiumFromMempool(ctx, cs, mp, nblocksincl, gl, types.EmptyTSK)
					if err != nil {
						return fmt.Errorf("epoch %d tx %d: %w", epoch, i, err)
					}
				}
				partialMsg := &types.Message{GasLimit: gl, GasPremium: premium}
				feeCap, err := gasutils.GasEstimateFeeCap(ctx, cs, partialMsg, int64(nblocksincl), types.EmptyTSK)
				if err != nil {
					return fmt.Errorf("epoch %d tx %d fee cap: %w", epoch, i, err)
				}

				addr, _ := address.NewIDAddress(nextSender)
				mempool = append(mempool, &types.SignedMessage{
					Message: types.Message{
						From:       addr,
						To:         addr,
						Nonce:      0,
						GasLimit:   gl,
						GasPremium: premium,
						GasFeeCap:  feeCap,
					},
				})
				deadlines = append(deadlines, epoch+int(nblocksincl))
				nblocksincls = append(nblocksincls, nblocksincl)
				switch nblocksincl {
				case 1:
					submitted1++
				case 2:
					submitted2++
				case 20:
					submitted20++
				default:
					submitted10++
				}
				nextSender++
			}
			newTxsThisEpoch = txsPerEpoch
		}

		effPremiums := make([]abi.TokenAmount, len(mempool))
		for i, m := range mempool {
			effPremiums[i] = simEffectivePremium(m.Message.GasFeeCap, m.Message.GasPremium, baseFee)
		}

		nProposers := simSampleNProposers(rng)
		selectedByAny := make([]bool, len(mempool))
		tipPremiums := make([]abi.TokenAmount, 0, len(mempool))
		tipLimits := make([]int64, 0, len(mempool))
		for range nProposers {
			tq := rng.Float64()
			chosen := simSelectMessages(mempool, effPremiums, tq, bgl)
			for i, s := range chosen {
				if s && !selectedByAny[i] {
					selectedByAny[i] = true
					tipPremiums = append(tipPremiums, effPremiums[i])
					tipLimits = append(tipLimits, mempool[i].Message.GasLimit)
				}
			}
		}

		if nProposers > 0 {
			switch baseFeeUpdate {
			case "utilization":
				var tipGasUsedForUpdate int64
				for _, l := range tipLimits {
					tipGasUsedForUpdate += l
				}
				baseFee = store.ComputeNextBaseFee(baseFee, tipGasUsedForUpdate, nProposers, buildconstants.UpgradeXxHeight-1)
			default: // "premium"
				percentilePremium := store.WeightedQuickSelect(tipPremiums, tipLimits, buildconstants.BlockGasTargetIndex)
				baseFee = store.NextBaseFeeFromPremium(baseFee, percentilePremium)
			}
		}

		// Record raw premiums of included messages into the circular epoch history buffer.
		ed := &epochHistory[epoch%20]
		ed.premiums = ed.premiums[:0]
		ed.limits = ed.limits[:0]
		ed.nProposers = nProposers
		for i, m := range mempool {
			if selectedByAny[i] {
				ed.premiums = append(ed.premiums, m.Message.GasPremium)
				ed.limits = append(ed.limits, m.Message.GasLimit)
			}
		}

		j := 0
		for i, m := range mempool {
			if selectedByAny[i] {
				if epoch >= deadlines[i] {
					switch nblocksincls[i] {
					case 1:
						everLate1++
					case 2:
						everLate2++
					case 20:
						everLate20++
					default:
						everLate10++
					}
				}
				ep := float64(effPremiums[i].Int64())
				gl := float64(m.Message.GasLimit)
				switch nblocksincls[i] {
				case 1:
					confirmedPremiumGas1 += ep * gl
					confirmedGas1 += gl
				case 2:
					confirmedPremiumGas2 += ep * gl
					confirmedGas2 += gl
				case 20:
					confirmedPremiumGas20 += ep * gl
					confirmedGas20 += gl
				default:
					confirmedPremiumGas10 += ep * gl
					confirmedGas10 += gl
				}
			} else {
				mempool[j] = m
				deadlines[j] = deadlines[i]
				nblocksincls[j] = nblocksincls[i]
				j++
			}
		}
		mempool = mempool[:j]
		deadlines = deadlines[:j]
		nblocksincls = nblocksincls[:j]

		var tipGasUsed int64
		for _, l := range tipLimits {
			tipGasUsed += l
		}
		totalGasUsed += tipGasUsed
		tipGasLimit := int64(nProposers) * bgl
		totalGasLimit += tipGasLimit

		var delayed1, delayed2, delayed10, delayed20 int
		for i, d := range deadlines {
			if epoch >= d {
				switch nblocksincls[i] {
				case 1:
					delayed1++
				case 2:
					delayed2++
				case 20:
					delayed20++
				default:
					delayed10++
				}
			}
		}
		delayedCount := delayed1 + delayed2 + delayed10 + delayed20

		fmt.Printf("epoch %7d: newTxs=%2d baseFee=%15s  mempoolSize=%5d  delayed=%4d (nb1=%d nb2=%d nb10=%d nb20=%d)  tipGas=%d/%d (%.1f%%)\n",
			currentEpochNum, newTxsThisEpoch, baseFee, len(mempool), delayedCount, delayed1, delayed2, delayed10, delayed20,
			tipGasUsed, tipGasLimit, float64(tipGasUsed)/float64(tipGasLimit)*100)
		if dynamicDemand {
			switch r := rng.Float64(); {
			case r < 0.1:
				txsPerEpoch += 5
			case r < 0.3:
				txsPerEpoch += 2
			case r >= 0.9:
				txsPerEpoch -= 5
			case r >= 0.7:
				txsPerEpoch -= 2
			default:
				txsPerEpoch += big.Cmp(startBaseFee, baseFee)
			}
			txsPerEpoch = max(0, min(50, txsPerEpoch))
		}
		select {
		case <-cctx.Done():
			// exit loop early
			epoch = epochs
		default:
		}
	}

	// Count remaining mempool messages that were ever past their deadline.
	for i, d := range deadlines {
		if epochs-1 >= d {
			switch nblocksincls[i] {
			case 1:
				everLate1++
			case 2:
				everLate2++
			case 20:
				everLate20++
			default:
				everLate10++
			}
		}
	}
	everLateTotal := everLate1 + everLate2 + everLate10 + everLate20

	submittedTotal := submitted1 + submitted2 + submitted10 + submitted20
	pct := func(n, d int) float64 {
		if d == 0 {
			return 0
		}
		return float64(n) / float64(d) * 100
	}
	avgPremium := func(premiumGas, gas float64) float64 {
		if gas == 0 {
			return 0
		}
		return premiumGas / gas
	}
	fmt.Printf("=== SUMMARY: total gas used %d / %d (%.1f%%)  ever-late=%d/%d (%.1f%%)  (nb1=%d/%d %.1f%%  nb2=%d/%d %.1f%%  nb10=%d/%d %.1f%%  nb20=%d/%d %.1f%%)\n",
		totalGasUsed, totalGasLimit,
		float64(totalGasUsed)/float64(totalGasLimit)*100,
		everLateTotal, submittedTotal, pct(everLateTotal, submittedTotal),
		everLate1, submitted1, pct(everLate1, submitted1),
		everLate2, submitted2, pct(everLate2, submitted2),
		everLate10, submitted10, pct(everLate10, submitted10),
		everLate20, submitted20, pct(everLate20, submitted20))
	fmt.Printf("=== AVG PRIORITY FEE/GAS (confirmed only): nb1=%.2f  nb2=%.2f  nb10=%.2f  nb20=%.2f\n",
		avgPremium(confirmedPremiumGas1, confirmedGas1),
		avgPremium(confirmedPremiumGas2, confirmedGas2),
		avgPremium(confirmedPremiumGas10, confirmedGas10),
		avgPremium(confirmedPremiumGas20, confirmedGas20))

	return nil
}
