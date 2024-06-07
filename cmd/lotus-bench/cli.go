package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v2"
)

var cliCmd = &cli.Command{
	Name:  "cli",
	Usage: "Runs a concurrent stress test on one or more binaries commands and prints the performance metrics including latency distribution and histogram",
	Description: `This benchmark has the following features:
* Can query each command both sequentially and concurrently
* Supports rate limiting
* Can query multiple different commands at once (supporting different concurrency level and rate limiting for each command)
* Gives a nice reporting summary of the stress testing of each command (including latency distribution, histogram and more)
* Easy to use

To use this benchmark you must specify the commands you want to test using the --cmd options, the format of it is:

  --cmd=CMD[:CONCURRENCY][:QPS] where only NAME is required.

Here are some real examples:
  lotus-bench cli --cmd='lotus-shed mpool miner-select-messages' // runs the command with default concurrency and qps
  lotus-bench cli --cmd='lotus-shed mpool miner-select-messages:3'  // override concurrency to 3
  lotus-bench cli --cmd='lotus-shed mpool miner-select-messages::100' // override to 100 qps while using default concurrency
  lotus-bench cli --cmd='lotus-shed mpool miner-select-messages:3:100' // run using 3 workers but limit to 100 qps
  lotus-bench cli --cmd='lotus-shed mpool miner-select-messages' --cmd='lotus sync wait' // run two commands at once
`,
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Name:  "duration",
			Value: 60 * time.Second,
			Usage: "Duration of benchmark in seconds",
		},
		&cli.IntFlag{
			Name:  "concurrency",
			Value: 10,
			Usage: "How many workers should be used per command (can be overridden per command)",
		},
		&cli.IntFlag{
			Name:  "qps",
			Value: 0,
			Usage: "How many requests per second should be sent per command (can be overridden per command), a value of 0 means no limit",
		},
		&cli.StringSliceFlag{
			Name:  "cmd",
			Usage: `Command to benchmark, you can specify multiple commands by repeating this flag. You can also specify command specific options to set the concurrency and qps for each command (see usage).`,
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
		if len(cctx.StringSlice("cmd")) == 0 {
			return errors.New("you must specify and least one cmd to benchmark")
		}

		var cmds []*CMD
		for _, str := range cctx.StringSlice("cmd") {
			entries := strings.SplitN(str, ":", 3)
			if len(entries) == 0 {
				return errors.New("invalid cmd format")
			}

			// check if concurrency was specified
			concurrency := cctx.Int("concurrency")
			if len(entries) > 1 {
				if len(entries[1]) > 0 {
					var err error
					concurrency, err = strconv.Atoi(entries[1])
					if err != nil {
						return fmt.Errorf("could not parse concurrency value from command %s: %v", entries[0], err)
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
						return fmt.Errorf("could not parse qps value from command %s: %v", entries[0], err)
					}
				}
			}

			cmds = append(cmds, &CMD{
				w:           os.Stdout,
				cmd:         entries[0],
				concurrency: concurrency,
				qps:         qps,
				printResp:   cctx.Bool("print-response"),
			})
		}

		// terminate early on ctrl+c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			fmt.Println("Received interrupt, stopping...")
			for _, cmd := range cmds {
				cmd.Stop()
			}
		}()

		// stop all threads after duration
		go func() {
			time.Sleep(cctx.Duration("duration"))
			for _, cmd := range cmds {
				cmd.Stop()
			}
		}()

		// start all threads
		var wg sync.WaitGroup
		wg.Add(len(cmds))

		for _, cmd := range cmds {
			go func(cmd *CMD) {
				defer wg.Done()
				err := cmd.Run()
				if err != nil {
					fmt.Printf("error running cmd: %v\n", err)
				}
			}(cmd)
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
						for i, cmd := range cmds {
							cmd.Report()
							if i < len(cmds)-1 {
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

		// print the report for each command
		for i, cmd := range cmds {
			cmd.Report()
			if i < len(cmds)-1 {
				fmt.Println()
			}
		}

		return nil
	},
}

// CMD handles the benchmarking of a single command.
type CMD struct {
	w io.Writer
	// the cmd we want to benchmark
	cmd string
	// the number of concurrent requests to make to this command
	concurrency int
	// if >0 then limit to qps is the max number of requests per second to make to this command (0 = no limit)
	qps int
	// whether or not to print the response of each request (useful for debugging)
	printResp bool
	// instruct the worker go routines to stop
	stopCh chan struct{}
	// when the command bencharking started
	start time.Time
	// results channel is used by the workers to send results to the reporter
	results chan *result
	// reporter handles reading the results from workers and printing the report statistics
	reporter *Reporter
}

func (c *CMD) Run() error {
	var wg sync.WaitGroup
	wg.Add(c.concurrency)

	c.results = make(chan *result, c.concurrency*1_000)
	c.stopCh = make(chan struct{}, c.concurrency)

	go func() {
		c.reporter = NewReporter(c.results, c.w)
		c.reporter.Run()
	}()

	c.start = time.Now()

	// throttle the number of requests per second
	var qpsTicker *time.Ticker
	if c.qps > 0 {
		qpsTicker = time.NewTicker(time.Second / time.Duration(c.qps))
	}

	for i := 0; i < c.concurrency; i++ {
		go func() {
			c.startWorker(qpsTicker)
			wg.Done()
		}()
	}
	wg.Wait()

	// close the results channel so reporter will stop
	close(c.results)

	// wait until the reporter is done
	<-c.reporter.doneCh

	return nil
}

func (c *CMD) startWorker(qpsTicker *time.Ticker) {
	for {
		// check if we should stop
		select {
		case <-c.stopCh:
			return
		default:
		}

		// wait for the next tick if we are rate limiting this command
		if qpsTicker != nil {
			<-qpsTicker.C
		}

		start := time.Now()

		var statusCode int

		arr := strings.Fields(c.cmd)

		data, err := exec.Command(arr[0], arr[1:]...).Output()
		if err != nil {
			fmt.Println("1")
			if exitError, ok := err.(*exec.ExitError); ok {
				statusCode = exitError.ExitCode()
			} else {
				statusCode = 1
			}
		} else {
			if c.printResp {
				fmt.Printf("[%s] %s", c.cmd, string(data))
			}
		}

		c.results <- &result{
			statusCode: &statusCode,
			err:        err,
			duration:   time.Since(start),
		}
	}
}

func (c *CMD) Stop() {
	for i := 0; i < c.concurrency; i++ {
		c.stopCh <- struct{}{}
	}
}

func (c *CMD) Report() {
	total := time.Since(c.start)
	fmt.Fprintf(c.w, "[%s]:\n", c.cmd)                       //nolint:errcheck
	fmt.Fprintf(c.w, "- Options:\n")                         //nolint:errcheck
	fmt.Fprintf(c.w, "  - concurrency: %d\n", c.concurrency) //nolint:errcheck
	fmt.Fprintf(c.w, "  - qps: %d\n", c.qps)                 //nolint:errcheck
	c.reporter.Print(total, c.w)
}
