package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/urfave/cli/v2"
)

func takeProfiles(ctx context.Context) (fname string, _err error) {
	dir, err := os.MkdirTemp(".", ".profiles-temp*")
	if err != nil {
		return "", err
	}

	if err := writeProfiles(ctx, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}

	fname = fmt.Sprintf("pprof-simulation-%s", time.Now().Format(time.RFC3339))
	if err := os.Rename(dir, fname); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return fname, nil
}

func writeProfiles(ctx context.Context, dir string) error {
	for _, profile := range pprof.Profiles() {
		file, err := os.Create(filepath.Join(dir, profile.Name()+".pprof.gz"))
		if err != nil {
			return err
		}
		if err := profile.WriteTo(file, 0); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	file, err := os.Create(filepath.Join(dir, "cpu.pprof.gz"))
	if err != nil {
		return err
	}

	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		return err
	}
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
	}
	pprof.StopCPUProfile()
	err = file.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	return err
}

func profileOnSignal(cctx *cli.Context, signals ...os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	defer signal.Stop(ch)

	for {
		select {
		case <-ch:
			fname, err := takeProfiles(cctx.Context)
			switch err {
			case context.Canceled:
				return
			case nil:
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Wrote profile to %q\n", fname)
			default:
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "ERROR: failed to write profile: %s\n", err)
			}
		case <-cctx.Done():
			return
		}
	}
}
