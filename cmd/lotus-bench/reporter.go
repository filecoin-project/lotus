package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

// result is the result of a single rpc method request.
type result struct {
	err        error
	statusCode *int
	duration   time.Duration
}

// Reporter reads the results from the workers through the results channel and aggregates the results.
type Reporter struct {
	// write the report to this writer
	w io.Writer
	// the reporter read the results from this channel
	results chan *result
	// doneCh is used to signal that the reporter has finished reading the results (channel has closed)
	doneCh chan bool

	// lock protect the following fields during critical sections (if --watch was specified)
	lock sync.Mutex
	// the latencies of all requests
	latencies []int64
	// the number of requests that returned each status code
	statusCodes map[int]int
	// the number of errors that occurred
	errors map[string]int
}

func NewReporter(results chan *result, w io.Writer) *Reporter {
	return &Reporter{
		w:           w,
		results:     results,
		doneCh:      make(chan bool, 1),
		statusCodes: make(map[int]int),
		errors:      make(map[string]int),
	}
}

func (r *Reporter) Run() {
	for res := range r.results {
		r.lock.Lock()

		r.latencies = append(r.latencies, res.duration.Milliseconds())

		if res.statusCode != nil {
			r.statusCodes[*res.statusCode]++
		}

		if res.err != nil {
			if len(r.errors) < 1_000_000 {
				r.errors[res.err.Error()]++
			} else {
				// we don't want to store too many errors in memory
				r.errors["hidden"]++
			}
		} else {
			r.errors["nil"]++
		}

		r.lock.Unlock()
	}

	r.doneCh <- true
}

func (r *Reporter) Print(elapsed time.Duration, w io.Writer) {
	r.lock.Lock()
	defer r.lock.Unlock()

	nrReq := int64(len(r.latencies))
	if nrReq == 0 {
		fmt.Println("No requests were made")
		return
	}

	// we need to sort the latencies slice to calculate the percentiles
	sort.Slice(r.latencies, func(i, j int) bool {
		return r.latencies[i] < r.latencies[j]
	})

	var totalLatency int64
	for _, latency := range r.latencies {
		totalLatency += latency
	}

	fmt.Fprintf(w, "- Total Requests: %d\n", nrReq)                          //nolint:errcheck
	fmt.Fprintf(w, "- Total Duration: %dms\n", elapsed.Milliseconds())       //nolint:errcheck
	fmt.Fprintf(w, "- Requests/sec: %f\n", float64(nrReq)/elapsed.Seconds()) //nolint:errcheck
	fmt.Fprintf(w, "- Avg latency: %dms\n", totalLatency/nrReq)              //nolint:errcheck
	fmt.Fprintf(w, "- Median latency: %dms\n", r.latencies[nrReq/2])         //nolint:errcheck
	fmt.Fprintf(w, "- Latency distribution:\n")                              //nolint:errcheck
	percentiles := []float64{0.1, 0.5, 0.9, 0.95, 0.99, 0.999}
	for _, p := range percentiles {
		idx := int64(p * float64(nrReq))
		_, _ = fmt.Fprintf(w, "    %s%% in %dms\n", fmt.Sprintf("%.2f", p*100.0), r.latencies[idx])
	}

	// create a simple histogram with 10 buckets spanning the range of latency
	// into equal ranges
	//
	nrBucket := 10
	buckets := make([]Bucket, nrBucket)
	latencyRange := r.latencies[len(r.latencies)-1]
	bucketRange := latencyRange / int64(nrBucket)

	// mark the end of each bucket
	for i := 0; i < nrBucket; i++ {
		buckets[i].start = int64(i) * bucketRange
		buckets[i].end = buckets[i].start + bucketRange
		// extend the last bucked by any remaining range caused by the integer division
		if i == nrBucket-1 {
			buckets[i].end = latencyRange
		}
	}

	// count the number of requests in each bucket
	currBucket := 0
	for i := 0; i < len(r.latencies); {
		if r.latencies[i] <= buckets[currBucket].end {
			buckets[currBucket].cnt++
			i++
		} else {
			currBucket++
		}
	}

	// print the histogram using a tabwriter which will align the columns nicely
	_, _ = fmt.Fprintf(w, "- Histogram:\n")
	const padding = 2
	tabWriter := tabwriter.NewWriter(w, 0, 0, padding, ' ', tabwriter.AlignRight|tabwriter.Debug)
	for i := 0; i < nrBucket; i++ {
		ratio := float64(buckets[i].cnt) / float64(nrReq)
		bars := strings.Repeat("#", int(ratio*100))
		_, _ = fmt.Fprintf(tabWriter, "  %d-%dms\t%d\t%s (%s%%)\n", buckets[i].start, buckets[i].end, buckets[i].cnt, bars, fmt.Sprintf("%.2f", ratio*100))
	}
	tabWriter.Flush() //nolint:errcheck

	_, _ = fmt.Fprintf(w, "- Status codes:\n")
	for code, cnt := range r.statusCodes {
		_, _ = fmt.Fprintf(w, "    [%d]: %d\n", code, cnt)
	}

	// print the 10 most occurring errors (in case error values are not unique)
	//
	type kv struct {
		err string
		cnt int
	}
	var sortedErrors []kv
	for err, cnt := range r.errors {
		sortedErrors = append(sortedErrors, kv{err, cnt})
	}
	sort.Slice(sortedErrors, func(i, j int) bool {
		return sortedErrors[i].cnt > sortedErrors[j].cnt
	})
	_, _ = fmt.Fprintf(w, "- Errors (top 10):\n")
	for i, se := range sortedErrors {
		if i > 10 {
			break
		}
		_, _ = fmt.Fprintf(w, "    [%s]: %d\n", se.err, se.cnt)
	}
}

type Bucket struct {
	start int64
	// the end value of the bucket
	end int64
	// how many entries are in the bucket
	cnt int
}
