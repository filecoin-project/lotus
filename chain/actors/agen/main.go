package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"text/template"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	lotusactors "github.com/filecoin-project/lotus/chain/actors"
)

var actors = map[string][]int{
	"account":  lotusactors.Versions,
	"cron":     lotusactors.Versions,
	"init":     lotusactors.Versions,
	"market":   lotusactors.Versions,
	"miner":    lotusactors.Versions,
	"multisig": lotusactors.Versions,
	"paych":    lotusactors.Versions,
	"power":    lotusactors.Versions,
	"system":   lotusactors.Versions,
	"reward":   lotusactors.Versions,
	"verifreg": lotusactors.Versions,
	"datacap":  lotusactors.Versions[8:],
	"evm":      lotusactors.Versions[9:],
}

func main() {
	var lets errgroup.Group
	lets.Go(generateAdapters)
	lets.Go(generatePolicy)
	lets.Go(generateBuiltin)
	lets.Go(generateRegistry)
	if err := lets.Wait(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("All chain actor files generated successfully.")
}

func generateAdapters() error {
	for act, versions := range actors {
		actDir := filepath.Join("chain/actors/builtin", act)

		if err := generateState(actDir, versions); err != nil {
			return err
		}

		if err := generateMessages(actDir); err != nil {
			return err
		}

		{
			af, err := os.ReadFile(filepath.Join(actDir, "actor.go.template"))
			if err != nil {
				return xerrors.Errorf("loading actor template: %w", err)
			}

			tpl := template.Must(template.New("").Funcs(template.FuncMap{
				"import": func(v int) string { return getVersionImports()[v] },
			}).Parse(string(af)))

			var b bytes.Buffer

			err = tpl.Execute(&b, map[string]interface{}{
				"versions":      versions,
				"latestVersion": lotusactors.LatestVersion,
			})
			if err != nil {
				return err
			}

			fmted, err := format.Source(b.Bytes())
			if err != nil {
				return err
			}

			if err := os.WriteFile(filepath.Join(actDir, fmt.Sprintf("%s.go", act)), fmted, 0666); err != nil {
				return err
			}
		}
	}

	return nil
}

func generateState(actDir string, versions []int) error {
	af, err := os.ReadFile(filepath.Join(actDir, "state.go.template"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip
		}

		return xerrors.Errorf("loading state adapter template: %w", err)
	}

	for _, version := range versions {
		tpl := template.Must(template.New("").Funcs(template.FuncMap{}).Parse(string(af)))

		var b bytes.Buffer

		err := tpl.Execute(&b, map[string]interface{}{
			"v":             version,
			"import":        getVersionImports()[version],
			"latestVersion": lotusactors.LatestVersion,
		})
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(actDir, fmt.Sprintf("v%d.go", version)), b.Bytes(), 0666); err != nil {
			return err
		}
	}

	return nil
}

func generateMessages(actDir string) error {
	af, err := os.ReadFile(filepath.Join(actDir, "message.go.template"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip
		}

		return xerrors.Errorf("loading message adapter template: %w", err)
	}

	for _, version := range lotusactors.Versions {
		tpl := template.Must(template.New("").Funcs(template.FuncMap{}).Parse(string(af)))

		var b bytes.Buffer

		err := tpl.Execute(&b, map[string]interface{}{
			"v":             version,
			"import":        getVersionImports()[version],
			"latestVersion": lotusactors.LatestVersion,
		})
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(actDir, fmt.Sprintf("message%d.go", version)), b.Bytes(), 0666); err != nil {
			return err
		}
	}

	return nil
}

func generatePolicy() error {
	const policyPath = "chain/actors/policy/policy.go"
	pf, err := os.ReadFile(policyPath + ".template")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip
		}

		return xerrors.Errorf("loading policy template file: %w", err)
	}

	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"import": func(v int) string { return getVersionImports()[v] },
	}).Parse(string(pf)))
	var b bytes.Buffer

	err = tpl.Execute(&b, map[string]interface{}{
		"versions":      lotusactors.Versions,
		"latestVersion": lotusactors.LatestVersion,
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(policyPath, b.Bytes(), 0666); err != nil {
		return err
	}

	return nil
}

func generateBuiltin() error {
	const builtinPath = "chain/actors/builtin/builtin.go"
	bf, err := os.ReadFile(builtinPath + ".template")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip
		}

		return xerrors.Errorf("loading builtin template file: %w", err)
	}

	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"import": func(v int) string { return getVersionImports()[v] },
	}).Parse(string(bf)))
	var b bytes.Buffer

	err = tpl.Execute(&b, map[string]interface{}{
		"versions":      lotusactors.Versions,
		"latestVersion": lotusactors.LatestVersion,
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(builtinPath, b.Bytes(), 0666); err != nil {
		return err
	}

	return nil
}

func generateRegistry() error {
	const registryPath = "chain/actors/builtin/registry.go"
	bf, err := os.ReadFile(registryPath + ".template")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip
		}

		return xerrors.Errorf("loading registry template file: %w", err)
	}

	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"import": func(v int) string { return getVersionImports()[v] },
	}).Parse(string(bf)))
	var b bytes.Buffer

	err = tpl.Execute(&b, map[string]interface{}{
		"versions": lotusactors.Versions,
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(registryPath, b.Bytes(), 0666); err != nil {
		return err
	}

	return nil
}

func getVersionImports() map[int]string {
	versionImports := make(map[int]string, lotusactors.LatestVersion)
	for _, v := range lotusactors.Versions {
		if v == 0 {
			versionImports[v] = "/"
		} else {
			versionImports[v] = "/v" + strconv.Itoa(v) + "/"
		}
	}

	return versionImports
}
