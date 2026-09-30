package main

import "testing"

// TestParseCmdEntryRejectsEmptyCommand covers the values that used to reach
// startWorker with an empty command, where strings.Fields returned an empty
// slice and indexing it panicked. They must now fail validation instead.
func TestParseCmdEntryRejectsEmptyCommand(t *testing.T) {
	const wantErr = "command must not be empty"

	for _, tc := range []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "spaces only", in: "   "},
		{name: "tab only", in: "\t"},
		{name: "newline only", in: "\n"},
		{name: "colon only", in: ":"},
		{name: "empty command with concurrency", in: ":3"},
		{name: "empty command with concurrency and qps", in: ":3:100"},
		{name: "whitespace command with concurrency", in: "   :3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := parseCmdEntry(tc.in, 10, 0)
			if err == nil {
				t.Fatalf("parseCmdEntry(%q) = %+v, want error %q", tc.in, cmd, wantErr)
			}
			if err.Error() != wantErr {
				t.Fatalf("parseCmdEntry(%q) error = %q, want %q", tc.in, err.Error(), wantErr)
			}
			if cmd != nil {
				t.Fatalf("parseCmdEntry(%q) returned a non-nil CMD alongside the error", tc.in)
			}
		})
	}
}

// TestParseCmdEntry checks that the valid forms documented in the command
// description keep parsing the same way.
func TestParseCmdEntry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       string
		defConc  int
		defQPS   int
		wantCmd  string
		wantConc int
		wantQPS  int
	}{
		{
			name: "command only", in: "lotus-shed mpool miner-select-messages",
			defConc: 10, defQPS: 0,
			wantCmd: "lotus-shed mpool miner-select-messages", wantConc: 10, wantQPS: 0,
		},
		{
			name: "concurrency overridden", in: "lotus-shed mpool miner-select-messages:3",
			defConc: 10, defQPS: 0,
			wantCmd: "lotus-shed mpool miner-select-messages", wantConc: 3, wantQPS: 0,
		},
		{
			name: "qps overridden, concurrency default", in: "lotus-shed mpool miner-select-messages::100",
			defConc: 10, defQPS: 0,
			wantCmd: "lotus-shed mpool miner-select-messages", wantConc: 10, wantQPS: 100,
		},
		{
			name: "concurrency and qps overridden", in: "lotus-shed mpool miner-select-messages:3:100",
			defConc: 10, defQPS: 0,
			wantCmd: "lotus-shed mpool miner-select-messages", wantConc: 3, wantQPS: 100,
		},
		{
			name: "command with arguments", in: "lotus sync wait",
			defConc: 5, defQPS: 7,
			wantCmd: "lotus sync wait", wantConc: 5, wantQPS: 7,
		},
		{
			name: "surrounding whitespace is preserved", in: "  lotus sync wait  ",
			defConc: 5, defQPS: 7,
			wantCmd: "  lotus sync wait  ", wantConc: 5, wantQPS: 7,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := parseCmdEntry(tc.in, tc.defConc, tc.defQPS)
			if err != nil {
				t.Fatalf("parseCmdEntry(%q) returned unexpected error: %v", tc.in, err)
			}
			if cmd.cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd.cmd, tc.wantCmd)
			}
			if cmd.concurrency != tc.wantConc {
				t.Errorf("concurrency = %d, want %d", cmd.concurrency, tc.wantConc)
			}
			if cmd.qps != tc.wantQPS {
				t.Errorf("qps = %d, want %d", cmd.qps, tc.wantQPS)
			}
		})
	}
}

// TestParseCmdEntryRejectsNonNumericOptions checks that the error messages for
// malformed concurrency and qps values are unchanged.
func TestParseCmdEntryRejectsNonNumericOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{name: "non numeric concurrency", in: "lotus sync wait:abc"},
		{name: "non numeric qps", in: "lotus sync wait::abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := parseCmdEntry(tc.in, 10, 0)
			if err == nil {
				t.Fatalf("parseCmdEntry(%q) = %+v, want error", tc.in, cmd)
			}
			if cmd != nil {
				t.Fatalf("parseCmdEntry(%q) returned a non-nil CMD alongside the error", tc.in)
			}
		})
	}
}
