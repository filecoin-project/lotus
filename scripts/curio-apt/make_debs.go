package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/codeskyblue/go-sh"
	"github.com/samber/lo"
)

var output string
var version string

func main() {
	if len(os.Args) < 2 || strings.EqualFold(os.Args[1], "help") {
		fmt.Println("Usage: make_debs <version>")
		fmt.Println("Be sure /tmp/apt-pri-key.pem exists.")
		fmt.Println("And run this from the root of the lotus repo.")
		os.Exit(1)
	}

	version = os.Args[1]

	base, err := os.MkdirTemp(os.TempDir(), "curio-apt")
	OrPanic(err)

	part2(base, "curio-cuda", "")
	part2(base, "curio-opencl", "FFI_USE_OPENCL=1")
	fmt.Println("Done.")
	fmt.Println(output)
}

func part2(base, product, extra string) {
	// copy scripts/curio-apt/debian  to dir/debian
	dir := path.Join(base, product)

	err := os.MkdirAll(path.Join(dir, "debian"), 0755)
	OrPanic(err)

	OrPanic(sh.Command("cp", "-r", "scripts/curio-apt/debian", dir).Run())
	sess := sh.NewSession()
	lo.ForEach(strings.Split(extra, " "), func(s string, _ int) {
		v := strings.Split(s, "=")
		sess.SetEnv(v[0], v[1])
	})
	OrPanic(sess.Command("make", "clean", "all").Run())
	OrPanic(copyFile("curio", path.Join(dir, "curio")))
	OrPanic(copyFile("sptool", path.Join(dir, "sptool")))

	// fix the debian/control "package" and "version" fields
	f, err := os.ReadFile(path.Join(dir, "debian", "control"))
	OrPanic(err)
	f = []byte(strings.ReplaceAll(string(f), "$PACKAGE", product))
	f = []byte(strings.ReplaceAll(string(f), "$VERSION", version))
	os.WriteFile(path.Join(dir, "debian", "control"), f, 0644)

	sh.Command("dpkg-buildpackage", "-us", "-uc").SetDir(dir).Run()
}

func copyFile(src, dest string) error {
	return sh.Command("cp", src, dest).Run()
}

func OrPanic(err error) {
	if err != nil {
		panic(err)
	}
}
