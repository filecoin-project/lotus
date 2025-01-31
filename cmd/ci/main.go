package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type TestGroup struct {
	Name   string   `json:"name"`
	Runner []string `json:"runner"`
}

type TestGroupMetadata struct {
	Packages          []string `json:"packages"`
	NeedsParameters   bool     `json:"needs_parameters"`
	SkipConformance   bool     `json:"skip_conformance"`
	TestRustProofLogs bool     `json:"test_rust_proof_logs"`
	Format            string   `json:"format"`
	GoTestFlags       string   `json:"go_test_flags"`
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getPackages(name string) []string {
	namesToPackages := map[string][]string{
		"multicore-sdr": {strings.Join([]string{"storage", "sealer", "ffiwrapper"}, string(os.PathSeparator))},
		"conformance":   {strings.Join([]string{"conformance"}, string(os.PathSeparator))},
		"unit-cli": {
			strings.Join([]string{"cli", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"cmd", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"api", "..."}, string(os.PathSeparator)),
		},
		"unit-storage": {
			strings.Join([]string{"storage", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"extern", "..."}, string(os.PathSeparator)),
		},
		"unit-node": {
			strings.Join([]string{"node", "..."}, string(os.PathSeparator)),
		},
		"unit-rest": {
			strings.Join([]string{"blockstore", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"build", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"chain", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"conformance", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"gateway", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"journal", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"lib", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"paychmgr", "..."}, string(os.PathSeparator)),
			strings.Join([]string{"tools", "..."}, string(os.PathSeparator)),
		},
	}

	if strings.HasPrefix(name, "itest-") {
		return []string{strings.Join([]string{"itests", strings.Join([]string{strings.TrimPrefix(name, "itest-"), "test.go"}, "_")}, string(os.PathSeparator))}
	}

	return namesToPackages[name]
}

func getNeedsParameters(name string) bool {
	names := []string{
		"conformance",
		"itest-api",
		"itest-direct_data_onboard_verified",
		"itest-direct_data_onboard",
		"itest-manual_onboarding",
		"itest-niporep_manual",
		"itest-net",
		"itest-path_detach_redeclare",
		"itest-sealing_resources",
		"itest-sector_import_full",
		"itest-sector_import_simple",
		"itest-sector_pledge",
		"itest-sector_unseal",
		"itest-wdpost_no_miner_storage",
		"itest-wdpost_worker_config",
		"itest-wdpost",
		"itest-worker",
		"multicore-sdr",
		"unit-cli",
		"unit-storage",
	}
	return contains(names, name)
}

func getSkipConformance(name string) bool {
	names := []string{
		"conformance",
	}
	return !contains(names, name)
}

func getTestRustProofLogs(name string) bool {
	names := []string{
		"multicore-sdr",
	}
	return contains(names, name)
}

func getFormat() string {
	return "standard-verbose"
}

func getGoTestFlags(name string) string {
	namesToFlags := map[string]string{
		"multicore-sdr": "-run=TestMulticoreSDR",
		"conformance":   "-run=TestConformance",
	}
	if flag, ok := namesToFlags[name]; ok {
		return flag
	}
	return ""
}

func getRunners(name string) [][]string {
	if os.Getenv("GITHUB_REPOSITORY_OWNER") != "filecoin-project" {
		return [][]string{{"ubuntu-latest"}}
	}

	namesToRunners := map[string][][]string{
		"itest-niporep_manual":    {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-sector_pledge":     {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-worker":            {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-manual_onboarding": {{"self-hosted", "linux", "x64", "4xlarge"}},

		"itest-gateway":              {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-sector_import_full":   {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-sector_import_simple": {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-wdpost":               {{"self-hosted", "linux", "x64", "2xlarge"}},
		"unit-storage":               {{"self-hosted", "linux", "x64", "2xlarge"}},

		"itest-cli":                      {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-deals_invalid_utf8_label": {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-decode_params":            {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-dup_mpool_messages":       {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_account_abstraction":  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_api":                  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_balance":              {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_bytecode":             {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_config":               {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_conformance":          {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_deploy":               {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_fee_history":          {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-eth_transactions":         {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-fevm_address":             {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-fevm_events":              {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-gas_estimation":           {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-get_messages_in_ts":       {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-lite_migration":           {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-lookup_robust_address":    {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-mempool":                  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-mpool_msg_uuid":           {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-mpool_push_with_uuid":     {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-msgindex":                 {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-multisig":                 {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-net":                      {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-nonce":                    {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-path_detach_redeclare":    {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-pending_deal_allocation":  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-remove_verifreg_datacap":  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-sector_miner_collateral":  {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-sector_numassign":         {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-self_sent_txn":            {{"self-hosted", "linux", "x64", "xlarge"}},
		"itest-verifreg":                 {{"self-hosted", "linux", "x64", "xlarge"}},
		"multicore-sdr":                  {{"self-hosted", "linux", "x64", "xlarge"}},
		"unit-node":                      {{"self-hosted", "linux", "x64", "xlarge"}},
	}

	if runners, ok := namesToRunners[name]; ok {
		return runners
	}

	return [][]string{{"ubuntu-latest"}}
}

func getTestGroups(name string) []TestGroup {
	runners := getRunners(name)

	groups := []TestGroup{}

	for _, runner := range runners {
		groups = append(groups, TestGroup{
			Name:   name,
			Runner: runner,
		})
	}

	return groups
}

func getTestGroupMetadata(name string) TestGroupMetadata {
	packages := getPackages(name)
	needsParameters := getNeedsParameters(name)
	skipConformance := getSkipConformance(name)
	testRustProofLogs := getTestRustProofLogs(name)
	format := getFormat()
	goTestFlags := getGoTestFlags(name)

	return TestGroupMetadata{
		Packages:          packages,
		NeedsParameters:   needsParameters,
		SkipConformance:   skipConformance,
		TestRustProofLogs: testRustProofLogs,
		Format:            format,
		GoTestFlags:       goTestFlags,
	}
}

func findIntegrationTestGroups() ([]TestGroup, error) {
	groups := []TestGroup{}

	err := filepath.Walk("itests", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
			parts := strings.Split(path, string(os.PathSeparator))
			name := strings.Join([]string{"itest", strings.TrimSuffix(parts[1], "_test.go")}, "-")
			groups = append(groups, getTestGroups(name)...)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return groups, nil
}

func getUnitTestGroups() []TestGroup {
	groups := []TestGroup{}

	groups = append(groups, getTestGroups("unit-cli")...)
	groups = append(groups, getTestGroups("unit-storage")...)
	groups = append(groups, getTestGroups("unit-node")...)
	groups = append(groups, getTestGroups("unit-rest")...)

	return groups
}

func getOtherTestGroups() []TestGroup {
	groups := []TestGroup{}

	groups = append(
		groups,
		getTestGroups("multicore-sdr")...,
	)
	groups = append(
		groups,
		getTestGroups("conformance")...,
	)

	return groups
}

func main() {
	app := &cli.App{
		Name:  "ci",
		Usage: "Lotus CI tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Format output as JSON",
			},
		},
		Before: func(c *cli.Context) error {
			if c.Bool("json") {
				log.SetFormatter(&log.JSONFormatter{})
			} else {
				log.SetFormatter(&log.TextFormatter{
					TimestampFormat: "2006-01-02 15:04:05",
					FullTimestamp:   true,
				})
			}
			log.SetOutput(os.Stdout)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "list-test-groups",
				Usage: "List all test groups",
				Action: func(c *cli.Context) error {
					integrationTestGroups, err := findIntegrationTestGroups()
					if err != nil {
						return err
					}
					unitTestGroups := getUnitTestGroups()
					otherTestGroups := getOtherTestGroups()
					groups := append(append(integrationTestGroups, unitTestGroups...), otherTestGroups...)
					b, err := json.MarshalIndent(groups, "", "  ")
					if err != nil {
						log.Fatal(err)
					}
					log.Info(string(b))
					return nil
				},
			},
			{
				Name:  "get-test-group-metadata",
				Usage: "Get the metadata for a test group",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Usage:    "Name of the test group",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					name := c.String("name")
					metadata := getTestGroupMetadata(name)
					b, err := json.MarshalIndent(metadata, "", "  ")
					if err != nil {
						log.Fatal(err)
					}
					log.Info(string(b))
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
