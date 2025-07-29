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
	"strconv"
	"strings"
	"sync"
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

  --method=NAME[:CONCURRENCY][:QPS][:PARAMS] where only NAME is required.

Here are some real examples:
  lotus-bench rpc --method='eth_chainId' // run eth_chainId with default concurrency and qps
  lotus-bench rpc --method='eth_chainId:3'  // override concurrency to 3
  lotus-bench rpc --method='eth_chainId::100' // override to 100 qps while using default concurrency
  lotus-bench rpc --method='eth_chainId:3:100' // run using 3 workers but limit to 100 qps
  lotus-bench rpc --method='eth_getTransactionCount:::["0xd4c70007F3F502f212c7e6794b94C06F36173B36", "latest"]' // run using optional params while using default concurrency and qps
  lotus-bench rpc --method='eth_chainId' --method='eth_getTransactionCount:10:0:["0xd4c70007F3F502f212c7e6794b94C06F36173B36", "latest"]' // run multiple methods at once

NOTE: The last two examples will not work until we upgrade urfave dependency (tracked in https://github.com/urfave/cli/issues/1734)`,
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
		&cli.DurationFlag{
			Name:  "watch",
			Value: 0 * time.Second,
			Usage: "If >0 then generates reports every N seconds (only supports linux/unix)",
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
			entries := strings.SplitN(str, ":", 4)
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
				params = entries[3]
			}

			rpcMethods = append(rpcMethods, &RPCMethod{
				w:           os.Stdout,
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
				defer wg.Done()
				err := e.Run()
				if err != nil {
					fmt.Printf("error running rpc method: %v\n", err)
				}
			}(e)
		}

		// if watch is set then print a report every N seconds
		var progressCh chan struct{}
		if cctx.Duration("watch") > 0 {
			progressCh = make(chan struct{}, 1)
			go func(progressCh chan struct{}) {
				ticker := time.NewTicker(cctx.Duration("watch"))
				for {
					clearAndPrintReport := func() {
						// clear the screen move the cursor to the top left
						fmt.Print("\033[2J")
						fmt.Printf("\033[%d;%dH", 1, 1)
						for i, e := range rpcMethods {
							e.Report()
							if i < len(rpcMethods)-1 {
								fmt.Println()
							}
						}
					}
					select {
					case <-ticker.C:
						clearAndPrintReport()
					case <-progressCh:
						clearAndPrintReport()
						return
					}
				}
			}(progressCh)
		}

		wg.Wait()

		if progressCh != nil {
			// wait for the watch go routine to return
			progressCh <- struct{}{}

			// no need to print the report again
			return nil
		}

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
	w io.Writer
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

func (rpc *RPCMethod) Run() error {
	client := &http.Client{
		Timeout: 0,
	}

	var wg sync.WaitGroup
	wg.Add(rpc.concurrency)

	rpc.results = make(chan *result, rpc.concurrency*1_000)
	rpc.stopCh = make(chan struct{}, rpc.concurrency)

	go func() {
		rpc.reporter = NewReporter(rpc.results, rpc.w)
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
		if err != nil {
			err = fmt.Errorf("HTTP error: %s", err.Error())
		} else {
			statusCode = &resp.StatusCode

			// there was not a HTTP error but we need to still check the json response for errors
			var data []byte
			data, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Fatalln(err)
			}

			// we are only interested if it has the error field in the response
			type respData struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}

			// unmarshal the response into a struct so we can check for errors
			var d respData
			err = json.Unmarshal(data, &d)
			if err != nil {
				log.Fatalln(err)
			}

			// if the response has an error json message then it should be considered an error just like any http error
			if len(d.Error.Message) > 0 {
				// truncate the error message if it is too long
				if len(d.Error.Message) > 1000 {
					d.Error.Message = d.Error.Message[:1000] + "..."
				}
				// remove newlines from the error message so we don't screw up the report
				d.Error.Message = strings.ReplaceAll(d.Error.Message, "\n", "")

				err = fmt.Errorf("JSON error: code:%d, message:%s", d.Error.Code, d.Error.Message)
			}

			if rpc.printResp {
				fmt.Printf("[%s] %s", rpc.method, string(data))
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
	fmt.Fprintf(rpc.w, "[%s]:\n", rpc.method)                    //nolint:errcheck
	fmt.Fprintf(rpc.w, "- Options:\n")                           //nolint:errcheck
	fmt.Fprintf(rpc.w, "  - concurrency: %d\n", rpc.concurrency) //nolint:errcheck
	fmt.Fprintf(rpc.w, "  - params: %s\n", rpc.params)           //nolint:errcheck
	fmt.Fprintf(rpc.w, "  - qps: %d\n", rpc.qps)                 //nolint:errcheck
	rpc.reporter.Print(total, rpc.w)
}
