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

func createPackagePath(pathParts ...string) string {
	return strings.Join(append([]string{"."}, pathParts...), string(os.PathSeparator))
}

func getPackages(testGroupName string) []string {
	testGroupNamesToPackages := map[string][]string{
		"multicore-sdr": {createPackagePath("storage", "sealer", "ffiwrapper")},
		"conformance":   {createPackagePath("conformance")},
		"unit-cli": {
			createPackagePath("cli", "..."),
			createPackagePath("cmd", "..."),
			createPackagePath("api", "..."),
		},
		"unit-storage": {
			createPackagePath("storage", "..."),
			createPackagePath("extern", "..."),
		},
		"unit-node": {
			createPackagePath("node", "..."),
		},
		"unit-rest": {
			createPackagePath("blockstore", "..."),
			createPackagePath("build", "..."),
			createPackagePath("chain", "..."),
			createPackagePath("conformance", "..."),
			createPackagePath("gateway", "..."),
			createPackagePath("journal", "..."),
			createPackagePath("lib", "..."),
			createPackagePath("paychmgr", "..."),
			createPackagePath("tools", "..."),
		},
	}

	if strings.HasPrefix(testGroupName, "itest-") {
		return []string{createPackagePath("itests", strings.Join([]string{strings.TrimPrefix(testGroupName, "itest-"), "test.go"}, "_"))}
	}

	return testGroupNamesToPackages[testGroupName]
}

func getNeedsParameters(testGroupName string) bool {
	testGroupNames := []string{
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
	return contains(testGroupNames, testGroupName)
}

func getSkipConformance(testGroupName string) bool {
	testGroupNames := []string{
		"conformance",
	}
	return !contains(testGroupNames, testGroupName)
}

func getTestRustProofLogs(testGroupName string) bool {
	testGroupNames := []string{
		"multicore-sdr",
	}
	return contains(testGroupNames, testGroupName)
}

func getFormat() string {
	return "standard-verbose"
}

func getGoTestFlags(testGroupName string) string {
	testGroupNamesToFlags := map[string]string{
		"multicore-sdr": "-run=TestMulticoreSDR",
		"conformance":   "-run=TestConformance",
	}
	if flag, ok := testGroupNamesToFlags[testGroupName]; ok {
		return flag
	}
	return ""
}

func getRunners(testGroupName string) [][]string {
	if os.Getenv("GITHUB_REPOSITORY_OWNER") != "filecoin-project" {
		return [][]string{{"ubuntu-latest"}}
	}

	testGroupNamesToRunners := map[string][][]string{
		"itest-niporep_manual":    {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-sector_pledge":     {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-worker":            {{"self-hosted", "linux", "x64", "4xlarge"}},
		"itest-manual_onboarding": {{"self-hosted", "linux", "x64", "4xlarge"}},

		"itest-gateway":              {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-sector_import_full":   {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-sector_import_simple": {{"self-hosted", "linux", "x64", "2xlarge"}},
		"itest-wdpost":               {{"self-hosted", "linux", "x64", "2xlarge"}},
		"unit-storage": {
			{"self-hosted", "linux", "x64", "2xlarge"},
			{"self-hosted", "linux", "arm64", "2xlarge"},
		},

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
		"unit-node": {
			{"self-hosted", "linux", "x64", "xlarge"},
			{"self-hosted", "linux", "arm64", "xlarge"},
		},

		"unit-cli": {
			{"ubuntu-latest"},
			{"ubuntu-24.04-arm"},
		},
		"unit-rest": {
			{"ubuntu-latest"},
			{"ubuntu-24.04-arm"},
		},
	}

	if runners, ok := testGroupNamesToRunners[testGroupName]; ok {
		return runners
	}

	return [][]string{{"ubuntu-latest"}}
}

func getTestGroups(testGroupName string) []TestGroup {
	runners := getRunners(testGroupName)

	groups := []TestGroup{}

	for _, runner := range runners {
		groups = append(groups, TestGroup{
			Name:   testGroupName,
			Runner: runner,
		})
	}

	return groups
}

func getTestGroupMetadata(testGroupName string) TestGroupMetadata {
	packages := getPackages(testGroupName)
	needsParameters := getNeedsParameters(testGroupName)
	skipConformance := getSkipConformance(testGroupName)
	testRustProofLogs := getTestRustProofLogs(testGroupName)
	format := getFormat()
	goTestFlags := getGoTestFlags(testGroupName)

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
			testGroupName := strings.Join([]string{"itest", strings.TrimSuffix(parts[1], "_test.go")}, "-")
			groups = append(groups, getTestGroups(testGroupName)...)
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
					testGroupName := c.String("name")
					metadata := getTestGroupMetadata(testGroupName)
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
