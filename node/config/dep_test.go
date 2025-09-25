package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func goRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func goCmd() string {
	var exeSuffix string
	if runtime.GOOS == "windows" {
		exeSuffix = ".exe"
	}
	root, _ := goRoot()
	path := filepath.Join(root, "bin", "go"+exeSuffix)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "go"
}

func TestDoesntDependOnFFI(t *testing.T) {
	deps, err := exec.Command(goCmd(), "list", "-deps", "github.com/filecoin-project/lotus/node/config").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range strings.Fields(string(deps)) {
		if pkg == "github.com/filecoin-project/filecoin-ffi" {
			t.Fatal("config depends on filecoin-ffi")
		}
	}
}
