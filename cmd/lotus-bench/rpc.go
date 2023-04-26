package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
)

var rpcCmd = &cli.Command{
	Name:  "rpc",
	Usage: "Runs a concurrent stress test on one or more rpc methods and prints the performance metrics including latency distribution and histogram",
	Description: `This benchmark is designed to stress test the rpc methods of a lotus node so that we can simulate real world usage and measure the performance of rpc methods on the node.
	
This benchmark has the following features:
* Can query each method both sequentially and concurrently
* Supports rate limiting
* Can query multiple different endpoints at once (supporting different concurrency level and rate limiting for each method)
* Gives a nice reporting summary of the stress testing of each method (including latency distribution, histogram and more)
* Easy to use

To use this benchmark you must specify the rpc methods you want to test using the --method options, the format of it is:

  --method=NAME[:CONCURRENCY][:QPS][:PARAMS] where only METHOD is required.

Here are some real examples:
  lotus-bench rpc --method='eth_chainId' // run eth_chainId with default concurrency and qps
  lotus-bench rpc --method='eth_chainId:3'  // override concurrency to 3
  lotus-bench rpc --method='eth_chainId::100' // override to 100 qps while using default concurrency
  lotus-bench rpc --method='eth_chainId:3:100' // run using 3 workers but limit to 100 qps
  lotus-bench rpc --method='eth_getTransactionCount:::["0xd4c70007F3F502f212c7e6794b94C06F36173B36", "latest"]' // run using optional params while using default concurrency and qps
  lotus-bench rpc --method='eth_chainId' --method='eth_getTransactionCount:10:0:["0xd4c70007F3F502f212c7e6794b94C06F36173B36", "latest"]' // run multiple methods at once`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "endpoint",
			Value: "http://127.0.0.1:1234/rpc/v1",
			Usage: "The rpc endpoint to benchmark",
		},
		&cli.DurationFlag{
			Name:  "duration",
			Value: 60 * time.Second,
			Usage: "Duration of benchmark in seconds",
		},
		&cli.IntFlag{
			Name:  "concurrency",
			Value: 10,
			Usage: "How many workers should be used per rpc method (can be overridden per method)",
		},
		&cli.IntFlag{
			Name:  "qps",
			Value: 0,
			Usage: "How many requests per second should be sent per rpc method (can be overridden per method), a value of 0 means no limit",
		},
		&cli.StringSliceFlag{
			Name: "method",
			Usage: `Method to benchmark, you can specify multiple methods by repeating this flag. You can also specify method specific options to set the concurrency and qps for each method (see usage).
`,
		},
		&cli.BoolFlag{
			Name:  "print-response",
			Value: false,
			Usage: "print the response of each request",
		},
	},
	Action: func(cctx *cli.Context) error {
		if len(cctx.StringSlice("method")) == 0 {
			return errors.New("you must specify and least one method to benchmark")
		}

		var rpcMethods []*RPCMethod
		for _, str := range cctx.StringSlice("method") {
			entries := strings.Split(str, ":")
			if len(entries) == 0 {
				return errors.New("invalid method format")
			}

			// check if concurrency was specified
			concurrency := cctx.Int("concurrency")
			if len(entries) > 1 {
				if len(entries[1]) > 0 {
					var err error
					concurrency, err = strconv.Atoi(entries[1])
					if err != nil {
						return fmt.Errorf("could not parse concurrency value from method %s: %v", entries[0], err)
					}
				}
			}

			// check if qps was specified
			qps := cctx.Int("qps")
			if len(entries) > 2 {
				if len(entries[2]) > 0 {
					var err error
					qps, err = strconv.Atoi(entries[2])
					if err != nil {
						return fmt.Errorf("could not parse qps value from method %s: %v", entries[0], err)
					}
				}
			}

			// check if params was specified
			params := "[]"
			if len(entries) > 3 {
				// the params are everything after the 3rd ':' character in str. Since params can itself
				// contain the ':' characters we can't just split on ':', so instead we need to locate the
				// index of the 3rd ':' character and then take everything after that
				occur := 0
				idx := -1
				for i := 0; i < len(str); i++ {
					if str[i] == ':' {
						occur++
						if occur == 3 {
							idx = i
							break
						}
					}
				}
				if idx == -1 {
					log.Fatalf("could not parse the params from method %s", entries[0])
				}

				params = str[idx+1:]
			}

			rpcMethods = append(rpcMethods, &RPCMethod{
				uri:         cctx.String("endpoint"),
				method:      entries[0],
				concurrency: concurrency,
				qps:         qps,
				params:      params,
				printResp:   cctx.Bool("print-response"),
			})
		}

		// terminate early on ctrl+c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			fmt.Println("Received interrupt, stopping...")
			for _, method := range rpcMethods {
				method.Stop()
			}
		}()

		// stop all threads after duration
		go func() {
			time.Sleep(cctx.Duration("duration"))
			for _, e := range rpcMethods {
				e.Stop()
			}
		}()

		// start all threads
		var wg sync.WaitGroup
		wg.Add(len(rpcMethods))

		for _, e := range rpcMethods {
			go func(e *RPCMethod) {
				err := e.Run()
				if err != nil {
					fmt.Printf("error running rpc method: %v\n", err)
				}
				wg.Done()
			}(e)
		}

		wg.Wait()

		// print the report for each endpoint
		for i, e := range rpcMethods {
			e.Report()
			if i < len(rpcMethods)-1 {
				fmt.Println()
			}
		}

		return nil
	},
}

// RPCMethod handles the benchmarking of a single endpoint method.
type RPCMethod struct {
	// the endpoint uri
	uri string
	// the rpc method we want to benchmark
	method string
	// the number of concurrent requests to make to this endpoint
	concurrency int
	// if >0 then limit to qps is the max number of requests per second to make to this endpoint (0 = no limit)
	qps int
	// many endpoints require specific parameters to be passed
	params string
	// whether or not to print the response of each request (useful for debugging)
	printResp bool
	// instruct the worker go routines to stop
	stopCh chan struct{}
	// when the endpoint bencharking started
	start time.Time
	// results channel is used by the workers to send results to the reporter
	results chan *result
	// reporter handles reading the results from workers and printing the report statistics
	reporter *Reporter
}

// result is the result of a single rpc method request.
type result struct {
	err        error
	statusCode *int
	duration   time.Duration
}

func (rpc *RPCMethod) Run() error {
	client := &http.Client{
		Timeout: 0,
	}

	var wg sync.WaitGroup
	wg.Add(rpc.concurrency)

	rpc.results = make(chan *result, rpc.concurrency*1_000)
	rpc.stopCh = make(chan struct{}, rpc.concurrency)

	go func() {
		rpc.reporter = NewReporter(rpc.results)
		rpc.reporter.Run()
	}()

	rpc.start = time.Now()

	// throttle the number of requests per second
	var qpsTicker *time.Ticker
	if rpc.qps > 0 {
		qpsTicker = time.NewTicker(time.Second / time.Duration(rpc.qps))
	}

	for i := 0; i < rpc.concurrency; i++ {
		go func() {
			rpc.startWorker(client, qpsTicker)
			wg.Done()
		}()
	}
	wg.Wait()

	// close the results channel so reporter will stop
	close(rpc.results)

	// wait until the reporter is done
	<-rpc.reporter.doneCh

	return nil
}

func (rpc *RPCMethod) startWorker(client *http.Client, qpsTicker *time.Ticker) {
	for {
		// check if we should stop
		select {
		case <-rpc.stopCh:
			return
		default:
		}

		// wait for the next tick if we are rate limiting this endpoint
		if qpsTicker != nil {
			<-qpsTicker.C
		}

		req, err := rpc.buildRequest()
		if err != nil {
			log.Fatalln(err)
		}

		start := time.Now()

		var statusCode *int

		// send request the endpoint
		resp, err := client.Do(req)
		if err == nil {
			statusCode = &resp.StatusCode
			if rpc.printResp {
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Fatalln(err)
				}
				fmt.Printf("[%s] %s", rpc.method, string(b))
			} else {
				io.Copy(io.Discard, resp.Body) //nolint:errcheck
			}
			resp.Body.Close() //nolint:errcheck
		}

		rpc.results <- &result{
			statusCode: statusCode,
			err:        err,
			duration:   time.Since(start),
		}
	}
}

func (rpc *RPCMethod) buildRequest() (*http.Request, error) {
	jreq, err := json.Marshal(struct {
		Jsonrpc string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{
		Jsonrpc: "2.0",
		Method:  rpc.method,
		Params:  json.RawMessage(rpc.params),
		ID:      0,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", rpc.uri, bytes.NewReader(jreq))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	return req, nil
}

func (rpc *RPCMethod) Stop() {
	for i := 0; i < rpc.concurrency; i++ {
		rpc.stopCh <- struct{}{}
	}
}

func (rpc *RPCMethod) Report() {
	total := time.Since(rpc.start)
	fmt.Printf("[%s]:\n", rpc.method)
	fmt.Printf("- Options:\n")
	fmt.Printf("  - concurrency: %d\n", rpc.concurrency)
	fmt.Printf("  - params: %s\n", rpc.params)
	fmt.Printf("  - qps: %d\n", rpc.qps)
	rpc.reporter.Print(total)
}

// Reporter reads the results from the workers through the results channel and aggregates the results.
type Reporter struct {
	// the reporter read the results from this channel
	results chan *result
	// doneCh is used to signal that the reporter has finished reading the results (channel has closed)
	doneCh chan bool
	// the latencies of all requests
	latencies []int64
	// the number of requests that returned each status code
	statusCodes map[int]int
	// the number of errors that occurred
	errors map[string]int
}

func NewReporter(results chan *result) *Reporter {
	return &Reporter{
		results:     results,
		doneCh:      make(chan bool, 1),
		statusCodes: make(map[int]int),
		errors:      make(map[string]int),
	}
}

func (r *Reporter) Run() {
	for res := range r.results {
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
	}

	r.doneCh <- true
}

func (r *Reporter) Print(elapsed time.Duration) {
	nrReq := int64(len(r.latencies))
	if nrReq == 0 {
		fmt.Println("No requests were made")
		return
	}

	// we need to sort the latencies slice to calculate the percentiles
	sort.Slice(r.latencies, func(i, j int) bool {
		return r.latencies[i] < r.latencies[j]
	})

	var totalLatency int64 = 0
	for _, latency := range r.latencies {
		totalLatency += latency
	}

	fmt.Printf("- Total Requests: %d\n", nrReq)
	fmt.Printf("- Total Duration: %dms\n", elapsed.Milliseconds())
	fmt.Printf("- Requests/sec: %f\n", float64(nrReq)/elapsed.Seconds())
	fmt.Printf("- Avg latency: %dms\n", totalLatency/nrReq)
	fmt.Printf("- Median latency: %dms\n", r.latencies[nrReq/2])
	fmt.Printf("- Latency distribution:\n")
	percentiles := []float64{0.1, 0.5, 0.9, 0.95, 0.99, 0.999}
	for _, p := range percentiles {
		idx := int64(p * float64(nrReq))
		fmt.Printf("    %s%% in %dms\n", fmt.Sprintf("%.2f", p*100.0), r.latencies[idx])
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
		// extend the last bucked by any remaning range caused by the integer division
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
	fmt.Printf("- Histogram:\n")
	const padding = 2
	w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, ' ', tabwriter.AlignRight|tabwriter.Debug)
	for i := 0; i < nrBucket; i++ {
		ratio := float64(buckets[i].cnt) / float64(nrReq)
		bars := strings.Repeat("#", int(ratio*100))
		fmt.Fprintf(w, "  %d-%dms\t%d\t%s (%s%%)\n", buckets[i].start, buckets[i].end, buckets[i].cnt, bars, fmt.Sprintf("%.2f", ratio*100))
	}
	w.Flush() //nolint:errcheck

	fmt.Printf("- Status codes:\n")
	for code, cnt := range r.statusCodes {
		fmt.Printf("    [%d]: %d\n", code, cnt)
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
	fmt.Printf("- Errors (top 10):\n")
	for i, se := range sortedErrors {
		if i > 10 {
			break
		}
		fmt.Printf("    [%s]: %d\n", se.err, se.cnt)
	}
}

type Bucket struct {
	start int64
	// the end value of the bucket
	end int64
	// how many entries are in the bucket
	cnt int
}
