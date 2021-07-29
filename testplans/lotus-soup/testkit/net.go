package testkit

import (
	"context"
	"fmt"
	"time"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/sync"
)

func ApplyNetworkParameters(t *TestEnvironment) {
	if !t.TestSidecar {
		t.RecordMessage("no test sidecar, skipping network config")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ls := network.LinkShape{}

	if t.IsParamSet("latency_range") {
		r := t.DurationRangeParam("latency_range")
		ls.Latency = r.ChooseRandom()
		t.D().RecordPoint("latency_ms", float64(ls.Latency.Milliseconds()))
	}

	if t.IsParamSet("jitter_range") {
		r := t.DurationRangeParam("jitter_range")
		ls.Jitter = r.ChooseRandom()
		t.D().RecordPoint("jitter_ms", float64(ls.Jitter.Milliseconds()))
	}

	if t.IsParamSet("loss_range") {
		r := t.FloatRangeParam("loss_range")
		ls.Loss = r.ChooseRandom()
		t.D().RecordPoint("packet_loss", float64(ls.Loss))
	}

	if t.IsParamSet("corrupt_range") {
		r := t.FloatRangeParam("corrupt_range")
		ls.Corrupt = r.ChooseRandom()
		t.D().RecordPoint("corrupt_packet_probability", float64(ls.Corrupt))
	}

	if t.IsParamSet("corrupt_corr_range") {
		r := t.FloatRangeParam("corrupt_corr_range")
		ls.CorruptCorr = r.ChooseRandom()
		t.D().RecordPoint("corrupt_packet_correlation", float64(ls.CorruptCorr))
	}

	if t.IsParamSet("reorder_range") {
		r := t.FloatRangeParam("reorder_range")
		ls.Reorder = r.ChooseRandom()
		t.D().RecordPoint("reordered_packet_probability", float64(ls.Reorder))
	}

	if t.IsParamSet("reorder_corr_range") {
		r := t.FloatRangeParam("reorder_corr_range")
		ls.ReorderCorr = r.ChooseRandom()
		t.D().RecordPoint("reordered_packet_correlation", float64(ls.ReorderCorr))
	}

	if t.IsParamSet("duplicate_range") {
		r := t.FloatRangeParam("duplicate_range")
		ls.Duplicate = r.ChooseRandom()
		t.D().RecordPoint("duplicate_packet_probability", float64(ls.Duplicate))
	}

	if t.IsParamSet("duplicate_corr_range") {
		r := t.FloatRangeParam("duplicate_corr_range")
		ls.DuplicateCorr = r.ChooseRandom()
		t.D().RecordPoint("duplicate_packet_correlation", float64(ls.DuplicateCorr))
	}

	if t.IsParamSet("bandwidth") {
		ls.Bandwidth = t.SizeParam("bandwidth")
		t.D().RecordPoint("bandwidth_bytes", float64(ls.Bandwidth))
	}

	t.NetClient.MustConfigureNetwork(ctx, &network.Config{
		Network:        "default",
		Enable:         true,
		Default:        ls,
		CallbackState:  sync.State(fmt.Sprintf("latency-configured-%s", t.TestGroupID)),
		CallbackTarget: t.TestGroupInstanceCount,
		RoutingPolicy:  network.AllowAll,
	})

	t.DumpJSON("network-link-shape.json", ls)
}
