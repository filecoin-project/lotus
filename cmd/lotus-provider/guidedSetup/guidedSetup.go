// guidedSetup for migration from lotus-miner to lotus-provider
//
//	IF STRINGS CHANGED {
//			follow instructions at ../internal/translations/translations.go
//	}
package guidedSetup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manifoldco/promptui"
	"github.com/samber/lo"

	_ "github.com/filecoin-project/lotus/cmd/lotus-provider/internal/translations"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var GuidedsetupCmd = &cli.Command{
	Name:  "guided-setup",
	Usage: "Run the guided setup for migrating from lotus-miner to lotus-provider",
	Action: func(cctx *cli.Context) (err error) {
		T, say := SetupLanguage()
		setupCtrlC(say)

		say(header, "This interactive tool will walk you through migration of lotus-provider.\nPress Ctrl+C to exit at any time.")

		say(notice, "This tool confirms each action it does.")

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
)

type migrationStep func(*MigrationData)

func SetupLanguage() (func(key message.Reference, a ...interface{}) string, func(style lipgloss.Style, key message.Reference, a ...interface{})) {
	lang, err := language.Parse(os.Getenv("LANG")[:2])
	if err != nil {
		lang = language.English
		fmt.Println("Error parsing language, defaulting to English")
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
	// probably should add default layers for enabling features.
	// probably should add a web layer for the web interface.
	//    This is where Boost configuration can be completed.
	// Ask if they want to report their MinerID as one that's running Provider
}

type MigrationData struct {
	T               func(key message.Reference, a ...interface{}) string
	say             func(style lipgloss.Style, key message.Reference, a ...interface{})
	selectTemplates *promptui.SelectTemplates
	MinerConfigPath string
	MinerConfig     *config.StorageMiner
	*harmonydb.DB
}

func configToDB(d *MigrationData) {
	d.say(section, "Migrating configuration to database.")
	d.say(notice, "This step will migrate the configuration from the config.toml to the database.\n")
	d.say(plain, `A Lotus-Miner cluster shares a database and shares the work of managing multiple Miner IDs and their sectors.\n`)

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
	//configs := lo.SliceToMap(configBytes, func(t rawConfig) (string, bool) {
	// TODO
	//})
	// NEEDS multiaddress PR committed to interpret configs correctly.

	// TODO	d.say(plain, "This lotus-miner services the Miner ID: %s\n", "TODO MinerID") // TODO!!!
	//TODO
	d.say(plain, "TODO FINISH THIS FUNCTION\n")
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
	d.say(plain, "Sectors verified. %d sector locations found.\n", i)
	d.say(plain, "Never remove the database info from the config.toml for lotus-miner as it avoids double PoSt.\n")

	stepCompleted(d, d.T("Verified Sectors in Database"))
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
		_, _ = (&promptui.Prompt{Label: "Press return to update config.toml with Yugabyte info. Backup the file now."}).Run()
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
