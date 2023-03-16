package config

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/xerrors"
)

// FromFile loads config from a specified file overriding defaults specified in
// the def parameter. If file does not exist or is empty defaults are assumed.
func FromFile(path string, def interface{}, lc loadabilityCheck) (interface{}, error) {
	// check for loadability
	file, err := os.Open(path)
	switch {
	case os.IsNotExist(err):
		if err := lc.configNotFound(); err != nil {
			return nil, err
		}
		return def, nil
	case err != nil:
		return nil, err
	}
	cfgBs, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, xerrors.Errorf("failed to read config for loadability checks %w", err)
	}
	s := string(cfgBs)
	if err := lc.configOk(s); err != nil {
		return nil, xerrors.Errorf("config failed loadability check: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	file, err = os.Open(path)
	switch {
	case os.IsNotExist(err):
		if err := lc.configNotFound(); err != nil {
			return nil, err
		}
		return def, nil
	case err != nil:
		return nil, err
	}
	defer file.Close() //nolint:errcheck // The file is RO
	return FromReader(file, def)
}

// FromReader loads config from a reader instance.
func FromReader(reader io.Reader, def interface{}) (interface{}, error) {
	cfg := def
	_, err := toml.NewDecoder(reader).Decode(cfg)
	if err != nil {
		return nil, err
	}

	err = envconfig.Process("LOTUS", cfg)
	if err != nil {
		return nil, fmt.Errorf("processing env vars overrides: %s", err)
	}

	return cfg, nil
}

// function from raw config string to loadability
type loadabilityCheck struct {
	configOk       func(string) error
	configNotFound func() error
}

func DefaultFullNodeLoadabilityCheck() loadabilityCheck {
	configOk := func(cfgRaw string) error {
		explicitSplitstoreRx := regexp.MustCompile("\n[\t\n\f\r ]*EnableSplitstore =")
		if match := explicitSplitstoreRx.FindString(cfgRaw); match == "" {
			return xerrors.Errorf("Config does not contain explicit set of EnableSplitstore field, refusing to load. Please explicitly set EnableSplitstore. Set it to false to keep chain management unchanged from March 2023 set it to true if you want to discard old state by default")
		}
		return nil
	}
	configNotFound := func() error {
		return xerrors.Errorf("FullNode config not found and fallback to default disallowed.  Set up config and explicitly set EnableSplitstore.  Set it to false to keep chain management unchanged from March 2023 set it to true if you want to discard old state by default ")
	}
	return loadabilityCheck{
		configOk:       configOk,
		configNotFound: configNotFound,
	}
}

func LoadabilityCheckNone() loadabilityCheck {
	return loadabilityCheck{
		configOk:       func(string) error { return nil },
		configNotFound: func() error { return nil },
	}
}

type keepUncommented func(string) bool

func DefaultFullNodeCommentFilter() keepUncommented {
	return func(s string) bool {
		return strings.Contains(s, "EnableSplitstore")
	}
}

func CommentFilterNone() keepUncommented {
	return func(string) bool {
		return false
	}
}

func ConfigUpdate(cfgCur, cfgDef interface{}, comment bool, filter keepUncommented) ([]byte, error) {
	var nodeStr, defStr string
	if cfgDef != nil {
		buf := new(bytes.Buffer)
		e := toml.NewEncoder(buf)
		if err := e.Encode(cfgDef); err != nil {
			return nil, xerrors.Errorf("encoding default config: %w", err)
		}

		defStr = buf.String()
	}

	{
		buf := new(bytes.Buffer)
		e := toml.NewEncoder(buf)
		if err := e.Encode(cfgCur); err != nil {
			return nil, xerrors.Errorf("encoding node config: %w", err)
		}

		nodeStr = buf.String()
	}

	if comment {
		// create a map of default lines, so we can comment those out later
		defLines := strings.Split(defStr, "\n")
		defaults := map[string]struct{}{}
		for i := range defLines {
			l := strings.TrimSpace(defLines[i])
			if len(l) == 0 {
				continue
			}
			if l[0] == '#' || l[0] == '[' {
				continue
			}
			defaults[l] = struct{}{}
		}

		nodeLines := strings.Split(nodeStr, "\n")
		var outLines []string

		sectionRx := regexp.MustCompile(`\[(.+)]`)
		var section string

		for i, line := range nodeLines {
			// if this is a section, track it
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				if trimmed[0] == '[' {
					m := sectionRx.FindSubmatch([]byte(trimmed))
					if len(m) != 2 {
						return nil, xerrors.Errorf("section didn't match (line %d)", i)
					}
					section = string(m[1])

					// never comment sections
					outLines = append(outLines, line)
					continue
				}
			}

			pad := strings.Repeat(" ", len(line)-len(strings.TrimLeftFunc(line, unicode.IsSpace)))

			// see if we have docs for this field
			{
				lf := strings.Fields(line)
				if len(lf) > 1 {
					doc := findDoc(cfgCur, section, lf[0])

					if doc != nil {
						// found docfield, emit doc comment
						if len(doc.Comment) > 0 {
							for _, docLine := range strings.Split(doc.Comment, "\n") {
								outLines = append(outLines, pad+"# "+docLine)
							}
							outLines = append(outLines, pad+"#")
						}

						outLines = append(outLines, pad+"# type: "+doc.Type)
					}

					outLines = append(outLines, pad+"# env var: LOTUS_"+strings.ToUpper(strings.ReplaceAll(section, ".", "_"))+"_"+strings.ToUpper(lf[0]))
				}
			}

			// if there is the same line in the default config, comment it out it output
			if _, found := defaults[strings.TrimSpace(nodeLines[i])]; (cfgDef == nil || found) && len(line) > 0 && !filter(line) {
				line = pad + "#" + line[len(pad):]
			}
			outLines = append(outLines, line)
			if len(line) > 0 {
				outLines = append(outLines, "")
			}
		}

		nodeStr = strings.Join(outLines, "\n")
	}

	// sanity-check that the updated config parses the same way as the current one
	if cfgDef != nil {
		cfgUpdated, err := FromReader(strings.NewReader(nodeStr), cfgDef)
		if err != nil {
			return nil, xerrors.Errorf("parsing updated config: %w", err)
		}

		if !reflect.DeepEqual(cfgCur, cfgUpdated) {
			return nil, xerrors.Errorf("updated config didn't match current config")
		}
	}

	return []byte(nodeStr), nil
}

func ConfigComment(t interface{}) ([]byte, error) {
	return ConfigUpdate(t, nil, true, DefaultFullNodeCommentFilter())
}
