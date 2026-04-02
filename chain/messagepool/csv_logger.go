package messagepool

import (
	"encoding/csv"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type mempoolCSVLogger struct {
	mu sync.Mutex
	w  *csv.Writer
	f  *os.File
}

func newMempoolCSVLogger(path string) (*mempoolCSVLogger, error) {
	needsHeader := true
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		needsHeader = false
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	w := csv.NewWriter(f)
	if needsHeader {
		_ = w.Write([]string{
			"arrival_timestamp_ms",
			"epoch",
			"next_base_fee",
			"sender",
			"sender_nonce",
			"tx_nonce",
			"gas_fee_cap",
			"gas_premium",
			"gas_limit",
		})
		w.Flush()
	}

	return &mempoolCSVLogger{w: w, f: f}, nil
}

func (l *mempoolCSVLogger) logTx(
	epoch abi.ChainEpoch,
	nextBaseFee types.BigInt,
	sender address.Address,
	senderNonce uint64,
	txNonce uint64,
	gasFeeCap types.BigInt,
	gasPremium types.BigInt,
	gasLimit int64,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = l.w.Write([]string{
		strconv.FormatInt(time.Now().UnixMilli(), 10),
		strconv.FormatInt(int64(epoch), 10),
		nextBaseFee.String(),
		sender.String(),
		strconv.FormatUint(senderNonce, 10),
		strconv.FormatUint(txNonce, 10),
		gasFeeCap.String(),
		gasPremium.String(),
		strconv.FormatInt(gasLimit, 10),
	})
	l.w.Flush()
}

func (l *mempoolCSVLogger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Flush()
	_ = l.f.Close()
}
