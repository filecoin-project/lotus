package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type TestGroupExecutionContext struct {
	Name   string `json:"name"`
	Runner Runner `json:"runner"`
}

// Runner is a list because it is a list of labels associated with a single runner.
type Runner []string

var (
	linux_x64_5xlarge   = []string{"self-hosted", "linux", "x64", "5xlarge"}
	linux_x64_4xlarge   = []string{"self-hosted", "linux", "x64", "4xlarge"}
	linux_x64_2xlarge   = []string{"self-hosted", "linux", "x64", "2xlarge"}
	linux_x64_xlarge    = []string{"self-hosted", "linux", "x64", "xlarge"}
	linux_arm64_2xlarge = []string{"self-hosted", "linux", "arm64", "2xlarge"}
	linux_arm64_xlarge  = []string{"self-hosted", "linux", "arm64", "xlarge"}
	linux_x64           = []string{"ubuntu-latest"}
	linux_arm64         = []string{"ubuntu-24.04-arm"}
)

type TestGroupMetadata struct {
	Packages          []string `json:"packages"`
	NeedsParameters   bool     `json:"needs_parameters"`
	SkipConformance   bool     `json:"skip_conformance"`
	TestRustProofLogs bool     `json:"test_rust_proof_logs"`
	Format            string   `json:"format"`
	GoTestFlags       string   `json:"go_test_flags"`
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
				Name:  "list-test-group-execution-contexts",
				Usage: "List all test group execution contexts",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "very-expensive-tests-run",
						Usage: "Whether to only include the groups with very expensive tests",
					},
				},
				Action: func(c *cli.Context) error {
					integrationTestGroups, err := getIntegrationTestGroups()
					if err != nil {
						return err
					}
					unitTestGroups := getUnitTestGroups()
					otherTestGroups := getOtherTestGroups()
					groups := append(append(integrationTestGroups, unitTestGroups...), otherTestGroups...)
					if c.Bool("very-expensive-tests-run") {
						var filteredGroups []TestGroupExecutionContext
						for _, group := range groups {
							if getHasVeryExpensiveTests(group.Name) {
								filteredGroups = append(filteredGroups, group)
							}
						}
						groups = filteredGroups
					}
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

func getIntegrationTestGroups() ([]TestGroupExecutionContext, error) {
	groups := []TestGroupExecutionContext{}

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

func getUnitTestGroups() []TestGroupExecutionContext {
	groups := []TestGroupExecutionContext{}

	for _, value := range []string{"cli", "node", "rest", "storage"} {
		groups = append(groups, getTestGroups("unit-"+value)...)
	}

	return groups
}

func getOtherTestGroups() []TestGroupExecutionContext {
	groups := []TestGroupExecutionContext{}

	for _, value := range []string{"conformance", "multicore-sdr"} {
		groups = append(groups, getTestGroups(value)...)
	}

	return groups
}

func getTestGroups(testGroupName string) []TestGroupExecutionContext {
	runners := getRunners(testGroupName)

	groups := []TestGroupExecutionContext{}

	for _, runner := range runners {
		groups = append(groups, TestGroupExecutionContext{
			Name:   testGroupName,
			Runner: runner,
		})
	}

	return groups
}

func getRunners(testGroupName string) []Runner {
	if os.Getenv("GITHUB_REPOSITORY_OWNER") != "filecoin-project" {
		return []Runner{linux_x64}
	}

	testGroupNameToRunners := map[string][]Runner{
		"itest-cli":                      {linux_x64_xlarge},
		"itest-deals_invalid_utf8_label": {linux_x64_xlarge},
		"itest-decode_params":            {linux_x64_xlarge},
		"itest-dup_mpool_messages":       {linux_x64_xlarge},
		"itest-eth_account_abstraction":  {linux_x64_xlarge},
		"itest-eth_api":                  {linux_x64_xlarge},
		"itest-eth_balance":              {linux_x64_xlarge},
		"itest-eth_bytecode":             {linux_x64_xlarge},
		"itest-eth_config":               {linux_x64_xlarge},
		"itest-eth_conformance":          {linux_x64_xlarge},
		"itest-eth_deploy":               {linux_x64_xlarge},
		"itest-eth_fee_history":          {linux_x64_xlarge},
		"itest-eth_transactions":         {linux_x64_xlarge},
		"itest-fevm_address":             {linux_x64_xlarge},
		"itest-fevm_events":              {linux_x64_xlarge},
		"itest-gas_estimation":           {linux_x64_xlarge},
		"itest-gateway":                  {linux_x64_2xlarge},
		"itest-get_messages_in_ts":       {linux_x64_xlarge},
		"itest-lite_migration":           {linux_x64_xlarge},
		"itest-lookup_robust_address":    {linux_x64_xlarge},
		"itest-manual_onboarding":        {linux_x64_4xlarge},
		"itest-mempool":                  {linux_x64_xlarge},
		"itest-mpool_msg_uuid":           {linux_x64_xlarge},
		"itest-mpool_push_with_uuid":     {linux_x64_xlarge},
		"itest-msgindex":                 {linux_x64_xlarge},
		"itest-multisig":                 {linux_x64_xlarge},
		"itest-net":                      {linux_x64_xlarge},
		"itest-niporep_manual":           {linux_x64_5xlarge},
		"itest-nonce":                    {linux_x64_xlarge},
		"itest-path_detach_redeclare":    {linux_x64_xlarge},
		"itest-pending_deal_allocation":  {linux_x64_xlarge},
		"itest-remove_verifreg_datacap":  {linux_x64_xlarge},
		"itest-sector_import_full":       {linux_x64_2xlarge},
		"itest-sector_import_simple":     {linux_x64_2xlarge},
		"itest-sector_miner_collateral":  {linux_x64_xlarge},
		"itest-sector_numassign":         {linux_x64_xlarge},
		"itest-sector_pledge":            {linux_x64_4xlarge},
		"itest-self_sent_txn":            {linux_x64_xlarge},
		"itest-verifreg":                 {linux_x64_xlarge},
		"itest-wdpost":                   {linux_x64_2xlarge},
		"itest-worker":                   {linux_x64_4xlarge},
		"multicore-sdr":                  {linux_x64_xlarge},
		"unit-cli":                       {linux_x64, linux_arm64},
		"unit-node":                      {linux_x64_xlarge, linux_arm64_xlarge},
		"unit-rest":                      {linux_x64, linux_arm64},
		"unit-storage":                   {linux_x64_2xlarge, linux_arm64_2xlarge},
	}

	if runners, ok := testGroupNameToRunners[testGroupName]; ok {
		return runners
	}

	return []Runner{linux_x64}
}

func getHasVeryExpensiveTests(testGroupName string) bool {
	testGroupNames := []string{
		"itest-niporep_manual",
	}
	return slices.Contains(testGroupNames, testGroupName)
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

func getPackages(testGroupName string) []string {
	if strings.HasPrefix(testGroupName, "itest-") {
		return []string{createPackagePath("itests", strings.Join([]string{strings.TrimPrefix(testGroupName, "itest-"), "test.go"}, "_"))}
	}

	testGroupNameToPackages := map[string][]string{
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

	return testGroupNameToPackages[testGroupName]
}

func getNeedsParameters(testGroupName string) bool {
	testGroupNames := []string{
		"conformance",
		"itest-api_v2",
		"itest-api",
		"itest-direct_data_onboard_verified",
		"itest-direct_data_onboard",
		"itest-eth_api_f3",
		"itest-manual_onboarding",
		"itest-net",
		"itest-niporep_manual",
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
	return slices.Contains(testGroupNames, testGroupName)
}

func getSkipConformance(testGroupName string) bool {
	testGroupNames := []string{
		"conformance",
	}
	return !slices.Contains(testGroupNames, testGroupName)
}

func getTestRustProofLogs(testGroupName string) bool {
	testGroupNames := []string{
		"multicore-sdr",
	}
	return slices.Contains(testGroupNames, testGroupName)
}

func getFormat() string {
	return "standard-verbose"
}

func getGoTestFlags(testGroupName string) string {
	testGroupNameToFlags := map[string]string{
		"multicore-sdr": "-run=TestMulticoreSDR",
		"conformance":   "-run=TestConformance",
	}
	if flag, ok := testGroupNameToFlags[testGroupName]; ok {
		return flag
	}
	return ""
}

func createPackagePath(pathParts ...string) string {
	return strings.Join(append([]string{"."}, pathParts...), string(os.PathSeparator))
}
