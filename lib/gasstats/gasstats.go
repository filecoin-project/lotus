// Package gasstats aggregates per-charge-name gas totals from an
// [types.ExecutionTrace] and formats them for human consumption.
package gasstats

import (
	"fmt"
	"io"
	"sort"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

// Tally is the aggregated gas for all charges with a given name.
type Tally struct {
	Name       string
	ComputeGas int64
	StorageGas int64
	Count      int
}

// Total returns the sum of compute and storage gas.
func (t Tally) Total() int64 {
	return t.ComputeGas + t.StorageGas
}

// Aggregate walks trace and returns one Tally per unique charge name plus a
// combined total. If recurse is true, subcalls are included; otherwise only
// the top-level trace's charges are counted.
//
// The returned tallies are sorted by name for stable output.
func Aggregate(trace types.ExecutionTrace, recurse bool) (tallies []Tally, total Tally) {
	byName := map[string]*Tally{}
	accumulate(byName, &total, trace, recurse)

	tallies = make([]Tally, 0, len(byName))
	for _, t := range byName {
		tallies = append(tallies, *t)
	}
	sort.Slice(tallies, func(i, j int) bool { return tallies[i].Name < tallies[j].Name })
	return tallies, total
}

func accumulate(byName map[string]*Tally, total *Tally, trace types.ExecutionTrace, recurse bool) {
	for _, charge := range trace.GasCharges {
		t, ok := byName[charge.Name]
		if !ok {
			t = &Tally{Name: charge.Name}
			byName[charge.Name] = t
		}
		t.ComputeGas += charge.ComputeGas
		t.StorageGas += charge.StorageGas
		t.Count++
		total.ComputeGas += charge.ComputeGas
		total.StorageGas += charge.StorageGas
		total.Count++
	}
	if recurse {
		for _, sub := range trace.Subcalls {
			accumulate(byName, total, sub, recurse)
		}
	}
}

// FormatTable renders tallies as a bordered table with per-column percentages
// of the supplied totals. A final "Total" row shows the grand total.
func FormatTable(w io.Writer, tallies []Tally, total Tally) error {
	tw := tablewriter.New(
		tablewriter.Col("Type"),
		tablewriter.Col("Count", tablewriter.RightAlign()),
		tablewriter.Col("Storage Gas", tablewriter.RightAlign()),
		tablewriter.Col("S%", tablewriter.RightAlign()),
		tablewriter.Col("Compute Gas", tablewriter.RightAlign()),
		tablewriter.Col("C%", tablewriter.RightAlign()),
		tablewriter.Col("Total Gas", tablewriter.RightAlign()),
		tablewriter.Col("T%", tablewriter.RightAlign()),
	)

	pct := func(num, den int64) string {
		if den == 0 {
			return "0.00"
		}
		return fmt.Sprintf("%.2f", float64(num)/float64(den)*100)
	}

	for _, t := range tallies {
		tw.Write(map[string]any{
			"Type":        t.Name,
			"Count":       t.Count,
			"Storage Gas": t.StorageGas,
			"S%":          pct(t.StorageGas, total.StorageGas),
			"Compute Gas": t.ComputeGas,
			"C%":          pct(t.ComputeGas, total.ComputeGas),
			"Total Gas":   t.Total(),
			"T%":          pct(t.Total(), total.Total()),
		})
	}
	tw.Write(map[string]any{
		"Type":        "Total",
		"Count":       total.Count,
		"Storage Gas": total.StorageGas,
		"S%":          pct(total.StorageGas, total.StorageGas),
		"Compute Gas": total.ComputeGas,
		"C%":          pct(total.ComputeGas, total.ComputeGas),
		"Total Gas":   total.Total(),
		"T%":          pct(total.Total(), total.Total()),
	})
	return tw.Flush(w, tablewriter.WithBorders())
}

// TopN returns the N tallies with the largest Total(), preserving descending
// order. If fewer than N tallies exist, all are returned. N <= 0 returns nil.
func TopN(tallies []Tally, n int) []Tally {
	if n <= 0 {
		return nil
	}
	sorted := make([]Tally, len(tallies))
	copy(sorted, tallies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Total() > sorted[j].Total()
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// PerCallTrace transforms an ExecutionTrace into a synthetic flat trace where
// each subcall's aggregated gas becomes a single named GasTrace. The returned
// trace is only suitable for rendering via Aggregate/FormatTable; it is not a
// valid execution trace.
func PerCallTrace(in types.ExecutionTrace) types.ExecutionTrace {
	out := types.ExecutionTrace{GasCharges: []*types.GasTrace{}}
	count := 1
	var visit func(name string, trace types.ExecutionTrace)
	visit = func(name string, trace types.ExecutionTrace) {
		_, total := Aggregate(trace, false)
		out.GasCharges = append(out.GasCharges, &types.GasTrace{
			Name:       fmt.Sprintf("#%d %s", count, name),
			ComputeGas: total.ComputeGas,
			StorageGas: total.StorageGas,
		})
		count++
		for _, sub := range trace.Subcalls {
			visit(name+"➜"+sub.Msg.To.String(), sub)
		}
	}
	visit(in.Msg.To.String(), in)
	return out
}
