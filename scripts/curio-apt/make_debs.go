// Run from lotus root.
// requires packages: dpkg-dev, dpkg-deb
// Usage:
// ~/GitHub/lotus$ go run scripts/curio-apt/make_debs.go 0.9.7 ~/apt-private.asc
package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/codeskyblue/go-sh"
)

var output string
var version string

func main() {
	if len(os.Args) < 3 || strings.EqualFold(os.Args[1], "help") {
		fmt.Println("Usage: make_debs <version> path_to_private_key.asc")
		fmt.Println("Run this from the root of the lotus repo as it runs 'make'.")
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
	err := os.MkdirAll(path.Join(dir, "DEBIAN"), 0755)
	OrPanic(err)

	OrPanic(sh.Command("cp", "-r", "scripts/curio-apt/DEBIAN", dir).Run())
	sess := sh.NewSession()
	for _, env := range strings.Split(extra, " ") {
		if len(env) == 0 {
			continue
		}
		v := strings.Split(env, "=")
		sess.SetEnv(v[0], v[1])
	}
	fmt.Println("making")
	if os.Getenv("CURIO_DEB_NOBUILD") != "1" {
		OrPanic(sess.Command("make", "clean", "all").Run())
	}
	fmt.Println("copying")
	{
		base := path.Join(dir, "usr", "local", "bin")
		OrPanic(os.MkdirAll(base, 0755))
		OrPanic(copyFile("curio", path.Join(base, "curio")))
		OrPanic(copyFile("sptool", path.Join(base, "sptool")))
	}
	// fix the debian/control "package" and "version" fields
	f, err := os.ReadFile(path.Join(dir, "DEBIAN", "control"))
	OrPanic(err)
	f = []byte(strings.ReplaceAll(string(f), "$PACKAGE", product))
	f = []byte(strings.ReplaceAll(string(f), "$VERSION", version))
	os.WriteFile(path.Join(dir, "DEBIAN", "control"), f, 0644)

	// Option 1: piece by piece
	// Build a .changes file
	//OrPanic(sh.Command("dpkg-genchanges", "-b", "-u.").SetDir(dir).Run())
	// Sign the .changes file
	//OrPanic(sh.Command("debsign", "--sign=origin", "--default", path.Join(dir, "..", "*.changes")).Run())
	// Build the .deb file
	//OrPanic(sh.Command("dpkg-deb", "--build", ".").SetDir(dir).Run())

	// Option 2: The following command should sign the deb file.
	// FAIL B/C wants to build
	//sh.Command("dpkg-buildpackage", "--build=binary").SetDir(dir).Run()

	fmt.Println(base)
	sh.Command("ls", "-al", base).Run()
	// Option 3: ChatGPT Said so
	OrPanic(sh.NewSession().SetDir(base).Command("dpkg-deb", "--build", product).Run())
	OrPanic(sh.NewSession().SetDir(base).Command(
		"dpkg-sig", "--sign builder", "--signbuilder", os.Args[2],
		product+".deb").Run())
}

func copyFile(src, dest string) error {
	return sh.Command("cp", src, dest).Run()
}

func OrPanic(err error) {
	if err != nil {
		panic(err)
	}
}
