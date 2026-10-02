package main

import "testing"

func TestParseCmdEntry(t *testing.T) {
	for _, tc := range []struct {
		in       string
		wantCmd  string
		wantConc int
		wantQPS  int
		wantErr  bool
	}{
		{in: "lotus sync wait", wantCmd: "lotus sync wait", wantConc: 10, wantQPS: 0},
		{in: "lotus sync wait:3", wantCmd: "lotus sync wait", wantConc: 3, wantQPS: 0},
		{in: "lotus sync wait::100", wantCmd: "lotus sync wait", wantConc: 10, wantQPS: 100},
		{in: "lotus sync wait:3:100", wantCmd: "lotus sync wait", wantConc: 3, wantQPS: 100},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: ":3", wantErr: true},
		{in: "lotus sync wait:abc", wantErr: true},
		{in: "lotus sync wait:0", wantErr: true},
		{in: "lotus sync wait:-1", wantErr: true},
		{in: "lotus sync wait::abc", wantErr: true},
		{in: "lotus sync wait::-1", wantErr: true},
		{in: "lotus sync wait::1000000001", wantErr: true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			cmd, err := parseCmdEntry(tc.in, 10, 0)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", cmd)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cmd.cmd != tc.wantCmd || cmd.concurrency != tc.wantConc || cmd.qps != tc.wantQPS {
				t.Fatalf("got %q:%d:%d, want %q:%d:%d", cmd.cmd, cmd.concurrency, cmd.qps, tc.wantCmd, tc.wantConc, tc.wantQPS)
			}
		})
	}
}
