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
		Background(lipgloss.Color("#FFFF00")).MarginBottom(1)

	green = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#00FF00"))

	plain = lipgloss.NewStyle().Align(lipgloss.Left)

	section = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("black")).
		Background(lipgloss.Color("#FFFFFF")).
		Underline(true)

	code = lipgloss.NewStyle().
		Align(lipgloss.Left).
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#f8f9fa"))
)

type migrationStep func(*MigrationData)

func SetupLanguage() (func(key message.Reference, a ...interface{}) string, func(style lipgloss.Style, key message.Reference, a ...interface{})) {
	lang, err := language.Parse(os.Getenv("LANG")[:2])
	if err != nil {
		lang = language.English
		fmt.Println("Error parsing language, defaulting to English. Please reach out to the Curio team if you would like to have additional language support.")
	}

	langs := message.DefaultCatalog.Languages()
	have := lo.SliceToMap(langs, func(t language.Tag) (string, bool) { return t.String(), true })
	if _, ok := have[lang.String()]; !ok {
		lang = language.English
		notice.Copy().AlignHorizontal(lipgloss.Right).
			Render("$LANG unsupported. Avaiable: " + strings.Join(lo.Keys(have), ", "))
	}
	return func(key message.Reference, a ...interface{}) string {
			return message.NewPrinter(lang).Sprintf(key, a...)
		}, func(sty lipgloss.Style, key message.Reference, a ...interface{}) {
			msg := message.NewPrinter(lang).Sprintf(key, a...)
			fmt.Println(sty.Render(msg))
		}
}

var migrationSteps = []migrationStep{
	readMinerConfig, // Tells them to be on the miner machine
	yugabyteConnect, // Miner is updated
	verifySectors,   // Verify the sectors are in the database
	configToDB,      // work on base configuration migration.
	doc,
	oneLastThing,
}

type MigrationData struct {
	T               func(key message.Reference, a ...interface{}) string
	say             func(style lipgloss.Style, key message.Reference, a ...interface{})
	selectTemplates *promptui.SelectTemplates
	MinerConfigPath string
	MinerConfig     *config.StorageMiner
	*harmonydb.DB
	MinerID address.Address
}

func configToDB(d *MigrationData) {
	d.say(section, "Migrating config.toml to database.\n")
	d.say(plain, "Curio run 1 instance per machine. Multiple machines cooperate through YugabyteDB.\n")
	d.say(plain, "For SPs with multiple Miner IDs, run 1 migration per lotus-miner all to the same 1 database. The cluster will serve all Miner IDs.\n")

	type rawConfig struct {
		Raw   []byte `db:"config"`
		Title string
	}
	var configBytes []rawConfig
	err := d.Select(context.Background(), &configBytes, `SELECT config, title FROM harmony_config `)
	if err != nil {
		d.say(notice, "Error reading from database: %s. Aborting Migration.\n", err.Error())
		os.Exit(1)
	}

	// SaveCredsToDB
	// 1. replace 'base' if not around with mig
	// 2. if base is already around, add the miner ID to the list with all the other goodies mig+MINERID
	// 3. If there's a base, diff it with the miner's config.toml and show the diff.
	// TODO watch what gets printed so we don't give advice to use "mig-??"

	//configs := lo.SliceToMap(configBytes, func(t rawConfig) (string, bool) {
	// TODO use new config reader
	//})
	// NEEDS multiaddress PR committed to interpret configs correctly.

	//TODO clean-up shared.go to be multilingual and use it.
	// TODO	d.say(plain, "This lotus-miner services the Miner ID: %s\n", "TODO MinerID")

	// This will be added to the new/existing base layer along with its specific wallet rules.
	// (if existing): Here's the diff of this miner's config.toml and the db's base layer.
	//      To edit, use the interactive editor by doing ....  after setup completes.
	// (if new): Writing the base layer.

	// commit the new base layer, also commit a miner-id-named layer.
	d.say(plain, "TODO FINISH THIS FUNCTION\n")
}

// bucket returns the nearest power of 2 (rounded down) for the given power.
func bucket(power *api.MinerPower) uint64 {
	return 1 << bits.LeadingZeros64(power.TotalPower.QualityAdjPower.Uint64())
}

func oneLastThing(d *MigrationData) {
	d.say(section, "To bring you the best SP tooling...")
	d.say(plain, "Share with the Curio team your interest in Curio for this Miner ID.\n")
	d.say(plain, "Hit return to tell http://CurioStorage.org that you've migrated to Curio.\n")
	_, err := (&promptui.Prompt{Label: d.T("Press return to continue")}).Run()
	if err != nil {
		d.say(notice, "Aborting remaining steps.\n", err.Error())
		os.Exit(1)
	}

	d.say(section, "We want to build what you're using.")
	i, _, err := (&promptui.Select{
		Label: d.T("Select what you want to share with the Curio team."),
		Items: []string{
			d.T("Individual Data: Miner ID, Curio version, net (mainnet/testnet). Signed."),
			d.T("Aggregate-Anonymous: Miner power (bucketed), version, and net."),
			d.T("Hint: I am someone running Curio on [test or main]."),
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
			msgMap["power"] = map[int]any{1: power, 2: bucket(power)}

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
		// TODO ensure this endpoint is up and running.
		http.DefaultClient.Post("https://curiostorage.org/api/v1/usage", "application/json", bytes.NewReader(msg))
	}
}

func doc(d *MigrationData) {
	d.say(plain, "The following configuration layers have been created for you: base, post, gui, seal.")
	d.say(plain, "Documentation: \n")
	d.say(plain, "Edit configuration layers with the command: \n")
	d.say(plain, "curio config edit <layername>\n\n")
	d.say(plain, "The 'base' layer should store common configuration. You likely want all curio to include it in their --layers argument.\n")
	d.say(plain, "Make other layers for per-machine changes.\n")

	d.say(plain, "Join #fil-curio-users in Filecoin slack for help.\n")
	d.say(plain, "TODO FINISH THIS FUNCTION.\n")
	// TODO !!
	// show the command to start the web interface & the command to start the main Curio instance.
	//    This is where Boost configuration can be completed.
	d.say(plain, "Want PoST redundancy? Run many Curio instances with the 'post' layer.\n")
	d.say(plain, "Point your browser to your web GUI to complete setup with Boost and advanced featues.\n")

	fmt.Println()
}

func verifySectors(d *MigrationData) {
	var i []int
	var lastError string
	d.say(section, "Waiting for lotus-miner to write sectors into Yugabyte.")
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
		time.Sleep(time.Second)
	}
	d.say(plain, "The sectors are in the database. The database is ready for Curio.\n")
	d.say(notice, "Now shut down lotus-miner and move the systems to Curio.\n")

	_, err := (&promptui.Prompt{Label: d.T("Press return to continue")}).Run()
	if err != nil {
		d.say(notice, "Aborting migration.\n")
		os.Exit(1)
	}
	stepCompleted(d, d.T("Sectors verified. %d sector locations found.\n", i))
}

func yugabyteConnect(d *MigrationData) {
	harmonycfg := d.MinerConfig.HarmonyDB //copy the config to a local variable
yugabyteFor:
	for {
		i, _, err := (&promptui.Select{
			Label: d.T("Enter the info to connect to your Yugabyte database installation (https://download.yugabyte.com/)"),
			Items: []string{
				d.T("Host: %s", strings.Join(harmonycfg.Hosts, ",")),
				d.T("Port: %s", harmonycfg.Port),
				d.T("Username: %s", harmonycfg.Username),
				d.T("Password: %s", harmonycfg.Password),
				d.T("Database: %s", harmonycfg.Database),
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
			harmonycfg.Hosts = strings.Split(host, ",")
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
				harmonycfg.Port = val
			case 2:
				harmonycfg.Username = val
			case 3:
				harmonycfg.Password = val
			case 4:
				harmonycfg.Database = val
			}
			continue
		case 5:
			d.DB, err = harmonydb.NewFromConfig(harmonycfg)
			if err != nil {
				if err.Error() == "^C" {
					os.Exit(1)
				}
				d.say(notice, "Error connecting to Yugabyte database: %s\n", err.Error())
				continue
			}
			break yugabyteFor
		}
	}

	d.say(plain, "Connected to Yugabyte. Schema is current.\n")
	if !reflect.DeepEqual(harmonycfg, d.MinerConfig.HarmonyDB) {
		d.MinerConfig.HarmonyDB = harmonycfg
		buf, err := config.ConfigUpdate(d.MinerConfig, config.DefaultStorageMiner())
		if err != nil {
			d.say(notice, "Error encoding config.toml: %s\n", err.Error())
			os.Exit(1)
		}
		_, _ = (&promptui.Prompt{Label: d.T("Press return to update config.toml with Yugabyte info. Backup the file now.")}).Run()
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
	d.say(plain, "To Start, ensure your sealing pipeline is drained and shut-down lotus-miner.\n")
	_, err := (&promptui.Prompt{Label: d.T("Press return to continue")}).Run()
	if err != nil {
		d.say(notice, "Aborting migration.\n")
		os.Exit(1)
	}
	verifyPath := func(dir string) (*config.StorageMiner, error) {
		cfg := config.DefaultStorageMiner()
		_, err := toml.DecodeFile(path.Join(dir, "config.toml"), &cfg)
		return cfg, err
	}

	dirs := map[string]*config.StorageMiner{"~/.lotusminer": nil, "~/.lotus-miner-local-net": nil}
	for dir := range dirs {
		cfg, err := verifyPath(dir)
		if err != nil {
			delete(dirs, dir)
		}
		dirs[dir] = cfg
	}

	var selected string
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
			selected = "Other"
		} else {
			d.MinerConfigPath = str
			d.MinerConfig = dirs[str]
		}
	}
	if selected == "Other" {
	minerPathEntry:
		str, err := (&promptui.Prompt{
			Label: d.T("Enter the path to the configuration directory used by lotus-miner"),
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
	d.say(plain, "Completed Step: %s\n\n", step)
}
