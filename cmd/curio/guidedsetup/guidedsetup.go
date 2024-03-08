// guidedSetup for migration from lotus-miner to Curio
//
//	IF STRINGS CHANGED {
//			follow instructions at ../internal/translations/translations.go
//	}
package guidedsetup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"os"
	"os/signal"
	"path"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/manifoldco/promptui"
	"github.com/mitchellh/go-homedir"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	_ "github.com/filecoin-project/lotus/cmd/curio/internal/translations"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var GuidedsetupCmd = &cli.Command{
	Name:  "guided-setup",
	Usage: "Run the guided setup for migrating from lotus-miner to Curio",
	Action: func(cctx *cli.Context) (err error) {
		T, say := SetupLanguage()
		setupCtrlC(say)

		say(header, "This interactive tool migrates lotus-miner to Curio in 5 minutes.\n")
		say(notice, "Each step needs your confirmation and can be reversed. Press Ctrl+C to exit at any time.")

		// Run the migration steps
		migrationData := MigrationData{
			T:   T,
			say: say,
			selectTemplates: &promptui.SelectTemplates{
				Help: T("Use the arrow keys to navigate: ↓ ↑ → ← "),
			},
		}
		for _, step := range migrationSteps {
			step(&migrationData)
		}
		return nil
	},
}

func setupCtrlC(say func(style lipgloss.Style, key message.Reference, a ...interface{})) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		say(notice, "Ctrl+C pressed in Terminal")
		os.Exit(2)
	}()
}

var (
	header = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#242424")).
		BorderStyle(lipgloss.NormalBorder()).
		Width(60).Margin(1)

	notice = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Bold(true).
		Foreground(lipgloss.Color("#CCCCCC")).
		Background(lipgloss.Color("#333300")).MarginBottom(1)

	green = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#000000"))

	plain = lipgloss.NewStyle().Align(lipgloss.Left)

	section = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFFFFF")).
		Underline(true)

	code = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#f8f9fa"))
)

func SetupLanguage() (func(key message.Reference, a ...interface{}) string, func(style lipgloss.Style, key message.Reference, a ...interface{})) {
	langText := "en"
	problem := false
	if len(os.Getenv("LANG")) > 1 {
		langText = os.Getenv("LANG")[:2]
	} else {
		problem = true
	}

	lang, err := language.Parse(langText)
	if err != nil {
		lang = language.English
		problem = true
		fmt.Println("Error parsing language")
		os.Setenv("LANG", "en-US") // for later users of this function
	}

	langs := message.DefaultCatalog.Languages()
	have := lo.SliceToMap(langs, func(t language.Tag) (string, bool) { return t.String(), true })
	if _, ok := have[lang.String()]; !ok {
		lang = language.English
		problem = true
	}
	if problem {
		notice.Copy().AlignHorizontal(lipgloss.Right).
			Render("$LANG=" + langText + " unsupported. Available: " + strings.Join(lo.Keys(have), ", "))
		fmt.Println("Defaulting to English. Please reach out to the Curio team if you would like to have additional language support.")
	}
	return func(key message.Reference, a ...interface{}) string {
			return message.NewPrinter(lang).Sprintf(key, a...)
		}, func(sty lipgloss.Style, key message.Reference, a ...interface{}) {
			msg := message.NewPrinter(lang).Sprintf(key, a...)
			fmt.Println(sty.Render(msg))
		}
}

type migrationStep func(*MigrationData)

var migrationSteps = []migrationStep{
	readMinerConfig, // Tells them to be on the miner machine
	yugabyteConnect, // Miner is updated
	configToDB,      // work on base configuration migration.
	verifySectors,   // Verify the sectors are in the database
	doc,
	oneLastThing,
	complete,
}

type MigrationData struct {
	T               func(key message.Reference, a ...interface{}) string
	say             func(style lipgloss.Style, key message.Reference, a ...interface{})
	selectTemplates *promptui.SelectTemplates
	MinerConfigPath string
	MinerConfig     *config.StorageMiner
	DB              *harmonydb.DB
	MinerID         address.Address
}

func complete(d *MigrationData) {
	stepCompleted(d, d.T("Lotus-Miner to Curio Migration."))
	d.say(plain, "Try the web interface with  for further guided improvements.\n", "--layers=gui")
	d.say(plain, "You can now migrate your market node (%s), if applicable.\n", "Boost")
}
func configToDB(d *MigrationData) {
	d.say(section, "Migrating config.toml to database.\n")

	type rawConfig struct {
		Raw   []byte `db:"config"`
		Title string
	}
	var configBytes []rawConfig
	err := d.DB.Select(context.Background(), &configBytes, `SELECT config, title FROM harmony_config `)
	if err != nil {
		d.say(notice, "Error reading from database: %s. Aborting Migration.\n", err.Error())
		os.Exit(1)
	}
	// Populate API Key
	_, header, err := cliutil.GetRawAPI(nil, repo.FullNode, "v0")
	if err != nil {
		d.say(plain, "cannot read API: %s. Aborting Migration", err.Error())
		os.Exit(1)
	}

	err = SaveConfigToLayer(d.MinerConfigPath, "", false, header)
	if err != nil {
		d.say(notice, "Error saving config to layer: %s. Aborting Migration", err.Error())
		os.Exit(1)
	}
}

// bucket returns the power's 4 highest bits (rounded down).
func bucket(power *api.MinerPower) uint64 {
	rawQAP := power.TotalPower.QualityAdjPower.Uint64()
	leadingDigit := 64 - bits.LeadingZeros64(rawQAP)
	if leadingDigit < 4 {
		return 0
	}
	return rawQAP >> (uint64(leadingDigit) - 4) << (uint64(leadingDigit - 4))
}

func oneLastThing(d *MigrationData) {
	d.say(section, "Protocol Labs wants to improve the software you use. Tell the team you're using Curio.")
	i, _, err := (&promptui.Select{
		Label: d.T("Select what you want to share with the Curio team."),
		Items: []string{
			d.T("Individual Data: Miner ID, Curio version, net (%s or %s). Signed.", "mainnet", "testnet"),
			d.T("Aggregate-Anonymous: version, net, and Miner power (bucketed)."),
			d.T("Hint: I am someone running Curio on net."),
			d.T("Nothing.")},
		Templates: d.selectTemplates,
	}).Run()
	if err != nil {
		d.say(notice, "Aborting remaining steps.\n", err.Error())
		os.Exit(1)
	}
	if i < 3 {
		msgMap := map[string]any{
			"chain": build.BuildTypeString(),
		}
		if i < 2 {
			api, closer, err := cliutil.GetFullNodeAPI(nil)
			if err != nil {
				d.say(notice, "Error connecting to lotus node: %s\n", err.Error())
				os.Exit(1)
			}
			defer closer()
			power, err := api.StateMinerPower(context.Background(), d.MinerID, types.EmptyTSK)
			if err != nil {
				d.say(notice, "Error getting miner power: %s\n", err.Error())
				os.Exit(1)
			}
			msgMap["version"] = build.BuildVersion
			msgMap["net"] = build.BuildType
			msgMap["power"] = map[int]uint64{1: power.MinerPower.QualityAdjPower.Uint64(), 2: bucket(power)}

			if i < 1 { // Sign it
				msgMap["miner_id"] = d.MinerID
				msg, err := json.Marshal(msgMap)
				if err != nil {
					d.say(notice, "Error marshalling message: %s\n", err.Error())
					os.Exit(1)
				}
				mi, err := api.StateMinerInfo(context.Background(), d.MinerID, types.EmptyTSK)
				if err != nil {
					d.say(notice, "Error getting miner info: %s\n", err.Error())
					os.Exit(1)
				}
				sig, err := api.WalletSign(context.Background(), mi.Worker, msg)
				if err != nil {
					d.say(notice, "Error signing message: %s\n", err.Error())
					os.Exit(1)
				}
				msgMap["signature"] = base64.StdEncoding.EncodeToString(sig.Data)
			}
		}
		msg, err := json.Marshal(msgMap)
		if err != nil {
			d.say(notice, "Error marshalling message: %s\n", err.Error())
			os.Exit(1)
		}

		resp, err := http.DefaultClient.Post("https://curiostorage.org/cgi-bin/savedata.php", "application/json", bytes.NewReader(msg))
		if err != nil {
			d.say(notice, "Error sending message: %s\n", err.Error())
		}
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				b, err := io.ReadAll(resp.Body)
				if err == nil {
					d.say(notice, "Error sending message: Status %s, Message: \n", resp.Status, string(b))
				}
			} else {
				stepCompleted(d, d.T("Message sent."))
			}
		}
	}
}

func doc(d *MigrationData) {
	d.say(plain, "Documentation: \n")
	d.say(plain, "The '%s' layer stores common configuration. All curio instances can include it in their %s argument.\n", "base", "--layers")
	d.say(plain, "You can add other layers for per-machine configuration changes.\n")

	d.say(plain, "Join %s in Filecoin %s for help.\n", "#fil-curio-help", "Slack")
	d.say(plain, "Join %s in Filecoin %s to follow development and feedback!\n", "#fil-curio-dev", "Slack")

	d.say(plain, "Want PoST redundancy? Run many Curio instances with the '%s' layer.\n", "post")
	d.say(plain, "Point your browser to your web GUI to complete setup with %s and advanced featues.\n", "Boost")
	d.say(plain, "For SPs with multiple Miner IDs, run 1 migration per lotus-miner all to the same 1 database. The cluster will serve all Miner IDs.\n")

	fmt.Println()
}

func verifySectors(d *MigrationData) {
	var i []int
	var lastError string
	d.say(section, "Please start %s now that database credentials are in %s.\n", "lotus-miner", "config.toml")
	d.say(notice, "Waiting for %s to write sectors into Yugabyte.\n", "lotus-miner")
	for {
		err := d.DB.Select(context.Background(), &i, "SELECT count(*) FROM sector_location")
		if err != nil {
			if err.Error() != lastError {
				d.say(notice, "Error verifying sectors: %s\n", err.Error())
				lastError = err.Error()
			}
			continue
		}
		if i[0] > 0 {
			break
		}
		fmt.Print(".")
		time.Sleep(5 * time.Second)
	}
	d.say(plain, "The sectors are in the database. The database is ready for %s.\n", "Curio")
	d.say(notice, "Now shut down lotus-miner and move the systems to %s.\n", "Curio")

	_, err := (&promptui.Prompt{Label: d.T("Press return to continue")}).Run()
	if err != nil {
		d.say(notice, "Aborting migration.\n")
		os.Exit(1)
	}
	stepCompleted(d, d.T("Sectors verified. %d sector locations found.\n", i))
}

func yugabyteConnect(d *MigrationData) {
	harmonyCfg := config.DefaultStorageMiner().HarmonyDB //copy the config to a local variable
	if d.MinerConfig != nil {
		harmonyCfg = d.MinerConfig.HarmonyDB //copy the config to a local variable
	}
	var err error
	d.DB, err = harmonydb.NewFromConfig(harmonyCfg)
	if err == nil {
		goto yugabyteConnected
	}
	for {
		i, _, err := (&promptui.Select{
			Label: d.T("Enter the info to connect to your Yugabyte database installation (https://download.yugabyte.com/)"),
			Items: []string{
				d.T("Host: %s", strings.Join(harmonyCfg.Hosts, ",")),
				d.T("Port: %s", harmonyCfg.Port),
				d.T("Username: %s", harmonyCfg.Username),
				d.T("Password: %s", harmonyCfg.Password),
				d.T("Database: %s", harmonyCfg.Database),
				d.T("Continue to connect and update schema.")},
			Size:      6,
			Templates: d.selectTemplates,
		}).Run()
		if err != nil {
			d.say(notice, "Database config error occurred, abandoning migration: %s \n", err.Error())
			os.Exit(1)
		}
		switch i {
		case 0:
			host, err := (&promptui.Prompt{
				Label: d.T("Enter the Yugabyte database host(s)"),
			}).Run()
			if err != nil {
				d.say(notice, "No host provided\n")
				continue
			}
			harmonyCfg.Hosts = strings.Split(host, ",")
		case 1, 2, 3, 4:
			val, err := (&promptui.Prompt{
				Label: d.T("Enter the Yugabyte database %s", []string{"port", "username", "password", "database"}[i-1]),
			}).Run()
			if err != nil {
				d.say(notice, "No value provided\n")
				continue
			}
			switch i {
			case 1:
				harmonyCfg.Port = val
			case 2:
				harmonyCfg.Username = val
			case 3:
				harmonyCfg.Password = val
			case 4:
				harmonyCfg.Database = val
			}
			continue
		case 5:
			d.DB, err = harmonydb.NewFromConfig(harmonyCfg)
			if err != nil {
				if err.Error() == "^C" {
					os.Exit(1)
				}
				d.say(notice, "Error connecting to Yugabyte database: %s\n", err.Error())
				continue
			}
			goto yugabyteConnected
		}
	}

yugabyteConnected:
	d.say(plain, "Connected to Yugabyte. Schema is current.\n")
	if !reflect.DeepEqual(harmonyCfg, d.MinerConfig.HarmonyDB) {
		d.MinerConfig.HarmonyDB = harmonyCfg
		buf, err := config.ConfigUpdate(d.MinerConfig, config.DefaultStorageMiner())
		if err != nil {
			d.say(notice, "Error encoding config.toml: %s\n", err.Error())
			os.Exit(1)
		}
		_, _ = (&promptui.Prompt{Label: d.T("Press return to update %s with Yugabyte info. Backup the file now.", "config.toml")}).Run()
		stat, err := os.Stat(path.Join(d.MinerConfigPath, "config.toml"))
		if err != nil {
			d.say(notice, "Error reading filemode of config.toml: %s\n", err.Error())
			os.Exit(1)
		}
		filemode := stat.Mode()
		err = os.WriteFile(path.Join(d.MinerConfigPath, "config.toml"), buf, filemode)
		if err != nil {
			d.say(notice, "Error writing config.toml: %s\n", err.Error())
			os.Exit(1)
		}
		d.say(section, "Restart Lotus Miner. \n")
	}
	stepCompleted(d, d.T("Connected to Yugabyte"))
}

func readMinerConfig(d *MigrationData) {
	d.say(plain, "To start, ensure your sealing pipeline is drained and shut-down lotus-miner.\n")

	if os.Getenv("FULLNODE_API_INFO") == "" {
		d.say(notice, "FULLNODE_API_INFO is not set. Aborting migration.\n")
		d.say(plain, "Set the environment variable and run again. Expected format: %s\n",
			"<"+d.T("api_token")+">:/ip4/<"+d.T("lotus_daemon_ip")+">/tcp/<"+d.T("lotus_daemon_port")+">/http")
		os.Exit(1)

	}
	verifyPath := func(dir string) (*config.StorageMiner, error) {
		cfg := config.DefaultStorageMiner()
		dir, err := homedir.Expand(dir)
		if err != nil {
			return nil, err
		}
		_, err = toml.DecodeFile(path.Join(dir, "config.toml"), &cfg)
		return cfg, err
	}

	dirs := map[string]*config.StorageMiner{"~/.lotusminer": nil, "~/.lotus-miner-local-net": nil}
	if v := os.Getenv("LOTUS_MINER_PATH"); v != "" {
		dirs[v] = nil
	}
	for dir := range dirs {
		cfg, err := verifyPath(dir)
		if err != nil {
			delete(dirs, dir)
		}
		dirs[dir] = cfg
	}

	var otherPath bool
	if len(dirs) > 0 {
		_, str, err := (&promptui.Select{
			Label:     d.T("Select the location of your lotus-miner config directory?"),
			Items:     append(lo.Keys(dirs), d.T("Other")),
			Templates: d.selectTemplates,
		}).Run()
		if err != nil {
			if err.Error() == "^C" {
				os.Exit(1)
			}
			otherPath = true
		} else {
			if str == d.T("Other") {
				otherPath = true
			} else {
				d.MinerConfigPath = str
				d.MinerConfig = dirs[str]
			}
		}
	}
	if otherPath {
	minerPathEntry:
		str, err := (&promptui.Prompt{
			Label: d.T("Enter the path to the configuration directory used by %s", "lotus-miner"),
		}).Run()
		if err != nil {
			d.say(notice, "No path provided, abandoning migration \n")
			os.Exit(1)
		}
		cfg, err := verifyPath(str)
		if err != nil {
			d.say(notice, "Cannot read the config.toml file in the provided directory, Error: %s\n", err.Error())
			goto minerPathEntry
		}
		d.MinerConfigPath = str
		d.MinerConfig = cfg
	}

	stepCompleted(d, d.T("Read Miner Config"))
}
func stepCompleted(d *MigrationData, step string) {
	fmt.Print(green.Render("✔ "))
	d.say(plain, "Step Complete: %s\n\n", step)
}
