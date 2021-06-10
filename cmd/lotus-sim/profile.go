package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"github.com/urfave/cli/v2"
)

func takeProfiles(ctx context.Context) (fname string, _err error) {
	file, err := os.CreateTemp(".", ".profiles*.tar")
	if err != nil {
		return "", err
	}
	defer file.Close()

	if err := writeProfiles(ctx, file); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}

	fname = fmt.Sprintf("pprof-simulation-%s.tar", time.Now())
	if err := os.Rename(file.Name(), fname); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return fname, nil
}

func writeProfiles(ctx context.Context, w io.Writer) error {
	tw := tar.NewWriter(w)
	for _, profile := range pprof.Profiles() {
		if err := tw.WriteHeader(&tar.Header{Name: profile.Name()}); err != nil {
			return err
		}
		if err := profile.WriteTo(tw, 0); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	if err := tw.WriteHeader(&tar.Header{Name: "cpu"}); err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(tw); err != nil {
		return err
	}
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		pprof.StopCPUProfile()
		return ctx.Err()
	}
	pprof.StopCPUProfile()
	return tw.Close()
}

func profileOnSignal(cctx *cli.Context, signals ...os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	defer signal.Stop(ch)

	for range ch {
		select {
		case <-ch:
			fname, err := takeProfiles(cctx.Context)
			switch err {
			case context.Canceled:
				return
			case nil:
				fmt.Fprintf(cctx.App.ErrWriter, "Wrote profile to %q\n", fname)
			default:
				fmt.Fprintf(cctx.App.ErrWriter, "ERROR: failed to write profile: %s\n", err)
			}
		case <-cctx.Done():
			return
		}
	}
}
