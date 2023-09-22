package paths

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
)

func Move(from, to string) error {
	from, err := homedir.Expand(from)
	if err != nil {
		return xerrors.Errorf("move: expanding from: %w", err)
	}

	to, err = homedir.Expand(to)
	if err != nil {
		return xerrors.Errorf("move: expanding to: %w", err)
	}

	if filepath.Base(from) != filepath.Base(to) {
		return xerrors.Errorf("move: base names must match ('%s' != '%s')", filepath.Base(from), filepath.Base(to))
	}

	log.Debugw("move sector data", "from", from, "to", to)

	toDir := filepath.Dir(to)

	// `mv` has decades of experience in moving files quickly; don't pretend we
	//  can do better

	var errOut bytes.Buffer

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		if err := os.MkdirAll(toDir, 0777); err != nil {
			return xerrors.Errorf("failed exec MkdirAll: %s", err)
		}

		cmd = exec.Command("/usr/bin/env", "mv", from, toDir) // nolint
	} else {
		cmd = exec.Command("/usr/bin/env", "mv", "-t", toDir, from) // nolint
	}

	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("exec mv (stderr: %s): %w", strings.TrimSpace(errOut.String()), err)
	}

	return nil
}
